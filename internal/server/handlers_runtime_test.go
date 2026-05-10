package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/store"
)

// fakeLogStreamer implements LogStreamer for tests.
type fakeLogStreamer struct {
	content []byte
	err     error
}

func (f *fakeLogStreamer) Logs(_ context.Context, _ string, _ bool, _ string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.content)), nil
}

// newTestServerFull builds a test server with both a Deployer and a LogStreamer wired.
func newTestServerFull(t *testing.T, dep Deployer, docker LogStreamer) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	srv := New(s, "test")
	srv.Deployer = dep
	srv.Docker = docker
	h := httptest.NewServer(srv.Router())
	t.Cleanup(h.Close)
	return h, s
}

// insertSucceededDeployment creates an app (owner: username), inserts a
// succeeded deployment with a container name, sets it as current, and returns
// the owner token.
func insertSucceededDeployment(t *testing.T, s *store.Store, username, appName string) (ownerTok string, appID int64) {
	t.Helper()
	ctx := context.Background()
	u, err := s.CreateUser(ctx, username, false)
	if err != nil {
		t.Fatal(err)
	}
	pt, _ := auth.Generate()
	tk, err := s.CreateUserToken(ctx, u.ID, auth.Hash(pt), "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateApp(ctx, appName, store.AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOwner(ctx, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}

	d, err := s.CreateDeployment(ctx, a.ID, "sha256:abc123", "nginx:latest", "", "", tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	cname := "app-" + appName + "-ctr"
	if err := s.UpdateDeploymentStatus(ctx, d.ID, store.DeploySucceeded, "", cname); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentDeploy(ctx, a.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	return pt, a.ID
}

// --- status tests ---

func TestStatusUserToken(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, nil)
	ownerTok, _ := insertSucceededDeployment(t, s, "alice", "myapp")

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/status", ownerTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["state"] != "running" {
		t.Fatalf("state: %v", out["state"])
	}
	if out["image_digest"] != "sha256:abc123" {
		t.Fatalf("image_digest: %v", out["image_digest"])
	}
	if out["container_name"] != "app-myapp-ctr" {
		t.Fatalf("container_name: %v", out["container_name"])
	}
}

func TestStatusNoDeploy(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, nil)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/status", ownerTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["state"] != "down" {
		t.Fatalf("state: %v (expected down)", out["state"])
	}
}

// --- restart tests ---

func TestRestartCallsDeployer(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	resp, body := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/restart", ownerTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	if !fd.restartCalled {
		t.Fatal("expected Restart to be called on deployer")
	}
}

func TestRestartForbidsNonOwner(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	mintOwnerAndApp(t, s, "alice", "myapp")
	bobTok := mintUserToken(t, s, "bob", false)

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/restart", bobTok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// --- delete app tests ---

func TestDeleteAppRequiresAdmin(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	req, _ := http.NewRequest("DELETE", srv.URL+"/v1/apps/myapp", nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for non-admin, got %d", resp.StatusCode)
	}
}

func TestDeleteAppAdminSucceeds(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	mintOwnerAndApp(t, s, "alice", "myapp")
	adminTok := mintUserToken(t, s, "admin", true)

	req, _ := http.NewRequest("DELETE", srv.URL+"/v1/apps/myapp", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if !fd.deleteCalled {
		t.Fatal("expected DeleteApp to be called on deployer")
	}
}

// --- logs tests ---

func TestLogsRequiresOwner(t *testing.T) {
	srv, s := newTestServerFull(t, newFakeDeployer(), &fakeLogStreamer{content: []byte("hello")})
	_, _ = insertSucceededDeployment(t, s, "alice", "myapp")
	bobTok := mintUserToken(t, s, "bob", false)

	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/logs", bobTok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestLogsStreamsBody(t *testing.T) {
	logContent := []byte("line1\nline2\n")
	srv, s := newTestServerFull(t, newFakeDeployer(), &fakeLogStreamer{content: logContent})
	ownerTok, _ := insertSucceededDeployment(t, s, "alice", "myapp")

	req, _ := http.NewRequest("GET", srv.URL+"/v1/apps/myapp/logs", nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(logContent) {
		t.Fatalf("expected %q, got %q", logContent, body)
	}
}

func TestLogsNoDeployReturns404(t *testing.T) {
	srv, s := newTestServerFull(t, newFakeDeployer(), &fakeLogStreamer{})
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/logs", ownerTok, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
