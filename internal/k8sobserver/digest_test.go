package k8sobserver

import (
	"testing"
)

func TestParseImageID(t *testing.T) {
	tests := []struct {
		imageID      string
		wantImage    string
		wantDigest   string
	}{
		{
			"docker-pullable://docker.io/library/nginx@sha256:abc123",
			"docker.io/library/nginx@sha256:abc123",
			"sha256:abc123",
		},
		{
			"ghcr.io/myorg/app@sha256:deadbeef",
			"ghcr.io/myorg/app@sha256:deadbeef",
			"sha256:deadbeef",
		},
		{"", "", ""},
		{"nginx:latest", "nginx:latest", ""},
	}
	for _, tt := range tests {
		img, d := ParseImageID(tt.imageID)
		if img != tt.wantImage || d != tt.wantDigest {
			t.Errorf("ParseImageID(%q) = (%q, %q), want (%q, %q)", tt.imageID, img, d, tt.wantImage, tt.wantDigest)
		}
	}
}

func TestImageTag(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"myapp:v1", "v1"},
		{"myapp@sha256:abc", ""},
		{"myapp", ""},
	}
	for _, tt := range tests {
		got := ImageTag(tt.ref)
		if got != tt.want {
			t.Errorf("ImageTag(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
