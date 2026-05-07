package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserCreatesAppOwnsIt(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	resp, body := doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": "myapp"})
	if resp.StatusCode != 201 {
		t.Fatalf("status: %d body: %s", resp.StatusCode, body)
	}
	var a map[string]any
	_ = json.Unmarshal(body, &a)
	owners, _ := a["owners"].([]any)
	if len(owners) != 1 || owners[0] != "alice" {
		t.Fatalf("owners: %+v", owners)
	}
}

func TestCreateAppInvalidName(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	for _, name := range []string{"", "Foo", "1abc", "api", "a-", "--", "_", strings.Repeat("a", 33)} {
		resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": name})
		if resp.StatusCode != 422 {
			t.Fatalf("name %q: status %d", name, resp.StatusCode)
		}
	}
}

func TestCreateDuplicateAppConflicts(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": "myapp"})
	if resp.StatusCode != 201 {
		t.Fatalf("first: %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": "myapp"})
	if resp.StatusCode != 409 {
		t.Fatalf("dup: %d", resp.StatusCode)
	}
}

func TestNonOwnerCannotReadApp(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	bobTok := mintUserToken(t, s, "bob", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "myapp"})
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp", bobTok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAdminListsAllApps(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	bobTok := mintUserToken(t, s, "bob", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "alice-app"})
	doJSON(t, "POST", srv.URL+"/v1/apps", bobTok, map[string]any{"name": "bob-app"})

	tok := mintUserToken(t, s, "admin", true)
	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body: %s", resp.StatusCode, body)
	}
	var apps []map[string]any
	_ = json.Unmarshal(body, &apps)
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}
}

func TestUserListsOnlyOwnApps(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	bobTok := mintUserToken(t, s, "bob", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "alice-app"})
	doJSON(t, "POST", srv.URL+"/v1/apps", bobTok, map[string]any{"name": "bob-app"})

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps", aliceTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var apps []map[string]any
	_ = json.Unmarshal(body, &apps)
	if len(apps) != 1 || apps[0]["name"] != "alice-app" {
		t.Fatalf("apps: %+v", apps)
	}
}

func TestPatchAppPort(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", tok, map[string]any{"name": "myapp"})
	port := int64(9000)
	resp, body := doJSON(t, "PATCH", srv.URL+"/v1/apps/myapp", tok, map[string]any{"port": port})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body: %s", resp.StatusCode, body)
	}
	var a map[string]any
	_ = json.Unmarshal(body, &a)
	if int64(a["port"].(float64)) != 9000 {
		t.Fatalf("port: %v", a["port"])
	}
}

func TestNonAdminCannotChangeOwners(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	_ = mintUserToken(t, s, "bob", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "myapp"})
	resp, _ := doJSON(t, "PATCH", srv.URL+"/v1/apps/myapp", aliceTok, map[string]any{"owners": []string{"alice", "bob"}})
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAdminCanReplaceOwners(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	_ = mintUserToken(t, s, "bob", false)
	doJSON(t, "POST", srv.URL+"/v1/apps", aliceTok, map[string]any{"name": "myapp"})
	adminTok := mintUserToken(t, s, "admin", true)
	resp, body := doJSON(t, "PATCH", srv.URL+"/v1/apps/myapp", adminTok, map[string]any{"owners": []string{"bob"}})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body: %s", resp.StatusCode, body)
	}

	// bob now owner; alice should NOT be
	resp, _ = doJSON(t, "GET", srv.URL+"/v1/apps/myapp", aliceTok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("alice should be forbidden after owner replace, got %d", resp.StatusCode)
	}
}
