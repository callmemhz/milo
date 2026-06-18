package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	return newTestServerWithDeployer(t, nil)
}

func newTestServerWithDeployer(t *testing.T, dep Deployer) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	srv := New(s, "test")
	srv.Deployer = dep
	srv.RootDomain = "app.example.com"
	h := httptest.NewServer(srv.Router())
	t.Cleanup(h.Close)
	return h, s
}

func mintUserToken(t *testing.T, s *store.Store, username string, isAdmin bool) string {
	t.Helper()
	ctx := context.Background()
	u, err := s.CreateUser(ctx, username, isAdmin)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUserToken(ctx, u.ID, auth.Hash(pt), ""); err != nil {
		t.Fatal(err)
	}
	return pt
}

func mintDeployToken(t *testing.T, s *store.Store, appName string) string {
	t.Helper()
	ctx := context.Background()
	a, err := s.CreateApp(ctx, appName, 8080, "/", 30, 0.5, 512)
	if err != nil {
		t.Fatal(err)
	}
	pt, _ := auth.Generate()
	_, err = s.CreateDeployToken(ctx, a.ID, auth.Hash(pt), "")
	if err != nil {
		t.Fatal(err)
	}
	return pt
}

func TestHealthzNoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["status"] != "ok" {
		t.Fatalf("body: %+v", out)
	}
}

func TestVersionNoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/v1/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["version"] != "test" {
		t.Fatalf("body: %+v", out)
	}
}

func TestWhoamiRequiresToken(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/v1/auth/whoami")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestWhoamiUserToken(t *testing.T) {
	srv, s := newTestServer(t)
	pt := mintUserToken(t, s, "alice", false)
	req, _ := http.NewRequest("GET", srv.URL+"/v1/auth/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+pt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["username"] != "alice" {
		t.Fatalf("body: %+v", out)
	}
	if out["token_kind"] != "user" {
		t.Fatalf("kind: %v", out["token_kind"])
	}
	if out["is_admin"] != false {
		t.Fatalf("is_admin: %v", out["is_admin"])
	}
}

func TestWhoamiAdminUser(t *testing.T) {
	srv, s := newTestServer(t)
	pt := mintUserToken(t, s, "admin", true)
	req, _ := http.NewRequest("GET", srv.URL+"/v1/auth/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+pt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["is_admin"] != true {
		t.Fatalf("is_admin: %v", out["is_admin"])
	}
}

func TestWhoamiDeployTokenIncludesScope(t *testing.T) {
	srv, s := newTestServer(t)
	pt := mintDeployToken(t, s, "myapp")
	req, _ := http.NewRequest("GET", srv.URL+"/v1/auth/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+pt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["token_kind"] != "deploy" {
		t.Fatalf("kind: %v", out["token_kind"])
	}
	if out["scope"] != "myapp" {
		t.Fatalf("scope: %v", out["scope"])
	}
	if _, ok := out["username"]; ok {
		t.Fatal("username should be absent on deploy token")
	}
}
