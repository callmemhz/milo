package manifest

import (
	"strings"
	"testing"
)

func TestParse_OK(t *testing.T) {
	m, err := Parse([]byte(`
name: my-app
image: ghcr.io/foo/bar:latest
port: 9000
resources:
  cpu: 0.5
  memory: 512
volumes:
  - cpfs://train-fs/datasets/foo:/data:ro
  - cpfs://other-fs/sub:/work
env:
  LOG_LEVEL: info
  FEATURE_X: "1"
`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m.Name != "my-app" {
		t.Errorf("name=%q", m.Name)
	}
	if m.Kind != KindViewer {
		t.Errorf("default kind=%q want %q", m.Kind, KindViewer)
	}
	if m.Image != "ghcr.io/foo/bar:latest" {
		t.Errorf("image=%q", m.Image)
	}
	if m.Port != 9000 {
		t.Errorf("port=%d", m.Port)
	}
	if m.Resources.CPU != 0.5 || m.Resources.Memory != 512 {
		t.Errorf("resources=%+v", m.Resources)
	}
	if m.Env["LOG_LEVEL"] != "info" || m.Env["FEATURE_X"] != "1" {
		t.Errorf("env=%+v", m.Env)
	}
	specs := m.VolumeSpecs()
	if len(specs) != 2 {
		t.Fatalf("specs len=%d", len(specs))
	}
	if !specs[0].ReadOnly || specs[0].Target != "/data" {
		t.Errorf("specs[0]=%+v", specs[0])
	}
	if specs[1].ReadOnly || specs[1].Target != "/work" {
		t.Errorf("specs[1]=%+v", specs[1])
	}
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{"missing name", `image: x:y`, "name is required"},
		{"missing image", `name: a`, "image is required"},
		{"unknown kind", "name: a\nimage: x:y\nkind: job", "unsupported kind"},
		{"unknown field", "name: a\nimage: x:y\nfoo: bar", "field foo not found"},
		{"bad volume", "name: a\nimage: x:y\nvolumes: [\"oss://b/p:/x\"]", "unsupported scheme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}
