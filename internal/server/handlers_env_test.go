package server

import (
	"encoding/json"
	"testing"
)

func TestEnvRequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", "", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestEnvOwnerCanGetSetUnset(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": "myapp"})

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get: %d body: %s", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, "PATCH", srv.URL+"/v1/apps/myapp/env", tok, map[string]any{
		"set": map[string]any{"FOO": "bar", "BAZ": "qux"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("patch: %d", resp.StatusCode)
	}

	resp, body = doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", tok, nil)
	var env map[string]string
	_ = json.Unmarshal(body, &env)
	if env["FOO"] != "bar" || env["BAZ"] != "qux" {
		t.Fatalf("env: %+v", env)
	}

	resp, _ = doJSON(t, "PATCH", srv.URL+"/v1/apps/myapp/env", tok, map[string]any{
		"unset": []string{"FOO"},
	})
	resp, body = doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", tok, nil)
	var env2 map[string]string
	_ = json.Unmarshal(body, &env2)
	if _, ok := env2["FOO"]; ok {
		t.Fatal("FOO should be unset")
	}
	_ = resp
}

func TestEnvPutReplaces(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": "myapp"})
	doJSON(t, "PATCH", srv.URL+"/v1/apps/myapp/env", tok, map[string]any{
		"set": map[string]any{"FOO": "bar"},
	})
	resp, _ := doJSON(t, "PUT", srv.URL+"/v1/apps/myapp/env", tok, map[string]any{"NEW": "v"})
	if resp.StatusCode != 200 {
		t.Fatalf("put: %d", resp.StatusCode)
	}
	_, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", tok, nil)
	var env map[string]string
	_ = json.Unmarshal(body, &env)
	if len(env) != 1 || env["NEW"] != "v" {
		t.Fatalf("after put: %+v", env)
	}
}

func TestEnvNonOwnerForbidden(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	bobTok := mintUserToken(t, s, "bob", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "myapp"})
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", bobTok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestEnvAdminCanRead(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "myapp"})
	adminTok := mintUserToken(t, s, "admin", true)
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/env", adminTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}
