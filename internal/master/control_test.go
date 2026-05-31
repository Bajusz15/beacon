package master

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"beacon/internal/remoteaccess"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"
)

func TestIsRemoteAccessControlAction(t *testing.T) {
	for _, action := range []string{
		actionRemoteAccessChallenge,
		actionRemoteAccessUnlock,
		actionRemoteAccessPasskeyChallenge,
		actionRemoteAccessPasskeyUnlock,
		actionRemoteAccessEnrollBegin,
		actionRemoteAccessEnrollFinish,
	} {
		require.True(t, isRemoteAccessControlAction(action), action)
	}
	require.False(t, isRemoteAccessControlAction("restart"))
	require.False(t, isRemoteAccessControlAction(""))
}

func TestControlWSBase(t *testing.T) {
	require.Equal(t, "wss://beaconinfra.dev/api", controlWSBase("https://beaconinfra.dev/api/"))
	require.Equal(t, "ws://localhost:8080/api", controlWSBase("http://localhost:8080/api"))
	require.Equal(t, "ws://beaconinfra.dev/api", controlWSBase("beaconinfra.dev/api"))
}

func readRemoteAccessControlReply(t *testing.T, dispatcher *CommandDispatcher, cmd HeartbeatCommand) controlReply {
	t.Helper()
	var upgrader websocket.Upgrader
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer close(done)
		defer conn.Close()
		var mu sync.Mutex
		handleRemoteAccessControl(conn, &mu, dispatcher, cmd)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	var reply controlReply
	require.NoError(t, conn.ReadJSON(&reply))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server did not finish")
	}
	return reply
}

func TestHandleRemoteAccessControlPassphraseChallengeAndUnlock(t *testing.T) {
	const passphrase = "correct horse battery"
	t.Setenv("BEACON_HOME", t.TempDir())
	require.NoError(t, remoteaccess.SetPassphrase(passphrase))
	dispatcher := NewCommandDispatcher(nil, nil)

	challengeReply := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-challenge",
		Action:  actionRemoteAccessChallenge,
		Payload: map[string]any{"session_id": "sess-1"},
	})
	require.True(t, challengeReply.OK)
	require.Equal(t, "cmd-challenge", challengeReply.ReplyTo)
	require.Equal(t, actionRemoteAccessChallenge, challengeReply.Type)
	require.Equal(t, actionTerminalOpen, challengeReply.Data["action"])

	challengeJSON, err := json.Marshal(challengeReply.Data)
	require.NoError(t, err)
	var ch remoteaccess.ChallengeResult
	require.NoError(t, json.Unmarshal(challengeJSON, &ch))
	proof := browserProofForControl(t, passphrase, &ch, actionTerminalOpen, "sess-1")

	unlockReply := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:     "cmd-unlock",
		Action: actionRemoteAccessUnlock,
		Payload: map[string]any{
			"session_id": "sess-1",
			"nonce":      ch.Nonce,
			"proof":      proof,
		},
	})
	require.True(t, unlockReply.OK)
	require.Empty(t, unlockReply.Error)
	require.True(t, dispatcher.Grants().Consume(actionTerminalOpen, "sess-1"))
}

func TestHandleRemoteAccessControlErrors(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	dispatcher := NewCommandDispatcher(nil, nil)

	missingSession := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-missing",
		Action:  actionRemoteAccessChallenge,
		Payload: map[string]any{},
	})
	require.False(t, missingSession.OK)
	require.Equal(t, "session_id required", missingSession.Error)

	notConfigured := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-passkey",
		Action:  actionRemoteAccessPasskeyChallenge,
		Payload: map[string]any{"session_id": "sess-2"},
	})
	require.False(t, notConfigured.OK)
	require.Equal(t, remoteaccess.ErrNotConfigured.Error(), notConfigured.Error)

	badUnlock := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-unlock",
		Action:  actionRemoteAccessUnlock,
		Payload: map[string]any{"session_id": "sess-2", "nonce": "bad", "proof": "bad"},
	})
	require.False(t, badUnlock.OK)
	require.Equal(t, remoteaccess.ErrNotConfigured.Error(), badUnlock.Error)
}

func TestHandleRemoteAccessControlPasskeyChallengeAndBadUnlock(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	require.NoError(t, remoteaccess.AddCredential(remoteaccess.PasskeyCredential{
		ID:        "cred-1",
		PublicKey: "cHVibGljLWtleQ==",
		RPID:      "beaconinfra.dev",
		Origin:    "https://beaconinfra.dev",
	}))
	dispatcher := NewCommandDispatcher(nil, nil)
	note := &recordingNotifierForControl{}
	dispatcher.Grants().SetNotifier(note)

	challenge := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-passkey",
		Action:  actionRemoteAccessPasskeyChallenge,
		Payload: map[string]any{"session_id": "sess-1", "bind_action": actionTunnelConnect},
	})
	require.True(t, challenge.OK)
	require.Equal(t, actionTunnelConnect, challenge.Data["action"])
	require.Equal(t, "beaconinfra.dev", challenge.Data["rpId"])
	require.NotEmpty(t, challenge.Data["allowCredentials"])
	require.NotEmpty(t, note.code)

	unlock := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:     "cmd-passkey-unlock",
		Action: actionRemoteAccessPasskeyUnlock,
		Payload: map[string]any{
			"session_id": "sess-1",
			"action":     actionTunnelConnect,
			"assertion":  "not-json",
			"oob_code":   note.code,
		},
	})
	require.False(t, unlock.OK)
	require.Equal(t, remoteaccess.ErrBadAssertion.Error(), unlock.Error)
}

func TestHandleRemoteAccessControlEnrollBeginAuthorization(t *testing.T) {
	t.Setenv("BEACON_HOME", t.TempDir())
	dispatcher := NewCommandDispatcher(nil, nil)

	denied := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-denied",
		Action:  actionRemoteAccessEnrollBegin,
		Payload: map[string]any{"session_id": "sess-1", "enroll_code": "000000"},
	})
	require.False(t, denied.OK)
	require.Contains(t, denied.Error, "enrollment not authorized")

	code, err := remoteaccess.SetEnrollToken(remoteaccess.DefaultEnrollTTL)
	require.NoError(t, err)
	allowed := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:     "cmd-allowed",
		Action: actionRemoteAccessEnrollBegin,
		Payload: map[string]any{
			"session_id":  "sess-1",
			"enroll_code": code,
			"rp_id":       "beaconinfra.dev",
			"origin":      "https://beaconinfra.dev",
		},
	})
	require.True(t, allowed.OK)
	require.NotEmpty(t, allowed.Data["challenge"])
	require.Equal(t, "beaconinfra.dev", allowed.Data["rpId"])

	finish := readRemoteAccessControlReply(t, dispatcher, HeartbeatCommand{
		ID:      "cmd-finish",
		Action:  actionRemoteAccessEnrollFinish,
		Payload: map[string]any{"session_id": "sess-1", "response": "not-json", "label": "Laptop"},
	})
	require.False(t, finish.OK)
	require.Contains(t, finish.Error, "parse registration")
}

type recordingNotifierForControl struct {
	code string
}

func (r *recordingNotifierForControl) SendOOBCode(action, code string) error {
	r.code = code
	return nil
}

func browserProofForControl(t *testing.T, passphrase string, ch *remoteaccess.ChallengeResult, action, sessionID string) string {
	t.Helper()
	salt, err := base64.StdEncoding.DecodeString(ch.Salt)
	if err != nil {
		t.Fatalf("decode salt: %v", err)
	}
	key := remoteaccessTestDeriveKey(passphrase, salt, ch.Params)
	return base64.StdEncoding.EncodeToString(remoteaccessTestComputeProof(key, ch.Nonce, action, sessionID))
}

func remoteaccessTestDeriveKey(passphrase string, salt []byte, p remoteaccess.Argon2Params) []byte {
	return argon2.IDKey([]byte(passphrase), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
}

func remoteaccessTestComputeProof(key []byte, nonce, action, sessionID string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(nonce))
	mac.Write([]byte{0})
	mac.Write([]byte(action))
	mac.Write([]byte{0})
	mac.Write([]byte(sessionID))
	return mac.Sum(nil)
}
