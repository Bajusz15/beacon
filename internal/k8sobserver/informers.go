package k8sobserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"beacon/internal/sources"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	resyncPeriod = 15 * time.Minute
	indexByOwner = "by-owner"
)

// Observer implements sources.Source for Kubernetes (read-only).
type Observer struct {
	cfg       K8sObserverConfig
	sink      sources.Sink
	client    kubernetes.Interface
	workloads map[string]WorkloadSnapshot
	workMu    sync.RWMutex
	stopCh    chan struct{}
	syncedCh  chan struct{} // closed after first cache sync and emit
}

// NewObserver builds an observer from config and sink. Call Start to begin watching.
func NewObserver(cfg K8sObserverConfig, sink sources.Sink) (*Observer, error) {
	restConfig, err := buildConfig(cfg.SourceConfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return NewObserverWithClient(cfg, sink, client)
}

// NewObserverWithClient builds an observer with a given Kubernetes client (e.g. fake client for tests).
func NewObserverWithClient(cfg K8sObserverConfig, sink sources.Sink, client kubernetes.Interface) (*Observer, error) {
	return &Observer{
		cfg:       cfg,
		sink:      sink,
		client:    client,
		workloads: make(map[string]WorkloadSnapshot),
		stopCh:    make(chan struct{}),
		syncedCh:  make(chan struct{}),
	}, nil
}

func buildConfig(src SourceConfig) (*rest.Config, error) {
	if src.InCluster {
		return rest.InClusterConfig()
	}
	kubeconfig := src.Kubeconfig
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// Name implements sources.Source.
func (o *Observer) Name() string { return o.cfg.SourceConfig.Name() }

// Type implements sources.Source.
func (o *Observer) Type() string { return "kubernetes" }

// Start implements sources.Source. It starts informers and blocks until ctx is done.
func (o *Observer) Start(ctx context.Context) error {
	ns := o.cfg.SourceConfig.Namespace
	var factory informers.SharedInformerFactory
	if ns != "" {
		factory = informers.NewSharedInformerFactoryWithOptions(o.client, resyncPeriod, informers.WithNamespace(ns))
	} else {
		factory = informers.NewSharedInformerFactory(o.client, resyncPeriod)
	}

	// Pod index: by owner UID so we can look up pods for a workload
	podIndexers := cache.Indexers{
		indexByOwner: func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return nil, nil
			}
			for _, ref := range pod.OwnerReferences {
				if ref.Controller != nil && *ref.Controller {
					return []string{pod.Namespace + "/" + string(ref.UID)}, nil
				}
			}
			return nil, nil
		},
	}
	podInformer := factory.Core().V1().Pods().Informer()
	if err := podInformer.AddIndexers(podIndexers); err != nil {
		return err
	}

	deployInformer := factory.Apps().V1().Deployments().Informer()
	stsInformer := factory.Apps().V1().StatefulSets().Informer()
	dsInformer := factory.Apps().V1().DaemonSets().Informer()

	deployInformer.AddEventHandler(o.deploymentHandler())
	stsInformer.AddEventHandler(o.statefulSetHandler())
	dsInformer.AddEventHandler(o.daemonSetHandler())
	podInformer.AddEventHandler(o.podHandler(podInformer.GetIndexer()))

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), deployInformer.HasSynced, stsInformer.HasSynced, dsInformer.HasSynced, podInformer.HasSynced) {
		return ctx.Err()
	}

	// One-time full snapshot after sync
	o.emitAllWorkloads()
	close(o.syncedCh)

	// Block until context is done
	<-ctx.Done()
	return nil
}

// Stop implements sources.Source.
func (o *Observer) Stop() error {
	close(o.stopCh)
	return nil
}

func (o *Observer) deploymentHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { o.upsertDeployment(obj.(*appsv1.Deployment)) },
		UpdateFunc: func(_, newObj interface{}) { o.upsertDeployment(newObj.(*appsv1.Deployment)) },
		DeleteFunc: func(obj interface{}) { o.deleteWorkload(workloadID(o.cfg.ClusterID, obj.(*appsv1.Deployment).Namespace, "Deployment", obj.(*appsv1.Deployment).Name)) },
	}
}

func (o *Observer) statefulSetHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { o.upsertStatefulSet(obj.(*appsv1.StatefulSet)) },
		UpdateFunc: func(_, newObj interface{}) { o.upsertStatefulSet(newObj.(*appsv1.StatefulSet)) },
		DeleteFunc: func(obj interface{}) { o.deleteWorkload(workloadID(o.cfg.ClusterID, obj.(*appsv1.StatefulSet).Namespace, "StatefulSet", obj.(*appsv1.StatefulSet).Name)) },
	}
}

func (o *Observer) daemonSetHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { o.upsertDaemonSet(obj.(*appsv1.DaemonSet)) },
		UpdateFunc: func(_, newObj interface{}) { o.upsertDaemonSet(newObj.(*appsv1.DaemonSet)) },
		DeleteFunc: func(obj interface{}) { o.deleteWorkload(workloadID(o.cfg.ClusterID, obj.(*appsv1.DaemonSet).Namespace, "DaemonSet", obj.(*appsv1.DaemonSet).Name)) },
	}
}

func (o *Observer) podHandler(indexer cache.Indexer) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { o.onPodChange(indexer, obj.(*corev1.Pod)) },
		UpdateFunc: func(_, newObj interface{}) { o.onPodChange(indexer, newObj.(*corev1.Pod)) },
		DeleteFunc: func(obj interface{}) { o.onPodChange(indexer, obj.(*corev1.Pod)) },
	}
}

func (o *Observer) upsertDeployment(d *appsv1.Deployment) {
	id := workloadID(o.cfg.ClusterID, d.Namespace, "Deployment", d.Name)
	now := time.Now().UTC()
	desiredReplicas := int32(1)
	if d.Spec.Replicas != nil {
		desiredReplicas = *d.Spec.Replicas
	}
	desiredImages := containerImagesFromSpec(d.Spec.Template.Spec.Containers)
	available := int32(0)
	if d.Status.AvailableReplicas > 0 {
		available = d.Status.AvailableReplicas
	}
	observedGen := d.Status.ObservedGeneration
	specGen := d.Generation

	o.workMu.Lock()
	w, exists := o.workloads[id]
	if !exists {
		w = WorkloadSnapshot{ID: id, ClusterID: o.cfg.ClusterID, Namespace: d.Namespace, Kind: "Deployment", Name: d.Name, UID: string(d.UID), FirstSeen: now}
	}
	w.DesiredReplicas = desiredReplicas
	w.AvailableReplicas = available
	w.SpecGeneration = specGen
	w.ObservedGeneration = observedGen
	w.DesiredImages = desiredImages
	w.LastSeen = now
	w.LastChange = now
	w.UID = string(d.UID)
	w.Conditions = deploymentConditions(d)
	o.workloads[id] = w
	o.workMu.Unlock()

	o.emitWorkload(id)
}

func (o *Observer) upsertStatefulSet(s *appsv1.StatefulSet) {
	id := workloadID(o.cfg.ClusterID, s.Namespace, "StatefulSet", s.Name)
	now := time.Now().UTC()
	desiredReplicas := int32(1)
	if s.Spec.Replicas != nil {
		desiredReplicas = *s.Spec.Replicas
	}
	desiredImages := containerImagesFromSpec(s.Spec.Template.Spec.Containers)
	available := s.Status.AvailableReplicas
	observedGen := s.Status.ObservedGeneration
	specGen := s.Generation

	o.workMu.Lock()
	w, exists := o.workloads[id]
	if !exists {
		w = WorkloadSnapshot{ID: id, ClusterID: o.cfg.ClusterID, Namespace: s.Namespace, Kind: "StatefulSet", Name: s.Name, UID: string(s.UID), FirstSeen: now}
	}
	w.DesiredReplicas = desiredReplicas
	w.AvailableReplicas = available
	w.SpecGeneration = specGen
	w.ObservedGeneration = observedGen
	w.DesiredImages = desiredImages
	w.LastSeen = now
	w.LastChange = now
	w.UID = string(s.UID)
	w.Conditions = statefulSetConditions(s)
	o.workloads[id] = w
	o.workMu.Unlock()

	o.emitWorkload(id)
}

func (o *Observer) upsertDaemonSet(ds *appsv1.DaemonSet) {
	id := workloadID(o.cfg.ClusterID, ds.Namespace, "DaemonSet", ds.Name)
	now := time.Now().UTC()
	desiredReplicas := ds.Status.DesiredNumberScheduled
	available := ds.Status.NumberAvailable
	observedGen := ds.Status.ObservedGeneration
	specGen := ds.Generation
	desiredImages := containerImagesFromSpec(ds.Spec.Template.Spec.Containers)

	o.workMu.Lock()
	w, exists := o.workloads[id]
	if !exists {
		w = WorkloadSnapshot{ID: id, ClusterID: o.cfg.ClusterID, Namespace: ds.Namespace, Kind: "DaemonSet", Name: ds.Name, UID: string(ds.UID), FirstSeen: now}
	}
	w.DesiredReplicas = desiredReplicas
	w.AvailableReplicas = available
	w.SpecGeneration = specGen
	w.ObservedGeneration = observedGen
	w.DesiredImages = desiredImages
	w.LastSeen = now
	w.LastChange = now
	w.UID = string(ds.UID)
	w.Conditions = daemonSetConditions(ds)
	o.workloads[id] = w
	o.workMu.Unlock()

	o.emitWorkload(id)
}

func (o *Observer) onPodChange(indexer cache.Indexer, pod *corev1.Pod) {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		key := pod.Namespace + "/" + string(ref.UID)
		objs, err := indexer.ByIndex(indexByOwner, key)
		if err != nil {
			log.Printf("[Beacon] k8s index lookup: %v", err)
			return
		}
		id := workloadID(o.cfg.ClusterID, pod.Namespace, ref.Kind, ref.Name)
		o.refreshWorkloadFromPods(id, ref.Kind, objs)
		return
	}
}

func (o *Observer) refreshWorkloadFromPods(id, kind string, pods []interface{}) {
	o.workMu.Lock()
	w, ok := o.workloads[id]
	if !ok {
		o.workMu.Unlock()
		return
	}
	var runningImages, runningDigests []string
	var healthSignals []string
	restartTotal := 0
	phaseCount := make(map[string]int)
	for _, obj := range pods {
		pod, _ := obj.(*corev1.Pod)
		if pod == nil {
			continue
		}
		phaseCount[string(pod.Status.Phase)]++
		for _, c := range pod.Status.ContainerStatuses {
			restartTotal += int(c.RestartCount)
			img, dig := ParseImageID(c.ImageID)
			if img != "" {
				runningImages = append(runningImages, img)
			}
			if dig != "" {
				runningDigests = append(runningDigests, dig)
			}
			if c.State.Waiting != nil {
				switch c.State.Waiting.Reason {
				case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull":
					healthSignals = append(healthSignals, c.State.Waiting.Reason)
				}
			}
			if c.LastTerminationState.Terminated != nil && c.LastTerminationState.Terminated.Reason == "OOMKilled" {
				healthSignals = append(healthSignals, HealthOOMKilled)
			}
		}
	}
	w.RunningImages = runningImages
	w.RunningDigests = runningDigests
	w.HealthSignals = healthSignals
	w.PodRestartCount = restartTotal
	w.PodCountByPhase = phaseCount
	w.LastSeen = time.Now().UTC()
	w.InDrift, w.DriftReasons = computeDrift(w)
	o.workloads[id] = w
	o.workMu.Unlock()
	o.emitWorkload(id)
}

func computeDrift(w WorkloadSnapshot) (bool, []string) {
	var reasons []string
	if w.SpecGeneration != w.ObservedGeneration {
		reasons = append(reasons, DriftReasonGeneration)
	}
	if w.AvailableReplicas < w.DesiredReplicas {
		reasons = append(reasons, DriftReasonReplicas)
	}
	if len(w.HealthSignals) > 0 {
		reasons = append(reasons, "unhealthy_pods")
	}
	// Optional: image drift (desired tag vs running digest) - compare if we have both
	return len(reasons) > 0, reasons
}

func (o *Observer) deleteWorkload(id string) {
	o.workMu.Lock()
	w, ok := o.workloads[id]
	if ok {
		_ = o.sink.RecordEvent(EventFromSnapshot("workload_deleted", "deleted", w, nil))
		delete(o.workloads, id)
	}
	o.workMu.Unlock()
}

func (o *Observer) emitWorkload(id string) {
	o.workMu.RLock()
	w, ok := o.workloads[id]
	o.workMu.RUnlock()
	if !ok {
		return
	}
	obs := w.toObservation()
	if err := o.sink.RecordObservation(obs); err != nil {
		log.Printf("[Beacon] k8s sink RecordObservation: %v", err)
	}
	if w.InDrift {
		_ = o.sink.RecordEvent(EventFromSnapshot("drift_detected", "drift", w, map[string]interface{}{"reasons": w.DriftReasons}))
	}
}

func (o *Observer) emitAllWorkloads() {
	o.workMu.RLock()
	snapshots := make([]WorkloadSnapshot, 0, len(o.workloads))
	for _, w := range o.workloads {
		snapshots = append(snapshots, w)
	}
	o.workMu.RUnlock()
	observations := make([]sources.Observation, 0, len(snapshots))
	for _, w := range snapshots {
		observations = append(observations, w.toObservation())
	}
	if len(observations) > 0 {
		if batch, ok := o.sink.(interface{ RecordObservationsBatch([]sources.Observation) error }); ok {
			_ = batch.RecordObservationsBatch(observations)
		} else {
			for _, obs := range observations {
				_ = o.sink.RecordObservation(obs)
			}
		}
	}
}

func containerImagesFromSpec(containers []corev1.Container) []string {
	out := make([]string, 0, len(containers))
	for _, c := range containers {
		out = append(out, c.Image)
	}
	return out
}

func deploymentConditions(d *appsv1.Deployment) []string {
	var c []string
	for _, cond := range d.Status.Conditions {
		if cond.Status == corev1.ConditionTrue {
			c = append(c, string(cond.Type)+"="+string(cond.Status))
		}
	}
	return c
}

func statefulSetConditions(s *appsv1.StatefulSet) []string {
	var c []string
	for _, cond := range s.Status.Conditions {
		if cond.Status == corev1.ConditionTrue {
			c = append(c, string(cond.Type)+"="+string(cond.Status))
		}
	}
	return c
}

func daemonSetConditions(ds *appsv1.DaemonSet) []string {
	var c []string
	for _, cond := range ds.Status.Conditions {
		if cond.Status == corev1.ConditionTrue {
			c = append(c, string(cond.Type)+"="+string(cond.Status))
		}
	}
	return c
}

func workloadID(clusterID, namespace, kind, name string) string {
	return clusterID + "/" + namespace + "/" + kind + "/" + name
}

// SourceConfig.Name() - we have Name as field; add method if needed.
func (s SourceConfig) Name() string {
	if s.Name != "" {
		return s.Name
	}
	return "kubernetes-default"
}

// ClusterIDFromKubeconfig returns a short stable id for the cluster (e.g. for in-cluster use "in-cluster").
func ClusterIDFromKubeconfig(kubeconfig string, inCluster bool) string {
	if inCluster {
		return "in-cluster"
	}
	h := sha256.Sum256([]byte(kubeconfig))
	return hex.EncodeToString(h[:])[:12]
}

// Synced returns a channel that is closed after the first cache sync and snapshot emit.
func (o *Observer) Synced() <-chan struct{} { return o.syncedCh }

// CurrentWorkloads returns a copy of the current workload snapshots (for status CLI).
func (o *Observer) CurrentWorkloads() []sources.Observation {
	o.workMu.RLock()
	defer o.workMu.RUnlock()
	out := make([]sources.Observation, 0, len(o.workloads))
	for _, w := range o.workloads {
		out = append(out, w.toObservation())
	}
	return out
}

// RunObserverOnce starts the observer, waits for first sync, returns current workloads, then cancels. Used by status CLI.
func RunObserverOnce(cfg K8sObserverConfig, sink sources.Sink, timeout time.Duration) ([]sources.Observation, error) {
	obs, err := NewObserver(cfg, sink)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() { _ = obs.Start(ctx) }()
	select {
	case <-obs.Synced():
		return obs.CurrentWorkloads(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
