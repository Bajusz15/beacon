package tunnel

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildLoopbackURL(t *testing.T) {
	u, err := buildLoopbackURL("http", 8123, "/api/v1?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "http" || u.Host != "127.0.0.1:8123" || u.Path != "/api/v1" || u.RawQuery != "x=1" {
		t.Fatalf("unexpected URL: %s", u.String())
	}
}

func TestBuildLoopbackURL_rejectsSSRFPatterns(t *testing.T) {
	bad := []string{
		"http://evil.test/",
		"//evil.test/",
		`\\evil`,
		"@evil",
		"/foo\nbar",
		`/foo\bar`,
	}
	for _, p := range bad {
		_, err := buildLoopbackURL("http", 80, p)
		if err == nil {
			t.Fatalf("expected error for path %q", p)
		}
	}
}

func TestBuildLoopbackURL_ws(t *testing.T) {
	u, err := buildLoopbackURL("ws", 3000, "/socket")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u.String(), "ws://127.0.0.1:3000/socket") {
		t.Fatalf("got %s", u.String())
	}
}

func TestBuildUpstreamURL(t *testing.T) {
	u, err := buildUpstreamURL("http", "homeassistant", 8123, "/?refresh=1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "http" || u.Host != "homeassistant:8123" || u.Path != "/" || u.RawQuery != "refresh=1" {
		t.Fatalf("unexpected URL: %s", u.String())
	}
}

func TestValidHTTPMethod(t *testing.T) {
	if !validHTTPMethod("GET") || !validHTTPMethod("post") {
		t.Fatal("expected valid")
	}
	if validHTTPMethod("TRACE") || validHTTPMethod("CONNECT") {
		t.Fatal("expected invalid")
	}
}

func FuzzBuildLoopbackURL(f *testing.F) {
	f.Add("http", 8080, "/")
	f.Add("ws", 1, "/a?b=c")
	f.Fuzz(func(t *testing.T, scheme string, port int, p string) {
		if port < 1 || port > 65535 {
			t.Skip()
		}
		u, err := buildLoopbackURL(scheme, port, p)
		if err != nil {
			return
		}
		if u.Hostname() != "127.0.0.1" {
			t.Fatalf("host leak: %s", u.String())
		}
		pu, err := url.Parse(u.String())
		if err != nil {
			t.Fatal(err)
		}
		if pu.Hostname() != "127.0.0.1" {
			t.Fatalf("parse host leak: %s", u.String())
		}
	})
}
