package audit

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func useTempLog(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	SetPathForTesting(p)
	t.Cleanup(func() { SetPathForTesting("") })
	return p
}

func TestRecordChainsAndReads(t *testing.T) {
	useTempLog(t)

	for i := 0; i < 3; i++ {
		if err := Record(Event{Action: "terminal_open", Source: "heartbeat", Status: "received"}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	entries, err := ReadAll()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	prev := ""
	for i, e := range entries {
		if e.Seq != int64(i+1) {
			t.Errorf("entry %d: seq = %d, want %d", i, e.Seq, i+1)
		}
		if e.PrevHash != prev {
			t.Errorf("entry %d: prev_hash not chained", i)
		}
		if e.Hash == "" {
			t.Errorf("entry %d: empty hash", i)
		}
		prev = e.Hash
	}
}

func TestRedaction(t *testing.T) {
	useTempLog(t)

	err := Record(Event{
		Action: "update_credential",
		Status: "received",
		Payload: map[string]any{
			"credential_type":  "git",
			"credential_value": "ghp_supersecret",
			"key_name":         "github_token",
			"nested":           map[string]any{"api_key": "abc123", "keep": "ok"},
		},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	entries, err := ReadAll()
	if err != nil || len(entries) != 1 {
		t.Fatalf("read: %v (n=%d)", err, len(entries))
	}
	p := entries[0].Payload
	if p["credential_value"] != "***" {
		t.Errorf("credential_value not redacted: %v", p["credential_value"])
	}
	if p["key_name"] != "github_token" {
		t.Errorf("key_name (a descriptive name, not a secret) should be preserved: %v", p["key_name"])
	}
	if p["credential_type"] != "git" {
		t.Errorf("credential_type should be preserved: %v", p["credential_type"])
	}
	nested, ok := p["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested payload missing or wrong type")
	}
	if nested["api_key"] != "***" {
		t.Errorf("nested api_key not redacted: %v", nested["api_key"])
	}
	if nested["keep"] != "ok" {
		t.Errorf("nested non-sensitive value altered: %v", nested["keep"])
	}
}

func TestVerifyOK(t *testing.T) {
	useTempLog(t)
	for i := 0; i < 5; i++ {
		if err := Record(Event{Action: "tunnel_connect", Status: "received"}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Verify()
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if !res.OK || res.Count != 5 {
		t.Fatalf("expected OK with 5 entries, got %+v", res)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	p := useTempLog(t)
	for i := 0; i < 4; i++ {
		if err := Record(Event{Action: "deploy_requested", Status: "received", Detail: "v1"}); err != nil {
			t.Fatal(err)
		}
	}

	// Tamper: flip a detail value on the 2nd line without recomputing hashes.
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := splitNonEmptyLines(string(data))
	lines[1] = replaceFirst(lines[1], `"detail":"v1"`, `"detail":"v2"`)
	if err := os.WriteFile(p, []byte(joinLines(lines)), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Verify()
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if res.OK {
		t.Fatalf("expected tamper to be detected, got OK")
	}
	if res.BrokenSeq != 2 {
		t.Errorf("expected break at seq 2, got %d (reason: %s)", res.BrokenSeq, res.Reason)
	}
}

func TestConcurrentAppendsChainCorrectly(t *testing.T) {
	useTempLog(t)
	var wg sync.WaitGroup
	const n = 30
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Record(Event{Action: "terminal_open", Status: "received"})
		}()
	}
	wg.Wait()

	res, err := Verify()
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if !res.OK || res.Count != n {
		t.Fatalf("expected intact chain of %d, got %+v", n, res)
	}
}

// --- tiny string helpers (avoid importing strings in test for clarity) ---

func splitNonEmptyLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func replaceFirst(s, old, new string) string {
	idx := indexOf(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
