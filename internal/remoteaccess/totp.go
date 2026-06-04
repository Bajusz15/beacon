package remoteaccess

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP/HOTP are defined over HMAC-SHA1 (RFC 6238/4226); authenticator apps default to it.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// This file is a self-contained TOTP (RFC 6238, built on HOTP RFC 4226)
// implementation used as the out-of-band approval factor. The secret is
// provisioned once on the device (shown as an otpauth:// QR in the CLI / add-on
// log) and scanned into the user's authenticator app. At unlock the user reads
// the current 6-digit code; the agent verifies it locally against the stored
// secret. The cloud never sees the secret or any code.
//
// Authenticator apps (Google Authenticator, 1Password, Authy, …) default to
// HMAC-SHA1, 6 digits, and a 30-second period, so those are the fixed parameters
// here for maximum compatibility.

const (
	totpDigits    = 6
	totpPeriod    = 30 * time.Second
	totpSecretLen = 20 // 160-bit secret, the RFC 4226 recommended length for SHA1.
	// totpSkew is how many periods on either side of "now" are accepted, to
	// tolerate clock drift between the device and the user's phone.
	totpSkew = 1
)

// totpBase32 is RFC 4648 base32 without padding — the encoding authenticator
// apps expect in the otpauth:// secret parameter.
var totpBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret returns a fresh base32-encoded TOTP secret.
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretLen)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return totpBase32.EncodeToString(buf), nil
}

// OTPAuthURL builds the otpauth://totp/ URI that an authenticator app parses
// (typically rendered as a QR code). account labels the entry (e.g. the device
// name); issuer groups it under "Beacon".
func OTPAuthURL(secret, account, issuer string) string {
	label := issuer + ":" + account
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", int(totpPeriod.Seconds())))
	return "otpauth://totp/" + url.PathEscape(label) + "?" + q.Encode()
}

// hotp computes the HOTP value (RFC 4226) for a counter under the given key.
func hotp(key []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, bin%mod)
}

// verifyTOTP checks code against the secret at time t, accepting ±totpSkew
// periods of clock drift. It returns the matched counter (the period index) and
// true on success, so the caller can enforce single-use by rejecting a counter
// it has already accepted. The code comparison is constant-time.
func verifyTOTP(secret, code string, t time.Time) (uint64, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0, false
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return 0, false
	}
	center := uint64(t.Unix()) / uint64(totpPeriod.Seconds())
	for d := -totpSkew; d <= totpSkew; d++ {
		counter := center
		if d < 0 {
			counter -= uint64(-d)
		} else {
			counter += uint64(d)
		}
		want := hotp(key, counter)
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return counter, true
		}
	}
	return 0, false
}

// decodeTOTPSecret decodes a base32 secret, tolerating padding and lower case so
// that secrets copied from various authenticator apps still validate.
func decodeTOTPSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimRight(s, "=")
	return totpBase32.DecodeString(s)
}
