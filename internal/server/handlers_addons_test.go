package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/callmemhz/milo/internal/store"
	"github.com/callmemhz/milo/pkg/api"
)

func TestCreateAddonProvisionsAndReturnsURL(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	fd.provisionStore = s
	tok := mintUserToken(t, s, "alice", false)

	resp, body := doJSON(t, "POST", srv.URL+"/v1/addons", tok, api.CreateAddonReq{Name: "mydb", Engine: "postgres"})
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d body: %s", resp.StatusCode, body)
	}
	if !fd.provisionCalled {
		t.Fatal("expected ProvisionAddon to be called")
	}
	var out api.AddonResp
	_ = json.Unmarshal(body, &out)
	if out.Engine != "postgres" || out.Version != "16" || out.Status != store.AddonRunning {
		t.Fatalf("unexpected: %+v", out)
	}
	if len(out.Owners) != 1 || out.Owners[0] != "alice" {
		t.Fatalf("owners: %+v", out.Owners)
	}
	if !strings.HasPrefix(out.URL, "postgres://app:") || !strings.Contains(out.URL, "@mydb:5432/app") {
		t.Fatalf("url: %q", out.URL)
	}
}

func TestCreateAddonRejectsBadEngineAndVersion(t *testing.T) {
	srv, s := newTestServerWithDeployer(t, newFakeDeployer())
	tok := mintUserToken(t, s, "alice", false)

	resp, _ := doJSON(t, "POST", srv.URL+"/v1/addons", tok, api.CreateAddonReq{Name: "mydb", Engine: "mysql"})
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, "POST", srv.URL+"/v1/addons", tok, api.CreateAddonReq{Name: "mydb", Engine: "postgres", Version: "13"})
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, "POST", srv.URL+"/v1/addons", tok, api.CreateAddonReq{Name: "x", Engine: "postgres"})
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422 for bad name, got %d", resp.StatusCode)
	}
}

func TestListAddonsScopedByOwner(t *testing.T) {
	fd := newFakeDeployer()
	srv, s := newTestServerWithDeployer(t, fd)
	alice := mintUserToken(t, s, "alice", false)
	bob := mintUserToken(t, s, "bob", false)

	if resp, body := doJSON(t, "POST", srv.URL+"/v1/addons", alice, api.CreateAddonReq{Name: "adb", Engine: "postgres"}); resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}

	var got []api.AddonResp
	_, body := doJSON(t, "GET", srv.URL+"/v1/addons", bob, nil)
	_ = json.Unmarshal(body, &got)
	if len(got) != 0 {
		t.Fatalf("bob should see none, got %+v", got)
	}
	_, body = doJSON(t, "GET", srv.URL+"/v1/addons", alice, nil)
	_ = json.Unmarshal(body, &got)
	if len(got) != 1 || got[0].Name != "adb" || got[0].URL != "" {
		t.Fatalf("alice list: %+v", got)
	}
}

// setupLinkFixture creates owner alice with app "web" and addon "mydb".
func setupLinkFixture(t *testing.T, fd *fakeDeployer) (srvURL string, s *store.Store, tok string) {
	t.Helper()
	srv, st := newTestServerWithDeployer(t, fd)
	fd.provisionStore = st
	tok = mintOwnerAndApp(t, st, "alice", "web")
	if resp, body := doJSON(t, "POST", srv.URL+"/v1/addons", tok, api.CreateAddonReq{Name: "mydb", Engine: "postgres"}); resp.StatusCode != 201 {
		t.Fatalf("create addon: %d %s", resp.StatusCode, body)
	}
	return srv.URL, st, tok
}

func TestExposeUnexposeAddonHTTP(t *testing.T) {
	fd := newFakeDeployer()
	url, s, tok := setupLinkFixture(t, fd)

	// expose: flips the flag, re-provisions, assigns a host port, and returns
	// an external URL pointing at <addon>.<root>:<host_port>.
	resp, body := doJSON(t, "POST", url+"/v1/addons/mydb/expose", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expose: %d %s", resp.StatusCode, body)
	}
	var out api.AddonResp
	_ = json.Unmarshal(body, &out)
	if !out.Exposed || out.HostPort != 54321 {
		t.Fatalf("expected exposed with host port 54321, got %+v", out)
	}
	if out.ExternalURL != "postgres://app:"+mustAddonPassword(t, s, "mydb")+"@mydb.app.example.com:54321/app?sslmode=disable" {
		t.Fatalf("external url: %q", out.ExternalURL)
	}

	// idempotent: exposing again is a no-op (does not re-provision).
	fd.provisionCalled = false
	if resp, _ := doJSON(t, "POST", url+"/v1/addons/mydb/expose", tok, nil); resp.StatusCode != 200 {
		t.Fatalf("re-expose: %d", resp.StatusCode)
	}
	if fd.provisionCalled {
		t.Fatal("re-expose should not re-provision when already exposed")
	}

	// unexpose: clears the flag and drops the external URL, but keeps host_port
	// in the DB so re-exposing reuses the same port.
	resp, body = doJSON(t, "DELETE", url+"/v1/addons/mydb/expose", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("unexpose: %d %s", resp.StatusCode, body)
	}
	var unexposed api.AddonResp
	_ = json.Unmarshal(body, &unexposed)
	if unexposed.Exposed || unexposed.ExternalURL != "" {
		t.Fatalf("expected not exposed and no external url, got %+v", unexposed)
	}
	if got, _ := s.GetAddonByName(context.Background(), "mydb"); got.HostPort != 54321 {
		t.Fatalf("expected host port retained, got %d", got.HostPort)
	}
}

func mustAddonPassword(t *testing.T, s *store.Store, name string) string {
	t.Helper()
	a, err := s.GetAddonByName(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return a.Password
}

func TestLinkLifecycleHTTP(t *testing.T) {
	fd := newFakeDeployer()
	url, s, tok := setupLinkFixture(t, fd)

	resp, body := doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "mydb"})
	if resp.StatusCode != 201 {
		t.Fatalf("link: %d %s", resp.StatusCode, body)
	}
	var link api.LinkResp
	_ = json.Unmarshal(body, &link)
	if link.EnvKey != "DATABASE_URL" || link.App != "web" || link.Addon != "mydb" {
		t.Fatalf("unexpected: %+v", link)
	}

	// duplicate link → conflict
	resp, _ = doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "mydb"})
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}

	// list links
	var links []api.LinkResp
	_, body = doJSON(t, "GET", url+"/v1/apps/web/links", tok, nil)
	_ = json.Unmarshal(body, &links)
	if len(links) != 1 || links[0].EnvKey != "DATABASE_URL" {
		t.Fatalf("links: %+v", links)
	}

	// unlink
	resp, _ = doJSON(t, "DELETE", url+"/v1/apps/web/links/mydb", tok, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	got, _ := s.ListLinksForApp(context.Background(), mustAppID(t, s, "web"))
	if len(got) != 0 {
		t.Fatalf("expected no links, got %+v", got)
	}
}

func TestLinkEnvKeyConflictRequiresAlias(t *testing.T) {
	fd := newFakeDeployer()
	url, _, tok := setupLinkFixture(t, fd)
	if resp, body := doJSON(t, "POST", url+"/v1/addons", tok, api.CreateAddonReq{Name: "otherdb", Engine: "postgres"}); resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}

	if resp, _ := doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "mydb"}); resp.StatusCode != 201 {
		t.Fatalf("first link: %d", resp.StatusCode)
	}
	// second postgres without alias collides on DATABASE_URL
	resp, _ := doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "otherdb"})
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	// alias resolves the conflict
	resp, body := doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "otherdb", Alias: "REPLICA"})
	if resp.StatusCode != 201 {
		t.Fatalf("aliased link: %d %s", resp.StatusCode, body)
	}
	var link api.LinkResp
	_ = json.Unmarshal(body, &link)
	if link.EnvKey != "REPLICA_URL" {
		t.Fatalf("env key: %q", link.EnvKey)
	}
	// invalid alias rejected
	resp, _ = doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "mydb", Alias: "bad-alias"})
	if resp.StatusCode != 422 && resp.StatusCode != 409 {
		t.Fatalf("expected 422/409, got %d", resp.StatusCode)
	}
}

func TestLinkForbiddenForNonAddonOwner(t *testing.T) {
	fd := newFakeDeployer()
	url, s, _ := setupLinkFixture(t, fd)
	// bob owns his own app but not the addon
	bob := mintOwnerAndApp(t, s, "bob", "bobapp")
	resp, _ := doJSON(t, "POST", url+"/v1/apps/bobapp/links", bob, api.CreateLinkReq{Addon: "mydb"})
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDeleteAddonBlockedByLinksUnlessForced(t *testing.T) {
	fd := newFakeDeployer()
	url, s, tok := setupLinkFixture(t, fd)
	fd.deleteAddonStore = s
	admin := mintUserToken(t, s, "root", true)

	if resp, _ := doJSON(t, "POST", url+"/v1/apps/web/links", tok, api.CreateLinkReq{Addon: "mydb"}); resp.StatusCode != 201 {
		t.Fatal("link failed")
	}

	// non-admin cannot delete
	resp, _ := doJSON(t, "DELETE", url+"/v1/addons/mydb", tok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	// linked → 409
	resp, _ = doJSON(t, "DELETE", url+"/v1/addons/mydb", admin, nil)
	if resp.StatusCode != 409 {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	// force → 204
	resp, _ = doJSON(t, "DELETE", url+"/v1/addons/mydb?force=true&delete_volumes=true", admin, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if !fd.deleteAddonCalled || !fd.deleteAddonVolume {
		t.Fatal("expected DeleteAddon(force volumes) to be called")
	}
}

func TestRestartAddonCallsProvision(t *testing.T) {
	fd := newFakeDeployer()
	url, s, tok := setupLinkFixture(t, fd)
	fd.provisionCalled = false

	resp, body := doJSON(t, "POST", url+"/v1/addons/mydb/restart", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("restart: %d %s", resp.StatusCode, body)
	}
	if !fd.provisionCalled {
		t.Fatal("expected ProvisionAddon to be called")
	}
	addon, _ := s.GetAddonByName(context.Background(), "mydb")
	if fd.lastProvisionID != addon.ID {
		t.Fatalf("provisioned wrong addon: %d != %d", fd.lastProvisionID, addon.ID)
	}
}

func mustAppID(t *testing.T, s *store.Store, name string) int64 {
	t.Helper()
	a, err := s.GetAppByName(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}
