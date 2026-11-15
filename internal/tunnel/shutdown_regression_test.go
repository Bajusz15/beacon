package tunnel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTunnelRegressionRetryExhaustionStopsHealthWorker(t *testing.T) {
	dir := t.TempDir()
	// Simulated days of backoff must not cause hundreds of thousands of writes.
	// Health persistence errors are tolerated by the client independently of Run.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "health.json.tmp"), 0700))
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client := NewClient(ClientConfig{
			TunnelID: "retry-exhaustion", IPCDir: dir,
			CloudURL: "://invalid", // Fails parsing before any real network I/O.
		})
		done := make(chan error, 1)
		go func() { done <- client.Run(ctx) }()
		select {
		case err := <-done:
			require.NoError(t, ctx.Err(), "Run must return without parent cancellation")
			require.ErrorContains(t, err, "max reconnects exceeded")
		case <-time.After(time.Duration(maxReconnects+1) * maxBackoff):
			// Unblock a broken implementation so failure doesn't leak its workers.
			cancel()
			<-done
			t.Fatal("Run hung after retry exhaustion while the parent context remained active")
		}
	})
}
