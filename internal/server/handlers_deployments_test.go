package server

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/deploy"
	"github.com/callmemhz/milo/internal/store"
	"github.com/callmemhz/milo/pkg/api"
)

// fakeDeployer records calls and returns canned responses.
type fakeDeployer struct {
	deployResp    store.Deployment
	deployErr     error
	lastDeployReq deploy.DeployRequest
	restartResp   store.Deployment
	restartErr    error
	deleteErr     error
	deployCalled  bool
	restartCalled bool
	deleteCalled  bool
	deleteVolume  bool
	locks         *deploy.LockManager

	provisionErr      error
	provisionCalled   bool
	lastProvisionID   int64
	deleteAddonErr    error
	deleteAddonCalled bool
	deleteAddonVolume bool
	provisionStore    *store.Store // when set, ProvisionAddon marks the addon running
	deleteAddonStore  *store.Store // when set, DeleteAddon removes links + soft-deletes
}

func newFakeDeployer() *fakeDeployer {
	return &fakeDeployer{
		deployResp:  store.Deployment{ID: 1, AppID: 1, ImageRef: "nginx:latest", Status: "succeeded", CreatedAt: time.Now()},
		restartResp: store.Deployment{ID: 2, AppID: 1, ImageRef: "nginx:latest", Status: "succeeded", CreatedAt: time.Now()},
		locks:       deploy.NewLockManager(),
	}
}

func (f *fakeDeployer) Deploy(_ context.Context, req deploy.DeployRequest) (store.Deployment, error) {
	f.deployCalled = true
	f.lastDeployReq = req
	return f.deployResp, f.deployErr
}

func (f *fakeDeployer) Restart(_ context.Context, _ int64) (store.Deployment, error) {
	f.restartCalled = true
	return f.restartResp, f.restartErr
}

func (f *fakeDeployer) DeleteApp(_ context.Context, _ int64, deleteVolume bool) error {
	f.deleteCalled = true
	f.deleteVolume = deleteVolume
	return f.deleteErr
}

func (f *fakeDeployer) ProvisionAddon(ctx context.Context, addonID int64) error {
	f.provisionCalled = true
	f.lastProvisionID = addonID
	if f.provisionErr == nil && f.provisionStore != nil {
		addon, err := f.provisionStore.GetAddonByID(ctx, addonID)
		if err != nil {
			return err
		}
		// Mirror the real orchestrator: first expose assigns a host port.
		if addon.Exposed && addon.HostPort == 0 {
			_ = f.provisionStore.SetAddonHostPort(ctx, addonID, 54321)
		}
		_ = f.provisionStore.UpdateAddonStatus(ctx, addonID, store.AddonRunning, "addon-"+addon.Name)
	}
	return f.provisionErr
}

func (f *fakeDeployer) DeleteAddon(ctx context.Context, addonID int64, deleteVolume bool) error {
	f.deleteAddonCalled = true
	f.deleteAddonVolume = deleteVolume
	if f.deleteAddonErr == nil && f.deleteAddonStore != nil {
		_ = f.deleteAddonStore.DeleteLinksForAddon(ctx, addonID)
		_ = f.deleteAddonStore.SoftDeleteAddon(ctx, addonID)
	}
	return f.deleteAddonErr
}

func (f *fakeDeployer) Locks() *deploy.LockManager { return f.locks }

// mintOwnerAndApp creates a user who owns an app and returns (userToken, appName).
func mintOwnerAndApp(t *testing.T, s *store.Store, username, appName string) string {
	t.Helper()
	ctx := context.Background()
	u, err := s.CreateUser(ctx, username, false)
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
	a, err := s.CreateApp(ctx, appName, 8080, "/", 30, 0.5, 512)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOwner(ctx, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	return pt
}

// insertDeployment inserts a deployment row directly via the store and returns it.
// triggeredBy must be a valid token ID (FK constraint).
func insertDeployment(t *testing.T, s *store.Store, appID, triggeredBy int64, status string) store.Deployment {
	t.Helper()
	ctx := context.Background()
	d, err := s.CreateDeployment(ctx, appID, "sha256:abc", "nginx:latest", "", "", triggeredBy)
	if err != nil {
		t.Fatalf("insertDeployment: %v", err)
	}
	if err := s.UpdateDeploymentStatus(ctx, d.ID, status, "", ""); err != nil {
		t.Fatal(err)
	}
	d, err = s.GetDeployment(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// mintOwnerAppAndToken creates a user who owns an app, creates a user token,
// and returns (userToken plaintext, appID, tokenID).
func mintOwnerAppAndToken(t *testing.T, s *store.Store, username, appName string) (string, int64, int64) {
	t.Helper()
	ctx := context.Background()
	u, err := s.CreateUser(ctx, username, false)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	tk, err := s.CreateUserToken(ctx, u.ID, auth.Hash(pt), "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateApp(ctx, appName, 8080, "/", 30, 0.5, 512)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOwner(ctx, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	return pt, a.ID, tk.ID
}

// --- auth/permission tests ---

func TestDeployUnauthorizedWithoutToken(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	// create app so it exists
	mintOwnerAndApp(t, s, "alice", "myapp")
	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", "", map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDeployForbiddenWithWrongScopeDeployToken(t *testing.T) {
	// deploy token for app-a used against app-b
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ctx := context.Background()

	// create app-a and a deploy token for it
	a, err := s.CreateApp(ctx, "app-a", 8080, "/", 30, 0.5, 512)
	if err != nil {
		t.Fatal(err)
	}
	pt, _ := auth.Generate()
	if _, err := s.CreateDeployToken(ctx, a.ID, auth.Hash(pt), ""); err != nil {
		t.Fatal(err)
	}

	// create app-b
	if _, err := s.CreateApp(ctx, "app-b", 8080, "/", 30, 0.5, 512); err != nil {
		t.Fatal(err)
	}

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/app-b/deployments", pt, map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDeployByOwnerSucceeds(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	resp, body := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", ownerTok, map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	if !fd.deployCalled {
		t.Fatal("expected Deploy to be called")
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["status"] != "succeeded" {
		t.Fatalf("status: %v", out["status"])
	}
}

func TestDeployByDeployTokenSucceeds(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ctx := context.Background()

	a, err := s.CreateApp(ctx, "myapp", 8080, "/", 30, 0.5, 512)
	if err != nil {
		t.Fatal(err)
	}
	// update fakeDeployer response to use correct AppID
	fd.deployResp.AppID = a.ID
	pt, _ := auth.Generate()
	if _, err := s.CreateDeployToken(ctx, a.ID, auth.Hash(pt), "ci"); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", pt, map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	if !fd.deployCalled {
		t.Fatal("expected Deploy to be called")
	}
}

func TestDeployMissingImageReturns422(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", ownerTok, map[string]any{})
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestDeployFailsReturns422(t *testing.T) {
	fd := newFakeDeployer()
	failDep := store.Deployment{
		ID: 99, AppID: 1, ImageRef: "nginx:latest", Status: "failed",
		CreatedAt: time.Now(),
	}
	reason := "pull_failed"
	failDep.FailureReason = &reason
	fd.deployResp = failDep
	fd.deployErr = &api.Error{Code: api.ErrDeployFailed, Message: "pull_failed"}

	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok := mintOwnerAndApp(t, s, "alice", "myapp")
	// update fakeDeployer AppID to match
	a, _ := s.GetAppByName(context.Background(), "myapp")
	fd.deployResp.AppID = a.ID

	resp, body := doJSON(t, "POST", srv.URL+"/v1/apps/myapp/deployments", ownerTok, map[string]any{"image": "nginx:latest"})
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422, got %d body: %s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	// response should include the deployment record
	dep, ok := out["deployment"].(map[string]any)
	if !ok {
		t.Fatalf("expected deployment field in response, body: %s", body)
	}
	if dep["status"] != "failed" {
		t.Fatalf("deployment status: %v", dep["status"])
	}
}

// --- list / get ---

func TestListDeployments(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	ownerTok, appID, tokID := mintOwnerAppAndToken(t, s, "alice", "myapp")
	insertDeployment(t, s, appID, tokID, "succeeded")
	insertDeployment(t, s, appID, tokID, "failed")

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/deployments", ownerTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	var list []map[string]any
	_ = json.Unmarshal(body, &list)
	if len(list) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(list))
	}
}

func TestGetDeployment(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	ownerTok, appID, tokID := mintOwnerAppAndToken(t, s, "alice", "myapp")
	d := insertDeployment(t, s, appID, tokID, "succeeded")

	resp, body := doJSON(t, "GET", srv.URL+"/v1/apps/myapp/deployments/"+itoa(d.ID), ownerTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body: %s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["status"] != "succeeded" {
		t.Fatalf("status: %v", out["status"])
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	ownerTok := mintOwnerAndApp(t, s, "alice2", "myapp2")

	resp, _ := doJSON(t, "GET", srv.URL+"/v1/apps/myapp2/deployments/9999", ownerTok, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// --- cancel ---

func TestCancelOnlyForInflight(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	ownerTok, appID, tokID := mintOwnerAppAndToken(t, s, "alice3", "myapp3")
	d := insertDeployment(t, s, appID, tokID, "succeeded")

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp3/deployments/"+itoa(d.ID)+"/cancel", ownerTok, nil)
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409 for non-inflight cancel, got %d", resp.StatusCode)
	}
}

func TestCancelInflight(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	ownerTok, appID, tokID := mintOwnerAppAndToken(t, s, "alice4", "myapp4")
	d := insertDeployment(t, s, appID, tokID, "deploying")

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp4/deployments/"+itoa(d.ID)+"/cancel", ownerTok, nil)
	if resp.StatusCode != 202 {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func TestRegistryAuthThreadedThrough(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok, _, _ := mintOwnerAppAndToken(t, s, "alice-ra", "myapp-ra")

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp-ra/deployments", ownerTok, map[string]any{
		"image": "ghcr.io/private/x@sha256:abc",
		"registry_auth": map[string]string{
			"username": "ci-bot",
			"password": "ghp_secret",
		},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if fd.lastDeployReq.RegistryAuth == nil {
		t.Fatal("expected RegistryAuth to be threaded through; got nil")
	}
	if fd.lastDeployReq.RegistryAuth.Username != "ci-bot" {
		t.Fatalf("user: %q", fd.lastDeployReq.RegistryAuth.Username)
	}
	if fd.lastDeployReq.RegistryAuth.Password != "ghp_secret" {
		t.Fatalf("password not threaded through")
	}
}

func TestRegistryAuthOmittedWhenAbsent(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	ownerTok, _, _ := mintOwnerAppAndToken(t, s, "alice-na", "myapp-na")

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/apps/myapp-na/deployments", ownerTok, map[string]any{
		"image": "nginx:alpine",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if fd.lastDeployReq.RegistryAuth != nil {
		t.Fatal("expected RegistryAuth=nil when omitted from request")
	}
}
