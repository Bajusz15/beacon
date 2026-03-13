package mcp

import (
	"testing"
)

func TestValidateGitRef(t *testing.T) {
	tests := []struct {
		ref   string
		valid bool
	}{
		{"v1.0.0", true},
		{"main", true},
		{"abc123", true},
		{"feature/foo-bar", true},
		{"", false},
		{"ref;rm -rf", false},
		{"ref$HOME", false},
	}
	for _, tt := range tests {
		err := ValidateGitRef(tt.ref)
		if tt.valid && err != nil {
			t.Errorf("ValidateGitRef(%q): unexpected error %v", tt.ref, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ValidateGitRef(%q): expected error", tt.ref)
		}
	}
}

func TestValidateGrepPattern(t *testing.T) {
	_, err := ValidateGrepPattern("hello")
	if err != nil {
		t.Errorf("ValidateGrepPattern(hello): %v", err)
	}
	_, err = ValidateGrepPattern(string(make([]byte, 300)))
	if err == nil {
		t.Error("expected error for long pattern")
	}
}

func TestConfirmationTokenStore(t *testing.T) {
	s := NewConfirmationTokenStore()
	token, err := s.CreateToken("beacon_deploy", map[string]any{"project": "p1", "ref": "v1"})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	tool, args, err := s.Confirm(token)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if tool != "beacon_deploy" {
		t.Errorf("tool %q", tool)
	}
	if args["project"] != "p1" {
		t.Errorf("args %v", args)
	}

	_, _, err = s.Confirm(token)
	if err == nil {
		t.Error("expected error on double confirm")
	}
}
