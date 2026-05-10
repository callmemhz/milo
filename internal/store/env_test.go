package store

import (
	"context"
	"testing"
)

func TestEnvSetGetUnset(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})

	if err := s.SetAppEnvVar(ctx, a.ID, "FOO", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAppEnvVar(ctx, a.ID, "BAZ", "qux"); err != nil {
		t.Fatal(err)
	}

	env, _ := s.GetAppEnv(ctx, a.ID)
	if env["FOO"] != "bar" || env["BAZ"] != "qux" {
		t.Fatalf("got %+v", env)
	}

	_ = s.SetAppEnvVar(ctx, a.ID, "FOO", "bar2") // upsert
	env, _ = s.GetAppEnv(ctx, a.ID)
	if env["FOO"] != "bar2" {
		t.Fatal("upsert failed")
	}

	_ = s.DeleteAppEnvVar(ctx, a.ID, "FOO")
	env, _ = s.GetAppEnv(ctx, a.ID)
	if _, ok := env["FOO"]; ok {
		t.Fatal("delete failed")
	}

	_ = s.ReplaceAppEnv(ctx, a.ID, map[string]string{"NEW": "v"})
	env, _ = s.GetAppEnv(ctx, a.ID)
	if len(env) != 1 || env["NEW"] != "v" {
		t.Fatalf("replace failed: %+v", env)
	}
}
