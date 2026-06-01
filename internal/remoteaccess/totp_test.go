package remoteaccess

import (
	"strings"
	"testing"
	"time"
)

// TestHOTPRFC4226Vectors checks the HOTP values from RFC 4226 Appendix D, which
// pin the HMAC-SHA1 + dynamic-truncation implementation to the standard.
func TestHOTPRFC4226Vectors(t *testing.T) {
	key := []byte("12345678901234567890") // the RFC's ASCII secret "Secret"
	want := []string{
		"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489",
	}
	for counter, w := range want {
		if got := hotp(key, uint64(counter)); got != w {
			t.Errorf("hotp counter %d = %s, want %s", counter, got, w)
		}
	}
}

// TestVerifyTOTPRoundTrip verifies a freshly generated secret validates its own
// current code and rejects a wrong one.
func TestVerifyTOTPRoundTrip(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatalf("decodeTOTPSecret: %v", err)
	}
	now := time.Now()
	counter := uint64(now.Unix()) / uint64(totpPeriod.Seconds())
	code := hotp(key, counter)

	got, ok := verifyTOTP(secret, code, now)
	if !ok || got != counter {
		t.Fatalf("verifyTOTP = (%d,%v), want (%d,true)", got, ok, counter)
	}
	if _, ok := verifyTOTP(secret, "000000", now); ok {
		t.Fatal("verifyTOTP accepted a wrong code")
	}
	if _, ok := verifyTOTP(secret, "", now); ok {
		t.Fatal("verifyTOTP accepted an empty code")
	}
}

// TestVerifyTOTPSkewWindow accepts codes from the adjacent periods (clock drift)
// but rejects ones two periods away.
func TestVerifyTOTPSkewWindow(t *testing.T) {
	secret, _ := GenerateTOTPSecret()
	key, _ := decodeTOTPSecret(secret)
	now := time.Now()
	period := uint64(totpPeriod.Seconds())
	center := uint64(now.Unix()) / period

	for _, d := range []int64{-1, 0, 1} {
		code := hotp(key, uint64(int64(center)+d))
		if _, ok := verifyTOTP(secret, code, now); !ok {
			t.Errorf("expected code at offset %d within skew to verify", d)
		}
	}
	for _, d := range []int64{-2, 2} {
		code := hotp(key, uint64(int64(center)+d))
		if _, ok := verifyTOTP(secret, code, now); ok {
			t.Errorf("expected code at offset %d outside skew to be rejected", d)
		}
	}
}

// TestDecodeTOTPSecretTolerance accepts lower case, spaces, and padding so codes
// copied from various authenticator apps still validate.
func TestDecodeTOTPSecretTolerance(t *testing.T) {
	secret, _ := GenerateTOTPSecret()
	variants := []string{
		strings.ToLower(secret),
		secret + "===",
		"  " + secret + "  ",
	}
	want, err := decodeTOTPSecret(secret)
	if err != nil {
		t.Fatalf("decode base: %v", err)
	}
	for _, v := range variants {
		got, err := decodeTOTPSecret(v)
		if err != nil {
			t.Errorf("decode %q: %v", v, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("decode %q mismatch", v)
		}
	}
}

// TestOTPAuthURL embeds the secret and standard parameters an authenticator app
// expects.
func TestOTPAuthURL(t *testing.T) {
	url := OTPAuthURL("ABC234", "my-device", "Beacon")
	for _, want := range []string{
		"otpauth://totp/", "secret=ABC234", "issuer=Beacon",
		"algorithm=SHA1", "digits=6", "period=30",
	} {
		if !strings.Contains(url, want) {
			t.Errorf("otpauth url %q missing %q", url, want)
		}
	}
}
