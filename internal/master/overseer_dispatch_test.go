package master

import (
	"context"
	"testing"

	"beacon/internal/ipc"
)

type fakePowerActor struct {
	vmid   int
	action string
	err    error
	calls  int
}

func (f *fakePowerActor) PowerAction(_ context.Context, vmid int, action string) error {
	f.calls++
	f.vmid = vmid
	f.action = action
	return f.err
}

func newDispatcherWithActor(t *testing.T, actor overseerPowerActor) *CommandDispatcher {
	t.Helper()
	t.Setenv("BEACON_HOME", t.TempDir()) // contain audit-log writes
	d := NewCommandDispatcher(nil, nil)
	d.overseerActor = actor
	return d
}

func TestDispatchOverseerPower_Success(t *testing.T) {
	actor := &fakePowerActor{}
	d := newDispatcherWithActor(t, actor)

	d.dispatchOverseerPower(HeartbeatCommand{
		ID:      "c1",
		Action:  actionOverseerPower,
		Payload: map[string]any{"vmid": float64(100), "action": "reboot"},
	})

	if actor.calls != 1 || actor.vmid != 100 || actor.action != "reboot" {
		t.Fatalf("actor not invoked correctly: %+v", actor)
	}
	res := d.GetPendingResults()
	if len(res) != 1 || res[0].CommandID != "c1" || res[0].Status != ipc.ResultSuccess {
		t.Fatalf("expected one success result, got %+v", res)
	}
}

func TestDispatchOverseerPower_MissingPayload(t *testing.T) {
	actor := &fakePowerActor{}
	d := newDispatcherWithActor(t, actor)

	d.dispatchOverseerPower(HeartbeatCommand{ID: "c2", Action: actionOverseerPower, Payload: map[string]any{"action": "start"}})

	if actor.calls != 0 {
		t.Fatal("actor should not be called when vmid is missing")
	}
	res := d.GetPendingResults()
	if len(res) != 1 || res[0].Status != ipc.ResultFailed {
		t.Fatalf("expected one failed result, got %+v", res)
	}
}

func TestDispatchOverseerPower_ActorError(t *testing.T) {
	actor := &fakePowerActor{err: context.DeadlineExceeded}
	d := newDispatcherWithActor(t, actor)

	d.dispatchOverseerPower(HeartbeatCommand{
		ID:      "c3",
		Action:  actionOverseerPower,
		Payload: map[string]any{"vmid": float64(105), "action": "stop"},
	})

	res := d.GetPendingResults()
	if len(res) != 1 || res[0].Status != ipc.ResultFailed {
		t.Fatalf("expected one failed result, got %+v", res)
	}
}
