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

func TestExternalConnectionURL(t *testing.T) {
	pg := ExternalConnectionURL("postgres", "mydb", "s3cret", "app.example.com", 54321)
	if pg != "postgres://app:s3cret@mydb.app.example.com:54321/app?sslmode=disable" {
		t.Fatalf("got %q", pg)
	}
	rd := ExternalConnectionURL("redis", "cache", "s3cret", "app.example.com", 54322)
	if rd != "redis://:s3cret@cache.app.example.com:54322/0" {
		t.Fatalf("got %q", rd)
	}
	// Not exposed (hostPort 0) or no root domain → empty.
	if got := ExternalConnectionURL("postgres", "mydb", "s3cret", "app.example.com", 0); got != "" {
		t.Fatalf("expected empty for hostPort 0, got %q", got)
	}
	if got := ExternalConnectionURL("postgres", "mydb", "s3cret", "", 54321); got != "" {
		t.Fatalf("expected empty for empty root domain, got %q", got)
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
