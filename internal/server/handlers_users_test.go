package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func doJSON(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

func TestAdminCreatesUser(t *testing.T) {
	srv, s := newTestServer(t)
	adminTok := mintUserToken(t, s, "admin", true)
	resp, body := doJSON(t, "POST", srv.URL+"/v1/users", adminTok, map[string]any{"username": "alice"})
	if resp.StatusCode != 201 {
		t.Fatalf("status: %d body: %s", resp.StatusCode, body)
	}
}

func TestNonAdminCannotCreateUser(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	resp, _ := doJSON(t, "POST", srv.URL+"/v1/users", tok, map[string]any{"username": "bob"})
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestCreateUserInvalidName(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "admin", true)
	for _, name := range []string{"", "Capital", "with space", "-leadingdash", strings.Repeat("a", 33)} {
		resp, _ := doJSON(t, "POST", srv.URL+"/v1/users", tok, map[string]any{"username": name})
		if resp.StatusCode != 422 {
			t.Fatalf("name %q: status %d", name, resp.StatusCode)
		}
	}
}

func TestCreateDuplicateUserConflicts(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "admin", true)
	resp, _ := doJSON(t, "POST", srv.URL+"/v1/users", tok, map[string]any{"username": "bob"})
	if resp.StatusCode != 201 {
		t.Fatalf("first: %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, "POST", srv.URL+"/v1/users", tok, map[string]any{"username": "bob"})
	if resp.StatusCode != 409 {
		t.Fatalf("dup: %d", resp.StatusCode)
	}
}

func TestSelfGetOwn(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	resp, body := doJSON(t, "GET", srv.URL+"/v1/users/alice", tok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestUserGetOtherUserForbidden(t *testing.T) {
	srv, s := newTestServer(t)
	_ = mintUserToken(t, s, "bob", false)
	tok := mintUserToken(t, s, "alice", false)
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/users/bob", tok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAdminGetAnyUser(t *testing.T) {
	srv, s := newTestServer(t)
	_ = mintUserToken(t, s, "alice", false)
	adminTok := mintUserToken(t, s, "admin", true)
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/users/alice", adminTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestListUsersAdminOnly(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	resp, _ := doJSON(t, "GET", srv.URL+"/v1/users", tok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("non-admin status: %d", resp.StatusCode)
	}
	adminTok := mintUserToken(t, s, "admin", true)
	resp, _ = doJSON(t, "GET", srv.URL+"/v1/users", adminTok, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("admin status: %d", resp.StatusCode)
	}
}

func TestSelfIssueAndUseToken(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	resp, body := doJSON(t, "POST", srv.URL+"/v1/users/alice/tokens", aliceTok, map[string]any{"name": "ci"})
	if resp.StatusCode != 201 {
		t.Fatalf("status: %d body: %s", resp.StatusCode, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	plaintext, _ := out["token"].(string)
	if plaintext == "" {
		t.Fatal("no plaintext token returned")
	}

	// use the new token to whoami
	resp, _ = doJSON(t, "GET", srv.URL+"/v1/auth/whoami", plaintext, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("whoami: %d", resp.StatusCode)
	}
}

func TestRevokeMakesTokenUnusable(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	resp, body := doJSON(t, "POST", srv.URL+"/v1/users/alice/tokens", aliceTok, map[string]any{"name": "ci"})
	if resp.StatusCode != 201 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	plaintext, _ := out["token"].(string)
	idF, _ := out["id"].(float64)
	tokID := strconv.FormatInt(int64(idF), 10)

	resp, _ = doJSON(t, "DELETE", srv.URL+"/v1/users/alice/tokens/"+tokID, aliceTok, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("revoke: %d", resp.StatusCode)
	}

	resp, _ = doJSON(t, "GET", srv.URL+"/v1/auth/whoami", plaintext, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 after revoke, got %d", resp.StatusCode)
	}
}

func TestUserCannotIssueTokenForOther(t *testing.T) {
	srv, s := newTestServer(t)
	aliceTok := mintUserToken(t, s, "alice", false)
	_ = mintUserToken(t, s, "bob", false)
	resp, _ := doJSON(t, "POST", srv.URL+"/v1/users/bob/tokens", aliceTok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAdminDeletesUser(t *testing.T) {
	srv, s := newTestServer(t)
	_ = mintUserToken(t, s, "alice", false)
	adminTok := mintUserToken(t, s, "admin", true)
	resp, _ := doJSON(t, "DELETE", srv.URL+"/v1/users/alice", adminTok, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestNonAdminCannotDeleteUser(t *testing.T) {
	srv, s := newTestServer(t)
	tok := mintUserToken(t, s, "alice", false)
	_ = mintUserToken(t, s, "bob", false)
	resp, _ := doJSON(t, "DELETE", srv.URL+"/v1/users/bob", tok, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}
