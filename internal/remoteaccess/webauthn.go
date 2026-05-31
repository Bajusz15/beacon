package remoteaccess

import (
	"bytes"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

// This file is the agent-side WebAuthn verifier. The agent is the relying-party
// verifier: it stores only public keys and checks registration/assertion
// ceremonies relayed (opaquely) by the cloud. rpID and origin are pinned to the
// values captured at enrollment, so a malicious relay cannot present a ceremony
// from a different origin. User verification (biometric/PIN) is required.

// allowedCredParams are the COSE algorithms accepted at registration.
var allowedCredParams = []protocol.CredentialParameter{
	{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgES256},
	{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgRS256},
	{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgEdDSA},
}

// RegistrationResult is the public material extracted from a verified passkey
// registration ceremony.
type RegistrationResult struct {
	CredentialID []byte // raw credential id
	PublicKey    []byte // COSE public key
	SignCount    uint32
}

// VerifyRegistration parses and verifies a WebAuthn registration response
// (navigator.credentials.create) against the agent-issued challenge, pinned
// rpID, and allowed origins. It requires user presence and user verification.
func VerifyRegistration(responseJSON []byte, challengeB64URL, rpID string, origins []string) (*RegistrationResult, error) {
	pcc, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(responseJSON))
	if err != nil {
		return nil, fmt.Errorf("parse registration: %w", err)
	}
	if _, err := pcc.Verify(
		challengeB64URL,
		rpID,
		origins,
		nil, // rpTopOrigins
		protocol.TopOriginImplicitVerificationMode,
		false, // allowCrossOrigin
		true,  // verifyUser (UV required)
		true,  // verifyUserPresence
		nil,   // metadata provider (no attestation trust chain required)
		allowedCredParams,
	); err != nil {
		return nil, fmt.Errorf("verify registration: %w", err)
	}
	att := pcc.Response.AttestationObject.AuthData
	if len(att.AttData.CredentialID) == 0 || len(att.AttData.CredentialPublicKey) == 0 {
		return nil, fmt.Errorf("registration missing credential material")
	}
	return &RegistrationResult{
		CredentialID: att.AttData.CredentialID,
		PublicKey:    att.AttData.CredentialPublicKey,
		SignCount:    att.Counter,
	}, nil
}

// VerifyAssertion parses and verifies a WebAuthn assertion response
// (navigator.credentials.get) against the agent-issued challenge, pinned rpID,
// allowed origins, and the stored COSE public key. It requires user presence and
// user verification, and enforces a monotonic signature counter (clone
// detection) when the authenticator uses one. Returns the new counter to persist.
func VerifyAssertion(responseJSON []byte, challengeB64URL, rpID string, origins []string, publicKeyCOSE []byte, storedSignCount uint32) (uint32, error) {
	par, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(responseJSON))
	if err != nil {
		return 0, fmt.Errorf("parse assertion: %w", err)
	}
	if err := par.Verify(
		challengeB64URL,
		rpID,
		"", // appID (legacy U2F)
		origins,
		nil, // rpTopOrigins
		protocol.TopOriginImplicitVerificationMode,
		false, // allowCrossOrigin
		true,  // verifyUser (UV required)
		true,  // verifyUserPresence
		publicKeyCOSE,
	); err != nil {
		return 0, fmt.Errorf("verify assertion: %w", err)
	}
	newCount := par.Response.AuthenticatorData.Counter
	// A counter of 0 on both sides means the authenticator does not implement a
	// signature counter; otherwise it must strictly increase (clone detection).
	counterImplemented := newCount != 0 || storedSignCount != 0
	if counterImplemented && newCount <= storedSignCount {
		return 0, fmt.Errorf("assertion sign count did not increase (possible cloned authenticator)")
	}
	return newCount, nil
}
