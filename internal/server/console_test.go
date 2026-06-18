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

// loginAs logs the client in and returns the CSRF token for subsequent POSTs.
func loginAs(t *testing.T, c *http.Client, base, user, pass string) string {
	t.Helper()
	c.Get(base + "/console/login")
	csrf := csrfFromJar(t, c.Jar, base)
	if _, err := c.PostForm(base+"/console/login", url.Values{
		"username": {user}, "password": {pass}, "_csrf": {csrf},
	}); err != nil {
		t.Fatal(err)
	}
	return csrf
}

func TestConsoleAdminPages(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "boss", "supersecret", true)
	c := newClient(t)
	csrf := loginAs(t, c, h.URL, "boss", "supersecret")

	// Users page lists the admin.
	resp, _ := c.Get(h.URL + "/console/users")
	if body := readBody(t, resp); !strings.Contains(body, "用户管理") || !strings.Contains(body, "boss") {
		t.Fatalf("users page missing content:\n%s", body)
	}

	// Create a new user, then confirm it shows up.
	if _, err := c.PostForm(h.URL+"/console/users/create", url.Values{
		"username": {"carol"}, "password": {"carolpass1"}, "_csrf": {csrf},
	}); err != nil {
		t.Fatal(err)
	}
	resp, _ = c.Get(h.URL + "/console/users")
	if !strings.Contains(readBody(t, resp), "carol") {
		t.Fatal("created user not listed")
	}
	// And the new user can log in (password was set).
	c2 := newClient(t)
	c2.Get(h.URL + "/console/login")
	csrf2 := csrfFromJar(t, c2.Jar, h.URL)
	resp, _ = c2.PostForm(h.URL+"/console/login", url.Values{
		"username": {"carol"}, "password": {"carolpass1"}, "_csrf": {csrf2},
	})
	if !strings.Contains(readBody(t, resp), "我的实例") {
		t.Fatal("created user could not log in")
	}

	// Admin overview renders.
	resp, _ = c.Get(h.URL + "/console/admin")
	if !strings.Contains(readBody(t, resp), "宿主机状态") {
		t.Fatal("admin page missing")
	}
}

func TestConsoleAllInstances(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "boss", "supersecret", true)
	// An app owned by someone else.
	ctx := context.Background()
	other, _ := s.CreateUser(ctx, "dana", false)
	a, _ := s.CreateApp(ctx, "dana-app", 8080, "/", 30, 0.5, 256)
	_ = s.AddOwner(ctx, a.ID, other.ID)

	c := newClient(t)
	loginAs(t, c, h.URL, "boss", "supersecret")

	// Personal dashboard must NOT show another user's app.
	resp, _ := c.Get(h.URL + "/console")
	if strings.Contains(readBody(t, resp), "dana-app") {
		t.Fatal("personal dashboard leaked another user's app")
	}
	// Admin all-instances view must show it, with owner.
	resp, _ = c.Get(h.URL + "/console/admin/instances")
	body := readBody(t, resp)
	if !strings.Contains(body, "dana-app") || !strings.Contains(body, "dana") {
		t.Fatalf("all-instances view missing app/owner:\n%s", body)
	}
}

func TestConsoleNonAdminForbidden(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "alice", "supersecret", false)
	c := newClient(t)
	loginAs(t, c, h.URL, "alice", "supersecret")

	for _, path := range []string{"/console/users", "/console/admin", "/console/admin/instances", "/console/admin/images"} {
		resp, err := c.Get(h.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s status = %d, want 403", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestConsoleFrozenUserBlocked(t *testing.T) {
	h, s := newConsoleServer(t)
	seedUserWithPassword(t, s, "eve", "supersecret", false)
	ctx := context.Background()
	u, _ := s.GetUserByUsername(ctx, "eve")
	if err := s.SetUserFrozen(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}

	c := newClient(t)
	c.Get(h.URL + "/console/login")
	csrf := csrfFromJar(t, c.Jar, h.URL)
	resp, err := c.PostForm(h.URL+"/console/login", url.Values{
		"username": {"eve"}, "password": {"supersecret"}, "_csrf": {csrf},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("frozen login status = %d, want 403", resp.StatusCode)
	}
	if !strings.Contains(readBody(t, resp), "冻结") {
		t.Fatal("expected frozen message")
	}

	// Unfreeze -> can log in again.
	if err := s.SetUserFrozen(ctx, u.ID, false); err != nil {
		t.Fatal(err)
	}
	c2 := newClient(t)
	loginAs(t, c2, h.URL, "eve", "supersecret")
	resp, _ = c2.Get(h.URL + "/console")
	if !strings.Contains(readBody(t, resp), "我的实例") {
		t.Fatal("unfrozen user should log in")
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
