// Package audit provides a local, append-only, tamper-evident audit trail for
// consequential actions performed by the Beacon agent — especially actions
// triggered remotely by the BeaconInfra cloud (remote terminal, tunnels,
// deploys, credential updates, VPN changes).
//
// The log lives at ~/.beacon/logs/audit.jsonl. Each record is a single JSON
// line and carries the SHA-256 hash of the previous record (prev_hash) plus its
// own hash. This forms a hash chain: any insertion, deletion, or edit of a past
// record breaks the chain and is detectable with `beacon audit verify`.
//
// Writing is best-effort and must never block or break the action being
// audited. Callers should prefer Log (which swallows errors) over Record.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"beacon/internal/config"
	"beacon/internal/util"
)

// FileName is the audit log filename within the Beacon logs directory.
const FileName = "audit.jsonl"

// Event is the input a caller provides to record an audit entry. The chain
// fields (Seq, TS, PrevHash, Hash) are filled in by the package.
type Event struct {
	Action    string         // e.g. terminal_open, tunnel_connect, deploy_requested
	Source    string         // heartbeat | control_ws | monitor | local
	Status    string         // received | executed | failed | denied | started | ended
	CommandID string         // cloud command id, if any
	DeviceID  string         // BeaconInfra device id, if known
	Device    string         // device name
	Project   string         // target project, if any
	Detail    string         // human-readable message or error
	Payload   map[string]any // command payload (sensitive values are redacted)
}

// Entry is a single persisted, hash-chained audit record.
type Entry struct {
	Seq       int64          `json:"seq"`
	TS        string         `json:"ts"`
	Action    string         `json:"action"`
	Source    string         `json:"source"`
	Status    string         `json:"status"`
	CommandID string         `json:"command_id,omitempty"`
	DeviceID  string         `json:"device_id,omitempty"`
	Device    string         `json:"device,omitempty"`
	Project   string         `json:"project,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	PrevHash  string         `json:"prev_hash"`
	Hash      string         `json:"hash"`
}

var (
	mu sync.Mutex

	// pathOverride lets tests redirect the audit file. Empty means use the
	// standard ~/.beacon/logs/audit.jsonl location.
	pathOverride string
)

// SetPathForTesting overrides the audit log path. Intended for tests only.
func SetPathForTesting(p string) { pathOverride = p }

// Path returns the resolved audit log path.
func Path() (string, error) {
	if pathOverride != "" {
		return pathOverride, nil
	}
	base, err := config.BeaconHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "logs", FileName), nil
}

// Log records an event, ignoring any error. Use this from action paths so a
// failure to write the audit trail never breaks the underlying operation.
func Log(ev Event) {
	_ = Record(ev)
}

// Record appends an event to the tamper-evident audit log. It is safe for
// concurrent use across goroutines and across processes (the master agent and
// per-project monitors all write to the same file under an advisory lock).
func Record(ev Event) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(p), 0o755); mkErr != nil {
		return mkErr
	}

	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer util.Close(f, "audit log file")

	if lockErr := lockFD(int(f.Fd())); lockErr != nil {
		return lockErr
	}
	defer func() { _ = unlockFD(int(f.Fd())) }()

	prevHash, prevSeq, err := lastChain(f)
	if err != nil {
		return err
	}

	entry := Entry{
		Seq:       prevSeq + 1,
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		Action:    ev.Action,
		Source:    ev.Source,
		Status:    ev.Status,
		CommandID: ev.CommandID,
		DeviceID:  ev.DeviceID,
		Device:    ev.Device,
		Project:   ev.Project,
		Detail:    ev.Detail,
		Payload:   redact(ev.Payload),
		PrevHash:  prevHash,
	}
	entry.Hash = computeHash(entry)

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// ReadAll returns every audit entry in chronological order. A missing file is
// not an error; it yields an empty slice.
func ReadAll() ([]Entry, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	defer util.Close(f, "audit log file")

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// A malformed line is itself evidence of tampering/corruption;
			// surface it rather than silently skipping.
			return entries, err
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

// VerifyResult reports the outcome of a chain integrity check.
type VerifyResult struct {
	OK        bool   // true if the chain is intact
	Count     int    // number of entries checked
	BrokenSeq int64  // seq of the first broken entry (0 if OK)
	Reason    string // human-readable reason for a break
}

// Verify recomputes the hash chain and reports the first inconsistency, if any.
func Verify() (VerifyResult, error) {
	entries, err := ReadAll()
	if err != nil {
		return VerifyResult{OK: false, Reason: "log unreadable or corrupt: " + err.Error()}, err
	}
	prev := ""
	for i, e := range entries {
		if e.PrevHash != prev {
			return VerifyResult{OK: false, Count: i, BrokenSeq: e.Seq, Reason: "prev_hash mismatch (record inserted, deleted, or reordered)"}, nil
		}
		if computeHash(e) != e.Hash {
			return VerifyResult{OK: false, Count: i, BrokenSeq: e.Seq, Reason: "hash mismatch (record contents were modified)"}, nil
		}
		prev = e.Hash
	}
	return VerifyResult{OK: true, Count: len(entries)}, nil
}

// lastChain returns the hash and seq of the final record currently in f.
// For an empty log it returns ("", 0).
func lastChain(f *os.File) (hash string, seq int64, err error) {
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if jsonErr := json.Unmarshal([]byte(line), &e); jsonErr != nil {
			return "", 0, jsonErr
		}
		hash = e.Hash
		seq = e.Seq
	}
	if scErr := sc.Err(); scErr != nil {
		return "", 0, scErr
	}
	return hash, seq, nil
}

// computeHash returns the SHA-256 (hex) of the entry with its Hash field
// cleared. Because Go marshals struct fields in declaration order and map keys
// in sorted order, the encoding is deterministic.
func computeHash(e Entry) string {
	e.Hash = ""
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// sensitiveKeys are matched as case-insensitive substrings of a payload key.
// They are intentionally specific: we redact secret values, not descriptive
// names like key_name or credential_type, which keep the trail useful.
var sensitiveKeys = []string{
	"token", "secret", "password", "passphrase",
	"credential_value", "api_key", "apikey", "authorization",
	"private_key", "privatekey", "access_key",
}

// redact returns a copy of in with the values of sensitive keys replaced by
// "***". It recurses into nested maps. Keys like credential_type or key_name
// are preserved so the trail stays useful.
func redact(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSensitive(k) {
			out[k] = "***"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = redact(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitive(key string) bool {
	lk := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}
