package k8sobserver

import (
	"strings"
)

// ParseImageID extracts image reference and digest from Kubernetes container status imageID.
// imageID is typically "docker-pullable://registry/repo@sha256:..." or "registry/repo@sha256:...".
// Returns (full image ref with digest, digest only). Digest may be empty if not present.
func ParseImageID(imageID string) (image string, digest string) {
	s := strings.TrimPrefix(imageID, "docker-pullable://")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if idx := strings.LastIndex(s, "@"); idx != -1 {
		digest = s[idx+1:]
		image = s
		return image, digest
	}
	return s, ""
}

// ImageTag returns the tag part of an image reference (e.g. "myapp:v1" -> "v1").
// If no tag, returns empty.
func ImageTag(imageRef string) string {
	if idx := strings.LastIndex(imageRef, ":"); idx != -1 {
		tag := imageRef[idx+1:]
		if strings.Contains(tag, "@") {
			return "" // digest only
		}
		return tag
	}
	return ""
}
