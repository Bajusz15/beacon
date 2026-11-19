package cloud

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeGatewayURL(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"https://access.example.com", "https://access.example.com/api"},
		{" https://access.example.com/api/ ", "https://access.example.com/api"},
		{"http://localhost:7070", "http://localhost:7070/api"},
		{"http://[::1]:7070/api", "http://[::1]:7070/api"},
	} {
		got, err := NormalizeGatewayURL(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("%q: got %q, %v", tt.input, got, err)
		}
	}
	for _, raw := range []string{"", "access.example.com", "http://access.example.com", "ftp://localhost", "https://user:secret@example.com", "https://example.com?key=secret", "https://example.com#fragment", "https://example.com/other", "http://localhost.evil.test", "https://example.com?"} {
		if _, err := NormalizeGatewayURL(raw); err == nil {
			t.Errorf("accepted unsafe gateway %q", raw)
		}
	}
}

func TestGatewayRedirectDoesNotForwardCredentials(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	request, err := http.NewRequest(http.MethodGet, source.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-API-Key", "test-key")
	client := &http.Client{CheckRedirect: RejectGatewayRedirect}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if called || response.StatusCode != http.StatusTemporaryRedirect {
		t.Fatal("gateway redirected a credential-bearing request")
	}
}
