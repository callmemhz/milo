// Package manifest reads and validates the milo.yaml manifest that describes
// an app. The manifest is the source of truth for `milo apply`: it carries
// both long-lived app config (port, resources, volumes) and the deployment
// directive (image to roll out).
package manifest

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/callmemhz/milo/internal/volumes"
	"github.com/callmemhz/milo/pkg/api"
)

const (
	KindViewer = "viewer"
)

// Manifest mirrors the on-disk milo.yaml.
//
// Phase 1 supports:
//
//	name: my-app
//	kind: viewer
//	image: ghcr.io/owner/repo:latest
//	port: 8080
//	resources:
//	  cpu: 0.5
//	  memory: 512        # MB
//	volumes:
//	  - cpfs://train-fs/datasets/foo:/data:ro
type Manifest struct {
	Name      string            `yaml:"name"`
	Kind      string            `yaml:"kind"`
	Image     string            `yaml:"image"`
	Port      int64             `yaml:"port,omitempty"`
	Resources Resources         `yaml:"resources,omitempty"`
	Volumes   []string          `yaml:"volumes,omitempty"`
	// Env declares non-secret environment variables. Apply overlays these
	// onto the app's existing env (additive: vars not in the manifest are
	// preserved, so secrets set via `milo env set` survive). To unset a
	// var, use `milo env unset` directly.
	Env map[string]string `yaml:"env,omitempty"`
}

type Resources struct {
	CPU    float64 `yaml:"cpu,omitempty"`
	Memory int64   `yaml:"memory,omitempty"` // MiB
}

// Load reads and parses the manifest at path.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes raw YAML bytes into a Manifest, applying defaults and
// validating semantic invariants.
func Parse(b []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true) // reject unknown keys: cheap typo detector
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("milo.yaml: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) validate() error {
	if m.Name == "" {
		return fmt.Errorf("milo.yaml: name is required")
	}
	if m.Kind == "" {
		m.Kind = KindViewer
	}
	if m.Kind != KindViewer {
		return fmt.Errorf("milo.yaml: unsupported kind %q (phase 1: only %q)", m.Kind, KindViewer)
	}
	if m.Image == "" {
		return fmt.Errorf("milo.yaml: image is required")
	}
	for i, v := range m.Volumes {
		if _, err := volumes.ParseString(v); err != nil {
			return fmt.Errorf("milo.yaml: volumes[%d]: %w", i, err)
		}
	}
	return nil
}

// VolumeSpecs returns the parsed VolumeSpec list. Safe to call after a
// successful Parse — validate() has already verified each entry.
func (m *Manifest) VolumeSpecs() []api.VolumeSpec {
	if len(m.Volumes) == 0 {
		return nil
	}
	out := make([]api.VolumeSpec, 0, len(m.Volumes))
	for _, s := range m.Volumes {
		spec, _ := volumes.ParseString(s)
		out = append(out, spec)
	}
	return out
}

