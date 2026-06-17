package overseer

import (
	"context"
	"errors"
	"testing"
)

// fakeRunner returns canned output (or an error) and records the last command it saw.
type fakeRunner struct {
	out      []byte
	err      error
	lastName string
	lastArgs []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.lastName = name
	f.lastArgs = args
	return f.out, f.err
}

const sampleResources = `[
  {"vmid":110,"name":"ci-runner-2","status":"stopped","node":"pve","type":"qemu"},
  {"vmid":100,"name":"home-assistant","status":"running","node":"pve","type":"qemu"},
  {"vmid":105,"name":"pihole","status":"running","node":"pve","type":"lxc"}
]`

func TestListGuests_ParsesAndSorts(t *testing.T) {
	r := &fakeRunner{out: []byte(sampleResources)}
	o := NewWithRunner(r)

	guests, err := o.ListGuests(context.Background())
	if err != nil {
		t.Fatalf("ListGuests: %v", err)
	}
	if len(guests) != 3 {
		t.Fatalf("expected 3 guests, got %d", len(guests))
	}

	// Sorted by VMID ascending.
	wantIDs := []int{100, 105, 110}
	for i, g := range guests {
		if g.VMID != wantIDs[i] {
			t.Fatalf("guest %d: expected vmid %d, got %d", i, wantIDs[i], g.VMID)
		}
	}

	// Spot-check field mapping and Running().
	if guests[0].Name != "home-assistant" || !guests[0].Running() {
		t.Fatalf("expected running home-assistant first, got %+v", guests[0])
	}
	if guests[2].Status != "stopped" || guests[2].Running() {
		t.Fatalf("expected stopped ci-runner-2 last, got %+v", guests[2])
	}
	if guests[1].Type != "lxc" {
		t.Fatalf("expected lxc container, got %q", guests[1].Type)
	}

	// Correct pvesh invocation.
	if r.lastName != "pvesh" || len(r.lastArgs) == 0 || r.lastArgs[0] != "get" {
		t.Fatalf("unexpected command: %s %v", r.lastName, r.lastArgs)
	}
}

func TestListGuests_RunnerError(t *testing.T) {
	o := NewWithRunner(&fakeRunner{err: errors.New("pvesh: command not found")})
	if _, err := o.ListGuests(context.Background()); err == nil {
		t.Fatal("expected error when pvesh fails")
	}
}

func TestListGuests_BadJSON(t *testing.T) {
	o := NewWithRunner(&fakeRunner{out: []byte("not json")})
	if _, err := o.ListGuests(context.Background()); err == nil {
		t.Fatal("expected parse error on malformed output")
	}
}

func TestPowerAction_IssuesPveshTransition(t *testing.T) {
	cases := []struct {
		name     string
		vmid     int
		action   string
		wantPath string
	}{
		{"start qemu", 100, "start", "/nodes/pve/qemu/100/status/start"},
		{"reboot lxc", 105, "reboot", "/nodes/pve/lxc/105/status/reboot"},
		{"stop qemu", 110, "stop", "/nodes/pve/qemu/110/status/stop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{out: []byte(sampleResources)}
			o := NewWithRunner(r)
			if err := o.PowerAction(context.Background(), tc.vmid, tc.action); err != nil {
				t.Fatalf("PowerAction: %v", err)
			}
			// The last command must be the pvesh create on the resolved path.
			if r.lastName != "pvesh" || len(r.lastArgs) != 2 || r.lastArgs[0] != "create" || r.lastArgs[1] != tc.wantPath {
				t.Fatalf("unexpected command: %s %v", r.lastName, r.lastArgs)
			}
		})
	}
}

func TestPowerAction_RejectsBadAction(t *testing.T) {
	r := &fakeRunner{out: []byte(sampleResources)}
	o := NewWithRunner(r)
	if err := o.PowerAction(context.Background(), 100, "explode"); err == nil {
		t.Fatal("expected error for unsupported action")
	}
	// Must reject before touching pvesh.
	if r.lastName != "" {
		t.Fatalf("expected no pvesh call, got %s %v", r.lastName, r.lastArgs)
	}
}

func TestPowerAction_UnknownVMID(t *testing.T) {
	o := NewWithRunner(&fakeRunner{out: []byte(sampleResources)})
	if err := o.PowerAction(context.Background(), 999, "start"); !errors.Is(err, ErrUnknownGuest) {
		t.Fatalf("expected ErrUnknownGuest, got %v", err)
	}
}

func TestAvailable(t *testing.T) {
	if !NewWithRunner(&fakeRunner{out: []byte(`{"version":"8.1"}`)}).Available(context.Background()) {
		t.Fatal("expected Available true when pvesh /version succeeds")
	}
	if NewWithRunner(&fakeRunner{err: errors.New("no pvesh")}).Available(context.Background()) {
		t.Fatal("expected Available false when pvesh missing")
	}
}
