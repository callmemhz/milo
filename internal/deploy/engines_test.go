package deploy

import (
	"strings"
	"testing"
)

func TestLookupEngine(t *testing.T) {
	e, v, err := LookupEngine("postgres", "")
	if err != nil || v != "16" || e.Images[v] != "postgres:16-alpine" {
		t.Fatalf("default version: %v %q", err, v)
	}
	if _, _, err := LookupEngine("postgres", "13"); err == nil {
		t.Fatal("expected unsupported version error")
	}
	if _, _, err := LookupEngine("mysql", ""); err == nil {
		t.Fatal("expected unknown engine error")
	}
	if _, v, _ := LookupEngine("redis", "7"); v != "7" {
		t.Fatalf("explicit version: %q", v)
	}
}

func TestLinkEnvKey(t *testing.T) {
	if k := LinkEnvKey("postgres", ""); k != "DATABASE_URL" {
		t.Fatalf("got %q", k)
	}
	if k := LinkEnvKey("redis", ""); k != "REDIS_URL" {
		t.Fatalf("got %q", k)
	}
	if k := LinkEnvKey("postgres", "CACHE"); k != "CACHE_URL" {
		t.Fatalf("got %q", k)
	}
}

func TestConnectionURL(t *testing.T) {
	pg := ConnectionURL("postgres", "mydb", "s3cret")
	if pg != "postgres://app:s3cret@mydb:5432/app?sslmode=disable" {
		t.Fatalf("got %q", pg)
	}
	rd := ConnectionURL("redis", "cache", "s3cret")
	if rd != "redis://:s3cret@cache:6379/0" {
		t.Fatalf("got %q", rd)
	}
}

func TestGeneratePassword(t *testing.T) {
	a, err := GeneratePassword()
	if err != nil || len(a) != 32 {
		t.Fatalf("password: %v %q", err, a)
	}
	b, _ := GeneratePassword()
	if a == b {
		t.Fatal("expected unique passwords")
	}
	if strings.ContainsAny(a, ":/@") {
		t.Fatal("password must be URL-safe")
	}
}
