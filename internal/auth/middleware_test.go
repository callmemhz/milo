package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/callmemhz/milo-apps-kit/internal/store"
)

func setupStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMiddlewareRejectsMissingHeader(t *testing.T) {
	s := setupStore(t)
	a := &Authenticator{Store: s}
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != 401 {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	s := setupStore(t)
	a := &Authenticator{Store: s}
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestMiddlewareAttachesUserIdentity(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "alice", false)
	plaintext, _ := Generate()
	_, _ = s.CreateUserToken(ctx, u.ID, Hash(plaintext), "")

	var got *Identity
	a := &Authenticator{Store: s}
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Fatal("no identity")
		}
		got = id
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status: %d", rr.Code)
	}
	if got.User == nil || got.User.Username != "alice" {
		t.Fatalf("identity: %+v", got)
	}
	if got.Token.Kind != "user" {
		t.Fatalf("kind: %s", got.Token.Kind)
	}
}

func TestMiddlewareAttachesDeployIdentity(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	app, _ := s.CreateApp(ctx, "myapp", 8080, "/", 30, 0.5, 512)
	plaintext, _ := Generate()
	_, _ = s.CreateDeployToken(ctx, app.ID, Hash(plaintext), "ci")

	var got *Identity
	a := &Authenticator{Store: s}
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := IdentityFromContext(r.Context())
		got = id
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status: %d", rr.Code)
	}
	if got.AppID == nil || *got.AppID != app.ID {
		t.Fatalf("identity: %+v", got)
	}
	if got.User != nil {
		t.Fatalf("expected no user on deploy token")
	}
	if got.Token.Kind != "deploy" {
		t.Fatalf("kind: %s", got.Token.Kind)
	}
}
