package k8sobserver

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SourceDisplayName returns the source name or a default if empty.
func SourceDisplayName(c SourceConfig) string {
	if c.Name != "" {
		return c.Name
	}
	return "kubernetes-default"
}

// SourceConfig is the configuration for a single Kubernetes observation source.
type SourceConfig struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"` // must be "kubernetes"
	Enabled    bool     `yaml:"enabled"`
	Kubeconfig string   `yaml:"kubeconfig,omitempty"`
	Namespace  string   `yaml:"namespace,omitempty"`  // single namespace; empty = all
	Namespaces []string `yaml:"namespaces,omitempty"` // optional list filter
	InCluster  bool     `yaml:"in_cluster,omitempty"`
}

// SourcesConfig is the top-level config file for observation sources (sources.yml).
type SourcesConfig struct {
	Sources []SourceConfig `yaml:"sources"`
}

// LoadSourcesConfig loads sources.yml from the given path.
func LoadSourcesConfig(path string) (*SourcesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg SourcesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse sources config: %w", err)
	}
	return &cfg, nil
}

// SaveSourcesConfig writes sources.yml to the given path.
func SaveSourcesConfig(path string, cfg *SourcesConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// K8sObserverConfig is the runtime config for the Kubernetes observer (state dir, cluster id, etc.).
type K8sObserverConfig struct {
	SourceConfig SourceConfig
	StateDir     string // e.g. ~/.beacon/state/<project>/k8s
	ClusterID    string
}
