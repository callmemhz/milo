package server

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/store"
)

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newConsoleServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	srv := New(s, "test")
	h := httptest.NewServer(srv.Router())
	t.Cleanup(h.Close)
	return h, s
}

func seedUserWithPassword(t *testing.T, s *store.Store, username, password string, admin bool) {
	t.Helper()
	ctx := context.Background()
	u, err := s.CreateUser(ctx, username, admin)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPassword(ctx, u.ID, hash); err != nil {
		t.Fatal(err)
	}
}

// csrfFromJar pulls the milo_csrf cookie value for base, after a GET that sets it.
func csrfFromJar(t *testing.T, jar http.CookieJar, base string) string {
	t.Helper()
	u, _ := url.Parse(base)
	for _, c := range jar.Cookies(u) {
		if c.Name == csrfCookie {
			return c.Value
		}
	}
	t.Fatal("csrf cookie not set")
	return ""
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func TestConsoleLoginAndDashboard(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "alice", "supersecret", false)
	c := newClient(t)

	// GET login to obtain the CSRF cookie.
	if _, err := c.Get(h.URL + "/console/login"); err != nil {
		t.Fatal(err)
	}
	csrf := csrfFromJar(t, c.Jar, h.URL)

	// POST login with correct credentials -> follows redirect to dashboard.
	resp, err := c.PostForm(h.URL+"/console/login", url.Values{
		"username": {"alice"}, "password": {"supersecret"}, "_csrf": {csrf},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status = %d", resp.StatusCode)
	}
	if !strings.Contains(body, "我的实例") || !strings.Contains(body, "alice") {
		t.Fatalf("dashboard body missing expected content:\n%s", body)
	}
}

func TestConsoleLoginWrongPassword(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "alice", "supersecret", false)
	c := newClient(t)
	c.Get(h.URL + "/console/login")
	csrf := csrfFromJar(t, c.Jar, h.URL)

	resp, err := c.PostForm(h.URL+"/console/login", url.Values{
		"username": {"alice"}, "password": {"wrong"}, "_csrf": {csrf},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(readBody(t, resp), "用户名或密码错误") {
		t.Fatal("missing error message")
	}
}

func TestConsoleRequiresSession(t *testing.T) {
	h, _ := newConsoleServer(t)
	// Client that does NOT follow redirects.
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := c.Get(h.URL + "/console")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/console/login" {
		t.Fatalf("redirect = %q, want /console/login", loc)
	}
}

func TestConsoleLogout(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "bob", "supersecret", true)
	c := newClient(t)
	c.Get(h.URL + "/console/login")
	csrf := csrfFromJar(t, c.Jar, h.URL)
	if _, err := c.PostForm(h.URL+"/console/login", url.Values{
		"username": {"bob"}, "password": {"supersecret"}, "_csrf": {csrf},
	}); err != nil {
		t.Fatal(err)
	}

	// Logout, then dashboard must bounce to login.
	if _, err := c.PostForm(h.URL+"/console/logout", url.Values{"_csrf": {csrf}}); err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(h.URL + "/console")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readBody(t, resp), "登录") {
		t.Fatal("expected to land on login page after logout")
	}
}
