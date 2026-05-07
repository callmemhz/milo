package server

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestOwnerCreatesDeployToken(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	resp, body := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/tokens", ownerTok, map[string]any{"name": "ci"})
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d body: %s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	plaintext, _ := out["token"].(string)
	if plaintext == "" {
		t.Fatal("expected plaintext token in response")
	}
	if out["name"] != "ci" {
		t.Fatalf("name: %v", out["name"])
	}

	// The deploy token should be usable to trigger a deploy on myapp.
	resp, body = doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", plaintext, map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 200 {
		t.Fatalf("deploy with deploy token: expected 200, got %d body: %s", resp.StatusCode, body)
	}
	if !fd.deployCalled {
		t.Fatal("expected Deploy to be called")
	}
}

func TestNonOwnerCannotCreateDeployToken(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	mintOwnerAndApp(t, s, "alice", "myapp")
	bobTok := mintUserToken(t, s, "bob", false)

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/tokens", bobTok, map[string]any{"name": "ci"})
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestListDeployTokens(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	// create two tokens
	doJSON(t, "POST", srv.URL+"/v1/apps/myapp/tokens", ownerTok, map[string]any{"name": "t1"})
	doJSON(t, "POST", srv.URL+"/v1/apps/myapp/tokens", ownerTok, map[string]any{"name": "t2"})

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/tokens", ownerTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	var list []map[string]any
	_ = json.Unmarshal(body, &list)
	if len(list) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(list))
	}
	for _, tk := range list {
		if tk["kind"] != "deploy" {
			t.Fatalf("expected kind=deploy, got %v", tk["kind"])
		}
	}
}

func TestRevokedDeployTokenIsUnusable(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	// Create deploy token
	_, body := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/tokens", ownerTok, map[string]any{"name": "ci"})
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	plaintext, _ := out["token"].(string)
	idF, _ := out["id"].(float64)
	tokID := strconv.FormatInt(int64(idF), 10)

	// Revoke it
	resp, _ := doJSON(t, "DELETE", srv.URL+"/v1/apps/myapp/tokens/"+tokID, ownerTok, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("revoke: expected 204, got %d", resp.StatusCode)
	}

	// Attempt deploy with the revoked token → should be 401
	resp, _ = doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", plaintext, map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 after revoke, got %d", resp.StatusCode)
	}
}
