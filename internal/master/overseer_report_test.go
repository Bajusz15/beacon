package master

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"beacon/internal/overseer"
)

type fakeLister struct {
	guests []overseer.Guest
	err    error
}

func (f fakeLister) Available(context.Context) bool { return true }
func (f fakeLister) ListGuests(context.Context) ([]overseer.Guest, error) {
	return f.guests, f.err
}

func TestBuildOverseerReport_MapsGuests(t *testing.T) {
	lister := fakeLister{guests: []overseer.Guest{
		{VMID: 100, Name: "home-assistant", Status: "running", Type: "qemu"},
		{VMID: 105, Name: "pihole", Status: "stopped", Type: "lxc"},
	}}
	rep, ok := buildOverseerReport(context.Background(), lister)
	if !ok {
		t.Fatal("expected ok=true for a reachable overseer")
	}
	if len(rep.Guests) != 2 {
		t.Fatalf("expected 2 guests, got %d", len(rep.Guests))
	}
	if rep.Guests[0].Name != "home-assistant" || rep.Guests[0].Status != "running" || rep.Guests[0].Type != "qemu" {
		t.Fatalf("unexpected first guest: %+v", rep.Guests[0])
	}
}

func TestBuildOverseerReport_EnumerateErrorStillHost(t *testing.T) {
	// A transient pvesh failure must still report role=host (ok=true) with no guests,
	// so the host doesn't flicker back to looking like a plain device.
	rep, ok := buildOverseerReport(context.Background(), fakeLister{err: errors.New("pvesh timeout")})
	if !ok {
		t.Fatal("expected ok=true even when enumeration fails")
	}
	if rep == nil || len(rep.Guests) != 0 {
		t.Fatalf("expected empty guest list, got %+v", rep)
	}
}

func TestBuildOverseerReport_NilListerNotHost(t *testing.T) {
	if _, ok := buildOverseerReport(context.Background(), nil); ok {
		t.Fatal("expected ok=false when there is no overseer")
	}
}

func TestHeartbeatRequest_OmitsRoleWhenNotHost(t *testing.T) {
	// A plain device's heartbeat must not carry role/overseer keys at all.
	body, err := json.Marshal(heartbeatRequest{Hostname: "pi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(body)
	if strings.Contains(s, "\"role\"") || strings.Contains(s, "\"overseer\"") {
		t.Fatalf("expected role/overseer omitted for non-host, got %s", s)
	}
}

func TestHeartbeatRequest_IncludesOverseerWhenHost(t *testing.T) {
	rep := &overseerReport{Guests: []overseerGuest{{VMID: 100, Name: "ha", Status: "running", Type: "qemu"}}}
	body, err := json.Marshal(heartbeatRequest{Hostname: "pve", Role: "host", Overseer: rep})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "\"role\":\"host\"") || !strings.Contains(s, "\"overseer\"") || !strings.Contains(s, "\"vmid\":100") {
		t.Fatalf("expected role/overseer/guest in payload, got %s", s)
	}
}
