// Package overseer implements the Proxmox "overseer" role: a beacon agent running on a
// Proxmox VE host that enumerates the guests it hosts (VMs and containers), reports their
// up/down state, and (later) power-cycles them. It reaches the guests through the host's
// own pvesh CLI, so no in-guest agent or separate credentials are needed — the overseer
// sees what the hypervisor sees, which is exactly the state a crashed guest cannot report
// for itself.
package overseer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// ErrUnknownGuest is returned when a power action targets a vmid the host doesn't run.
var ErrUnknownGuest = errors.New("guest not found")

// validPowerActions are the pvesh status transitions the overseer will perform.
var validPowerActions = map[string]bool{
	"start":    true,
	"stop":     true,
	"reboot":   true,
	"shutdown": true,
}

// Guest is a VM or container as the Proxmox host sees it.
type Guest struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"` // "running" | "stopped" | other pvesh status
	Node   string `json:"node"`
	Type   string `json:"type"` // "qemu" (VM) | "lxc" (container)
}

// Running reports whether the guest is powered on.
func (g Guest) Running() bool { return g.Status == "running" }

// Runner executes a command and returns its stdout. It abstracts pvesh so the overseer
// is unit-testable without a live Proxmox host.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execRunner runs real binaries via os/exec.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		// exec.ExitError carries stderr; surface it for a useful message.
		var ee *exec.ExitError
		if as := asExitError(err, &ee); as && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

// asExitError is a tiny helper so the import of errors stays local to one spot.
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// Overseer enumerates and acts on Proxmox guests.
type Overseer struct {
	runner Runner
	// pveshBin is the pvesh binary path; overridable for tests/non-standard installs.
	pveshBin string
}

// New returns an Overseer that shells out to the host's pvesh.
func New() *Overseer {
	return &Overseer{runner: execRunner{}, pveshBin: "pvesh"}
}

// NewWithRunner returns an Overseer backed by a custom Runner (used in tests).
func NewWithRunner(r Runner) *Overseer {
	return &Overseer{runner: r, pveshBin: "pvesh"}
}

// pveshResource mirrors the relevant fields of a /cluster/resources entry.
type pveshResource struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Node   string `json:"node"`
	Type   string `json:"type"`
}

// ListGuests returns every VM and container the host knows about, sorted by VMID.
func (o *Overseer) ListGuests(ctx context.Context) ([]Guest, error) {
	out, err := o.runner.Run(ctx, o.pveshBin, "get", "/cluster/resources", "--type", "vm", "--output-format", "json")
	if err != nil {
		return nil, fmt.Errorf("pvesh list resources: %w", err)
	}

	var resources []pveshResource
	if err := json.Unmarshal(out, &resources); err != nil {
		return nil, fmt.Errorf("parse pvesh output: %w", err)
	}

	guests := make([]Guest, 0, len(resources))
	for _, r := range resources {
		guests = append(guests, Guest{
			VMID:   r.VMID,
			Name:   r.Name,
			Status: r.Status,
			Node:   r.Node,
			Type:   r.Type,
		})
	}
	sort.Slice(guests, func(i, j int) bool { return guests[i].VMID < guests[j].VMID })
	return guests, nil
}

// PowerAction performs a start/stop/reboot/shutdown on a guest by VMID. It resolves the
// guest's node and type (qemu vs lxc) from the live inventory, then issues the matching
// pvesh status transition — so it stays correct on a multi-node cluster, not just a single
// host. Unknown vmids and unsupported actions are rejected before any change is made.
func (o *Overseer) PowerAction(ctx context.Context, vmid int, action string) error {
	if !validPowerActions[action] {
		return fmt.Errorf("unsupported power action %q", action)
	}
	guests, err := o.ListGuests(ctx)
	if err != nil {
		return err
	}
	var target *Guest
	for i := range guests {
		if guests[i].VMID == vmid {
			target = &guests[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: vmid %d", ErrUnknownGuest, vmid)
	}

	kind := "qemu"
	if target.Type == "lxc" {
		kind = "lxc"
	}
	path := fmt.Sprintf("/nodes/%s/%s/%d/status/%s", target.Node, kind, vmid, action)
	if _, err := o.runner.Run(ctx, o.pveshBin, "create", path); err != nil {
		return fmt.Errorf("pvesh %s vmid %d: %w", action, vmid, err)
	}
	return nil
}

// Available reports whether pvesh is usable on this host (i.e. we are on a Proxmox VE node).
func (o *Overseer) Available(ctx context.Context) bool {
	_, err := o.runner.Run(ctx, o.pveshBin, "get", "/version", "--output-format", "json")
	return err == nil
}
