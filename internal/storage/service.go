// Package storage implements Beacon's read-only local filesystem gateway.
package storage

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"beacon/internal/identity"
)

var (
	ErrNotConfigured = errors.New("storage not configured")
	ErrShareNotFound = errors.New("storage share not found")
	ErrForbidden     = errors.New("storage path forbidden")
	ErrIsDirectory   = errors.New("storage path is a directory")
)

// Service exposes configured storage shares through path-safe filesystem operations.
type Service struct {
	cfg *identity.StorageConfig
}

// Share describes one configured share without exposing its local root path.
type Share struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ReadOnly bool   `json:"read_only"`
}

// Entry describes one file or directory.
type Entry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        string    `json:"type"`
	Size        int64     `json:"size,omitempty"`
	ModTime     time.Time `json:"modified_at"`
	ContentType string    `json:"content_type,omitempty"`
}

// ListResponse is returned for directory listings.
type ListResponse struct {
	Share   Share   `json:"share"`
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

// MetadataResponse is returned for file or directory metadata requests.
type MetadataResponse struct {
	Share Share `json:"share"`
	Entry Entry `json:"entry"`
}

// FileResponse is an opened file ready for http.ServeContent.
type FileResponse struct {
	Share       Share
	Path        string
	Name        string
	ModTime     time.Time
	ContentType string
	File        *os.File
}

type resolvedPath struct {
	share identity.StorageShareConfig
	root  string
	rel   string
	real  string
	info  os.FileInfo
}

// NewService constructs a storage service for the current config snapshot.
func NewService(cfg *identity.StorageConfig) *Service {
	return &Service{cfg: cfg}
}

// Shares returns enabled shares. Local root paths are intentionally omitted.
func (s *Service) Shares() ([]Share, error) {
	if s == nil || s.cfg == nil || !s.cfg.Enabled {
		return nil, ErrNotConfigured
	}
	shares := make([]Share, 0, len(s.cfg.Shares))
	for _, share := range s.cfg.Shares {
		if !shareEnabled(share) || strings.TrimSpace(share.ID) == "" || strings.TrimSpace(share.Root) == "" {
			continue
		}
		shares = append(shares, shareView(share))
	}
	sort.Slice(shares, func(i, j int) bool { return shares[i].ID < shares[j].ID })
	return shares, nil
}

// List returns entries for a directory within a share.
func (s *Service) List(shareID, requestedPath string) (*ListResponse, error) {
	rp, err := s.resolve(shareID, requestedPath)
	if err != nil {
		return nil, err
	}
	if !rp.info.IsDir() {
		return nil, ErrIsDirectory
	}
	entries, err := os.ReadDir(rp.real)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, de := range entries {
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		childRel := joinRel(rp.rel, de.Name())
		out = append(out, entryFromInfo(de.Name(), childRel, info))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "directory"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return &ListResponse{Share: shareView(rp.share), Path: responsePath(rp.rel), Entries: out}, nil
}

// Metadata returns metadata for a file or directory within a share.
func (s *Service) Metadata(shareID, requestedPath string) (*MetadataResponse, error) {
	rp, err := s.resolve(shareID, requestedPath)
	if err != nil {
		return nil, err
	}
	return &MetadataResponse{
		Share: shareView(rp.share),
		Entry: entryFromInfo(path.Base(responsePath(rp.rel)), rp.rel, rp.info),
	}, nil
}

// Open opens a regular file for download.
func (s *Service) Open(shareID, requestedPath string) (*FileResponse, error) {
	rp, err := s.resolve(shareID, requestedPath)
	if err != nil {
		return nil, err
	}
	if rp.info.IsDir() {
		return nil, ErrIsDirectory
	}
	f, err := os.Open(rp.real)
	if err != nil {
		return nil, err
	}
	name := path.Base(responsePath(rp.rel))
	return &FileResponse{
		Share:       shareView(rp.share),
		Path:        responsePath(rp.rel),
		Name:        name,
		ModTime:     rp.info.ModTime(),
		ContentType: contentType(name, rp.info),
		File:        f,
	}, nil
}

func (s *Service) resolve(shareID, requestedPath string) (*resolvedPath, error) {
	share, err := s.findShare(shareID)
	if err != nil {
		return nil, err
	}
	rel, err := cleanSharePath(requestedPath)
	if err != nil {
		return nil, err
	}
	root, err := canonicalRoot(share.Root)
	if err != nil {
		return nil, err
	}
	target := root
	if rel != "." {
		target = filepath.Join(root, filepath.FromSlash(rel))
	}
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if !insideRoot(root, real) {
		return nil, ErrForbidden
	}
	info, err := os.Stat(real)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return &resolvedPath{share: share, root: root, rel: rel, real: real, info: info}, nil
}

func (s *Service) findShare(id string) (identity.StorageShareConfig, error) {
	if s == nil || s.cfg == nil || !s.cfg.Enabled {
		return identity.StorageShareConfig{}, ErrNotConfigured
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return identity.StorageShareConfig{}, ErrShareNotFound
	}
	for _, share := range s.cfg.Shares {
		if strings.TrimSpace(share.ID) != id {
			continue
		}
		if !shareEnabled(share) {
			return identity.StorageShareConfig{}, ErrShareNotFound
		}
		if strings.TrimSpace(share.Root) == "" {
			return identity.StorageShareConfig{}, ErrShareNotFound
		}
		return share, nil
	}
	return identity.StorageShareConfig{}, ErrShareNotFound
}

func cleanSharePath(p string) (string, error) {
	if p == "" || p == "." {
		return ".", nil
	}
	if strings.Contains(p, "\x00") || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") {
		return "", ErrForbidden
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", ErrForbidden
		}
	}
	return path.Clean(p), nil
}

func canonicalRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", ErrShareNotFound
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("storage share root is not a directory")
	}
	return real, nil
}

func insideRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func shareEnabled(share identity.StorageShareConfig) bool {
	return share.Enabled == nil || *share.Enabled
}

func shareReadOnly(share identity.StorageShareConfig) bool {
	return share.ReadOnly == nil || *share.ReadOnly
}

func shareView(share identity.StorageShareConfig) Share {
	name := strings.TrimSpace(share.Name)
	if name == "" {
		name = strings.TrimSpace(share.ID)
	}
	return Share{ID: strings.TrimSpace(share.ID), Name: name, ReadOnly: shareReadOnly(share)}
}

func entryFromInfo(name, rel string, info os.FileInfo) Entry {
	typ := "file"
	switch {
	case info.IsDir():
		typ = "directory"
	case info.Mode()&os.ModeSymlink != 0:
		typ = "symlink"
	}
	entry := Entry{
		Name:    name,
		Path:    responsePath(rel),
		Type:    typ,
		ModTime: info.ModTime(),
	}
	if !info.IsDir() {
		entry.Size = info.Size()
		entry.ContentType = contentType(name, info)
	}
	return entry
}

func contentType(name string, info os.FileInfo) string {
	if info.IsDir() {
		return ""
	}
	if typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); typ != "" {
		return typ
	}
	return "application/octet-stream"
}

func responsePath(rel string) string {
	if rel == "." || rel == "" {
		return ""
	}
	return filepath.ToSlash(rel)
}

func joinRel(parent, name string) string {
	if parent == "." || parent == "" {
		return name
	}
	return path.Join(parent, name)
}
