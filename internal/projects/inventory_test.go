package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInventory_LoadSaveAddRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	inv, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}
	if len(inv.Projects) != 0 {
		t.Errorf("expected empty inventory, got %d", len(inv.Projects))
	}

	AddProject(inv, "proj1", "/home/user/beacon/proj1", "/home/.beacon/config/projects/proj1")
	AddProject(inv, "proj2", "/home/user/beacon/proj2", "/home/.beacon/config/projects/proj2")

	if err := SaveInventory(path, inv); err != nil {
		t.Fatalf("SaveInventory: %v", err)
	}

	inv2, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("LoadInventory (reload): %v", err)
	}
	if len(inv2.Projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(inv2.Projects))
	}

	p := GetProject(inv2, "proj1")
	if p == nil || p.Location != "/home/user/beacon/proj1" {
		t.Errorf("GetProject proj1: got %+v", p)
	}

	RemoveProject(inv2, "proj1")
	if err := SaveInventory(path, inv2); err != nil {
		t.Fatalf("SaveInventory after remove: %v", err)
	}

	inv3, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("LoadInventory (after remove): %v", err)
	}
	if len(inv3.Projects) != 1 || inv3.Projects[0].Name != "proj2" {
		t.Errorf("expected proj2 only, got %+v", inv3.Projects)
	}
}

func TestInventory_AddProjectUpdatesExisting(t *testing.T) {
	inv := &Inventory{Projects: []ProjectEntry{}}
	AddProject(inv, "p1", "/old/loc", "/old/config")
	AddProject(inv, "p1", "/new/loc", "/new/config")

	if len(inv.Projects) != 1 {
		t.Fatalf("expected 1 project after update, got %d", len(inv.Projects))
	}
	if inv.Projects[0].Location != "/new/loc" {
		t.Errorf("expected updated location, got %s", inv.Projects[0].Location)
	}
}

func TestInventory_LoadNonExistent(t *testing.T) {
	inv, err := LoadInventory(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("LoadInventory non-existent: %v", err)
	}
	if inv == nil || len(inv.Projects) != 0 {
		t.Errorf("expected empty inventory for non-existent file")
	}
}

func TestInventory_SaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "projects.json")

	inv := &Inventory{Projects: []ProjectEntry{{Name: "x", Location: "/l", ConfigDir: "/c"}}}
	if err := SaveInventory(path, inv); err != nil {
		t.Fatalf("SaveInventory: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
