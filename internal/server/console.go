package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/docker"
	"github.com/callmemhz/milo/internal/store"
)

// ContainerRuntime is the subset of the docker client the console needs for
// live introspection (uptime + load). Kept separate from LogStreamer so
// existing tests that only fake Logs keep working.
type ContainerRuntime interface {
	InspectByName(ctx context.Context, name string) (*docker.ContainerInfo, error)
	SampleStats(ctx context.Context, name string) (docker.Stats, error)
	StatsStream(ctx context.Context, name string) (io.ReadCloser, error)
}

const (
	sessionCookie = "milo_session"
	csrfCookie    = "milo_csrf"
	sessionTTL    = 12 * time.Hour
)

//go:embed templates/*.tmpl templates/pages/*.tmpl
var templateFS embed.FS

//go:embed assets/*
var assetFS embed.FS

// initConsole parses templates. Called lazily from the router so unit tests that
// never hit the console don't pay for it.
func (s *Server) initConsole() {
	if s.tmpls != nil {
		return
	}
	fm := template.FuncMap{
		"humanDuration": humanDuration,
		"humanBytes":    humanBytes,
		"pct":           func(f float64) string { return fmt.Sprintf("%.1f", f) },
	}
	pages, _ := fs.Glob(templateFS, "templates/pages/*.tmpl")
	s.tmpls = map[string]*template.Template{}
	for _, p := range pages {
		name := strings.TrimSuffix(p[len("templates/pages/"):], ".tmpl")
		t := template.Must(template.New("").Funcs(fm).ParseFS(templateFS, "templates/layout.tmpl", p))
		s.tmpls[name] = t
	}
}

func (s *Server) registerConsoleRoutes(r chi.Router) {
	s.initConsole()

	sub, _ := fs.Sub(assetFS, "assets")
	r.Handle("/console/assets/*", http.StripPrefix("/console/assets/", http.FileServer(http.FS(sub))))

	// Public (no session required).
	r.Get("/console/login", s.consoleLoginForm)
	r.Post("/console/login", s.consoleLoginSubmit)
	r.Post("/console/logout", s.consoleLogout)

	// Authenticated console pages.
	r.Group(func(r chi.Router) {
		r.Use(s.requireSession)
		r.Get("/console", s.consoleDashboard)
		r.Get("/console/apps/{app}", s.consoleAppDetail)
		r.Get("/console/apps/{app}/logs/stream", s.consoleAppLogsStream)
		r.Get("/console/apps/{app}/stats/stream", s.consoleAppStatsStream)
	})

	// Root redirect for convenience.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/console", http.StatusFound)
	})
}

// requireSession resolves the browser session cookie into an auth.Identity and
// stores it in the request context, exactly like the Bearer middleware does for
// the API. On failure it redirects to the login page.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.resolveSession(w, r)
		if !ok {
			http.Redirect(w, r, "/console/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), id)))
	})
}

func (s *Server) resolveSession(w http.ResponseWriter, r *http.Request) (*auth.Identity, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, false
	}
	sess, err := s.Store.GetSessionByHash(r.Context(), auth.Hash(c.Value))
	if err != nil {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.Store.DeleteSession(r.Context(), sess.TokenHash)
		s.clearSessionCookie(w)
		return nil, false
	}
	u, err := s.Store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		return nil, false
	}
	// Sliding expiry (best-effort).
	_ = s.Store.RefreshSession(r.Context(), sess.ID, time.Now().Add(sessionTTL))
	return &auth.Identity{User: &u}, true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// render executes a page template wrapped in the base layout.
func (s *Server) render(w http.ResponseWriter, page string, data map[string]any) {
	t, ok := s.tmpls[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		// Headers may already be sent; log-and-move-on.
		_, _ = w.Write([]byte("<!-- render error: " + template.HTMLEscapeString(err.Error()) + " -->"))
	}
}

// --- view models / helpers ---------------------------------------------------

type appCard struct {
	Name   string
	State  string
	Uptime string
	Mem    string
	CPU    string
}

type addonCard struct {
	Name    string
	Engine  string
	Status  string
	Uptime  string
	Mem     string
	Exposed bool
}

// currentContainer returns the running container name for an app, or "".
func (s *Server) currentContainer(ctx context.Context, a store.App) string {
	if a.CurrentDeployID == nil {
		return ""
	}
	d, err := s.Store.GetDeployment(ctx, *a.CurrentDeployID)
	if err != nil || d.ContainerName == nil {
		return ""
	}
	return *d.ContainerName
}

// inspectCard fills uptime/mem for a container name (best-effort, runtime may be nil).
func (s *Server) inspectCard(ctx context.Context, name string) (state, uptime, mem string) {
	if name == "" || s.Runtime == nil {
		return "down", "", ""
	}
	info, err := s.Runtime.InspectByName(ctx, name)
	if err != nil {
		return "down", "", ""
	}
	state = info.State
	if t, perr := time.Parse(time.RFC3339Nano, info.StartedAt); perr == nil && !t.IsZero() && info.State == "running" {
		uptime = humanDuration(time.Since(t))
	}
	if st, serr := s.Runtime.SampleStats(ctx, name); serr == nil {
		mem = humanBytes(st.MemoryUsage)
	}
	return state, uptime, mem
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
