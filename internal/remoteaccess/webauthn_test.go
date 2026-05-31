package remoteaccess

import (
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/require"
)

func TestVerifyRegistrationRejectsInvalidJSON(t *testing.T) {
	res, err := VerifyRegistration([]byte(`not-json`), "challenge", "beaconinfra.dev", []string{"https://beaconinfra.dev"})
	require.Nil(t, res)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse registration")
}

func TestVerifyAssertionRejectsInvalidJSON(t *testing.T) {
	count, err := VerifyAssertion([]byte(`not-json`), "challenge", "beaconinfra.dev", []string{"https://beaconinfra.dev"}, []byte("public-key"), 0)
	require.Zero(t, count)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse assertion")
}

func TestAllowedCredentialParams(t *testing.T) {
	require.Len(t, allowedCredParams, 3)
	for _, param := range allowedCredParams {
		require.Equal(t, protocol.PublicKeyCredentialType, param.Type)
	}
}
