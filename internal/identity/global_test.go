package identity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGlobalSettings_HeartbeatDuration(t *testing.T) {
	g := GlobalSettings{HeartbeatInterval: "90s"}
	require.Equal(t, 90*time.Second, g.HeartbeatDuration())

	g2 := GlobalSettings{HeartbeatInterval: "nope"}
	require.Equal(t, 60*time.Second, g2.HeartbeatDuration())
}
