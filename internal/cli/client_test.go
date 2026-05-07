package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		Endpoint: srv.URL,
		Token:    "test-token",
		HTTP:     srv.Client(),
	}
}

func TestClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"key": "value"})
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	var result map[string]string
	if err := cli.Get("/v1/test", &result); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestClient_Post(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"echo": body["name"]})
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	var result map[string]string
	if err := cli.Post("/v1/things", map[string]string{"name": "foo"}, &result); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if result["echo"] != "foo" {
		t.Errorf("unexpected echo: %q", result["echo"])
	}
}

func TestClient_Patch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"patched": "yes"})
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	var result map[string]string
	if err := cli.Patch("/v1/things/1", map[string]string{"x": "y"}, &result); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if result["patched"] != "yes" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestClient_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	if err := cli.Delete("/v1/things/1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "thing not found",
		})
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	var out map[string]string
	err := cli.Get("/v1/missing", &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "thing not found") {
		t.Errorf("error should mention message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not_found") {
		t.Errorf("error should mention code, got: %v", err)
	}
}

func TestClient_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "line1\nline2\n")
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	rc, err := cli.Stream("/v1/apps/myapp/logs")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "line1\nline2\n" {
		t.Errorf("unexpected stream data: %q", string(data))
	}
}

func TestClient_Stream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cli := newTestClient(srv)
	_, err := cli.Stream("/v1/apps/nope/logs")
	if err == nil {
		t.Fatal("expected error for 403 status")
	}
}
