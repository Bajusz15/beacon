package k8sobserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"beacon/internal/sources"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// collectingSink records observations for tests.
type collectingSink struct {
	mu   sync.Mutex
	obs  []sources.Observation
	evts []sources.Event
}

func (c *collectingSink) RecordObservation(obs sources.Observation) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.obs = append(c.obs, obs)
	return nil
}

func (c *collectingSink) RecordEvent(ev sources.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evts = append(c.evts, ev)
	return nil
}

func (c *collectingSink) observations() []sources.Observation {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]sources.Observation, len(c.obs))
	copy(out, c.obs)
	return out
}

func TestObserverWithFakeClient(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
			UID:       "deploy-uid-123",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(2)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx:alpine"}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			AvailableReplicas:  2,
			ObservedGeneration: 1,
		},
	}
	deploy.Generation = 1

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc",
			Namespace: "default",
			UID:       "pod-uid-1",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
					UID:        "deploy-uid-123",
					Controller: ptr(true),
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Image:   "nginx:alpine",
					ImageID: "docker-pullable://nginx@sha256:abc",
					Ready:   true,
				},
			},
		},
	}

	// Fake client does not reliably sync pre-seeded objects with informers (no resource version).
	// Use a watch reactor so List+Watch complete; then create resources so the informer gets events.
	client := fake.NewSimpleClientset()
	watcherStarted := make(chan struct{})
	var closeOnce sync.Once
	client.PrependWatchReactor("*", func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
		gvr := action.GetResource()
		ns := action.GetNamespace()
		w, err := client.Tracker().Watch(gvr, ns)
		if err != nil {
			return false, nil, err
		}
		closeOnce.Do(func() { close(watcherStarted) })
		return true, w, nil
	})

	sink := &collectingSink{}
	cfg := K8sObserverConfig{
		SourceConfig: SourceConfig{Name: "test", Namespace: "default"},
		StateDir:     t.TempDir(),
		ClusterID:    "test-cluster",
	}

	obs, err := NewObserverWithClient(cfg, sink, client)
	if err != nil {
		t.Fatalf("NewObserverWithClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() { _ = obs.Start(ctx) }()

	// Wait for informers to complete initial List (sync can complete with empty cache).
	select {
	case <-obs.Synced():
		// Initial sync done (possibly with zero workloads).
	case <-ctx.Done():
		t.Fatal("observer did not complete initial sync in time")
	}

	// Wait for watcher to be started so Create events are not missed (fake client doesn't support resource version).
	select {
	case <-watcherStarted:
	case <-time.After(5 * time.Second):
		t.Log("warning: watcher started signal not received, continuing anyway")
	}

	// Now create resources so the informer's watch delivers ADD events (fake client tracker).
	_, err = client.AppsV1().Deployments("default").Create(ctx, deploy, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	_, err = client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}

	// Give the informer handlers time to process ADD events and update workload map.
	deadline := time.Now().Add(5 * time.Second)
	var workloads []sources.Observation
	for time.Now().Before(deadline) {
		workloads = obs.CurrentWorkloads()
		if len(workloads) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(workloads) == 0 {
		t.Fatal("expected at least one workload (Deployment nginx) after creating resources")
	}
	var found bool
	for _, w := range workloads {
		if w.Kind == "Deployment" && w.Name == "nginx" && w.Namespace == "default" {
			found = true
			if w.DesiredReplicas != 2 || w.AvailableReplicas != 2 {
				t.Errorf("replicas: desired=2 available=2, got desired=%d available=%d", w.DesiredReplicas, w.AvailableReplicas)
			}
			if len(w.DesiredImages) != 1 || w.DesiredImages[0] != "nginx:alpine" {
				t.Errorf("desired images: want [nginx:alpine], got %v", w.DesiredImages)
			}
			break
		}
	}
	if !found {
		t.Errorf("Deployment nginx not found in workloads: %+v", workloads)
	}
	cancel()
}

func ptr[T any](v T) *T { return &v }
