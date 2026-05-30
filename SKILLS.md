# Beacon coding conventions

Project-specific conventions for this repo (and beaconinfra, which is also Go).
These complement `CLAUDE.md`.

## Never silently ignore errors

Do **not** discard errors with the blank identifier. These are banned:

```go
defer func() { _ = f.Close() }()          // NO
_, _ = fmt.Fprintf(w, "...", ...)         // NO
_ = w.Flush()                             // NO
```

Every error must be checked. When the call is in a `defer` or a spot where you
genuinely cannot propagate the error, route it through a helper that **checks
and logs** it — never `_ =`.

### Closing resources

Use the existing helpers in `internal/util/closer.go`:

```go
defer util.Close(f, "audit log file")     // checks Close() and logs on failure
// or, when you need the func value:
defer util.DeferClose(f, "audit log file")()
```

### Other ignored returns (Fprintf, Flush, Write, …)

Check the error and route it through `util.LogError` (or propagate it):

```go
if _, err := fmt.Fprintln(w, header); err != nil {
    util.LogError(err, "write audit table header")
    return
}
if err := w.Flush(); err != nil {
    util.LogError(err, "flush audit table")
}
```

If a genuinely new "check + log + defer" shape is needed, add a util helper in
`internal/util/` rather than writing `_ =` at the call site.
