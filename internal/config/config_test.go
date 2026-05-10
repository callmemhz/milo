package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ROOT_DOMAIN", "app.example.com")
	t.Setenv("API_DOMAIN", "milo.example.com")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8000" {
		t.Fatalf("HTTPAddr: %q", c.HTTPAddr)
	}
	if c.Network != "milo-net" {
		t.Fatalf("Network: %q", c.Network)
	}
	if c.Version != "dev" {
		t.Fatalf("Version: %q", c.Version)
	}
}

func TestLoadRequiresRootDomain(t *testing.T) {
	os.Unsetenv("ROOT_DOMAIN")
	os.Unsetenv("API_DOMAIN")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when ROOT_DOMAIN/API_DOMAIN missing")
	}
}
