package cli

import (
	"path/filepath"
	"testing"
)

func TestLoadConfig_missing(t *testing.T) {
	t.Setenv("MILO_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CurrentContext != "" {
		t.Errorf("expected empty current_context, got %q", cfg.CurrentContext)
	}
	if len(cfg.Contexts) != 0 {
		t.Errorf("expected empty contexts map, got %v", cfg.Contexts)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("MILO_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))

	original := &Config{
		CurrentContext: "prod",
		Contexts: map[string]Context{
			"prod": {Endpoint: "https://milo.prod.example.com", Token: "tok-prod"},
			"dev":  {Endpoint: "http://localhost:8000", Token: "tok-dev"},
		},
	}
	if err := SaveConfig(original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.CurrentContext != original.CurrentContext {
		t.Errorf("current_context: want %q, got %q", original.CurrentContext, loaded.CurrentContext)
	}
	for name, want := range original.Contexts {
		got, ok := loaded.Contexts[name]
		if !ok {
			t.Errorf("context %q missing after round-trip", name)
			continue
		}
		if got.Endpoint != want.Endpoint {
			t.Errorf("context %q endpoint: want %q, got %q", name, want.Endpoint, got.Endpoint)
		}
		if got.Token != want.Token {
			t.Errorf("context %q token: want %q, got %q", name, want.Token, got.Token)
		}
	}
}

func TestActive(t *testing.T) {
	t.Setenv("MILO_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))

	cfg := &Config{
		CurrentContext: "staging",
		Contexts: map[string]Context{
			"staging": {Endpoint: "https://staging.example.com", Token: "tok-stg"},
		},
	}

	ctx, ok := cfg.Active()
	if !ok {
		t.Fatal("expected Active to return ok=true")
	}
	if ctx.Endpoint != "https://staging.example.com" {
		t.Errorf("wrong endpoint: %q", ctx.Endpoint)
	}

	// Missing context
	cfg.CurrentContext = "nonexistent"
	_, ok = cfg.Active()
	if ok {
		t.Fatal("expected Active to return ok=false for nonexistent context")
	}
}

func TestMultiContext(t *testing.T) {
	t.Setenv("MILO_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))

	cfg, _ := LoadConfig()
	cfg.Contexts["a"] = Context{Endpoint: "https://a.example.com", Token: "token-a"}
	cfg.Contexts["b"] = Context{Endpoint: "https://b.example.com", Token: "token-b"}
	cfg.CurrentContext = "a"
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, _ := LoadConfig()
	if len(loaded.Contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(loaded.Contexts))
	}
	if loaded.CurrentContext != "a" {
		t.Errorf("expected current=a, got %q", loaded.CurrentContext)
	}

	// Switch context
	loaded.CurrentContext = "b"
	_ = SaveConfig(loaded)

	reloaded, _ := LoadConfig()
	if reloaded.CurrentContext != "b" {
		t.Errorf("expected current=b after switch, got %q", reloaded.CurrentContext)
	}
}
