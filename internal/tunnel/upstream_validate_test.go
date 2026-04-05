package tunnel

import "testing"

func TestValidateDialTarget_ok(t *testing.T) {
	cases := []struct {
		proto, host string
		port        int
	}{
		{"http", "127.0.0.1", 80},
		{"http", "192.168.1.1", 8080},
		{"https", "10.0.0.5", 443},
		{"http", "homeassistant", 8123},
		{"http", "localhost", 3000},
	}
	for _, c := range cases {
		if err := ValidateDialTarget(c.proto, c.host, c.port); err != nil {
			t.Errorf("%s %s %d: %v", c.proto, c.host, c.port, err)
		}
	}
}

func TestValidateDialTarget_reject(t *testing.T) {
	if err := ValidateDialTarget("http", "8.8.8.8", 53); err == nil {
		t.Fatal("expected error for public IP")
	}
	if err := ValidateDialTarget("http", "169.254.169.254", 80); err == nil {
		t.Fatal("expected error for metadata IP")
	}
}
