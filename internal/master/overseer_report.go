package master

import (
	"context"
	"sync"
	"time"

	"beacon/internal/overseer"
)

// overseerGuest is a single VM/container as reported to the cloud in the heartbeat.
type overseerGuest struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// overseerReport is the heartbeat section a Proxmox host sends about the guests it runs.
type overseerReport struct {
	Guests []overseerGuest `json:"guests"`
}

// guestLister is the slice of overseer.Overseer the heartbeat needs; an interface so the
// report builder is unit-testable without a live Proxmox host.
type guestLister interface {
	Available(ctx context.Context) bool
	ListGuests(ctx context.Context) ([]overseer.Guest, error)
}

var (
	overseerOnce      sync.Once
	cachedOverseer    guestLister
	overseerAvailable bool
)

// detectOverseer probes once (cached for the process lifetime) whether this host is a
// Proxmox node we can oversee, so we don't shell out to pvesh on every heartbeat just to
// learn we're not on Proxmox.
func detectOverseer() (guestLister, bool) {
	overseerOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		o := overseer.New()
		if o.Available(ctx) {
			cachedOverseer = o
			overseerAvailable = true
		}
	})
	return cachedOverseer, overseerAvailable
}

// toOverseerGuests maps overseer guests to the heartbeat wire shape.
func toOverseerGuests(guests []overseer.Guest) []overseerGuest {
	out := make([]overseerGuest, 0, len(guests))
	for _, g := range guests {
		out = append(out, overseerGuest{VMID: g.VMID, Name: g.Name, Status: g.Status, Type: g.Type})
	}
	return out
}

// buildOverseerReport returns the guest inventory when this host can oversee guests. The
// second return is whether this host is an overseer at all (role=host). A transient pvesh
// failure still reports role=host with an empty inventory, so a flaky enumerate doesn't
// make the host momentarily look like a plain device.
func buildOverseerReport(ctx context.Context, lister guestLister) (*overseerReport, bool) {
	if lister == nil {
		return nil, false
	}
	lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	guests, err := lister.ListGuests(lctx)
	if err != nil {
		logger.Warnf("overseer: could not enumerate guests this heartbeat: %v", err)
		return &overseerReport{Guests: []overseerGuest{}}, true
	}
	return &overseerReport{Guests: toOverseerGuests(guests)}, true
}
