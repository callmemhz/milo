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
	Info(ctx context.Context) (docker.HostInfo, error)
	DiskUsage(ctx context.Context) (docker.DiskUsage, error)
	VolumeSize(ctx context.Context, name string) (int64, bool)
	ImageList(ctx context.Context) ([]docker.Image, error)
	ImageRemove(ctx context.Context, id string, force bool) error
	ImageUsage(ctx context.Context) (map[string][]string, error)
	VolumeList(ctx context.Context) ([]docker.VolumeInfo, error)
	VolumeRemove(ctx context.Context, name string, force bool) error
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
		"meter":         meterHTML,
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
	r.Get("/console/lang", s.handleSetLang)
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
		r.Get("/console/addons/{addon}", s.consoleAddonDetail)
		r.Get("/console/addons/{addon}/logs/stream", s.consoleAddonLogsStream)
		r.Get("/console/addons/{addon}/stats/stream", s.consoleAddonStatsStream)
		r.Post("/console/addons/{addon}/expose", s.consoleAddonExpose)
		// Self-service account / CLI tokens (any signed-in user).
		r.Get("/console/account", s.consoleAccount)
		r.Post("/console/account/tokens", s.consoleAccountTokenCreate)
		r.Post("/console/account/tokens/revoke", s.consoleAccountTokenRevoke)
	})

	// Admin-only pages.
	r.Group(func(r chi.Router) {
		r.Use(s.requireSession, s.requireAdminPage)
		r.Get("/console/admin", s.consoleAdmin)
		r.Get("/console/admin/instances", s.consoleAllInstances)
		r.Get("/console/admin/images", s.consoleImages)
		r.Post("/console/admin/images/delete", s.consoleImageDelete)
		r.Get("/console/admin/volumes", s.consoleVolumes)
		r.Post("/console/admin/volumes/delete", s.consoleVolumeDelete)
		r.Get("/console/users", s.consoleUsers)
		r.Post("/console/users/create", s.consoleUserCreate)
		r.Post("/console/users/password", s.consoleUserSetPassword)
		r.Post("/console/users/freeze", s.consoleUserFreeze)
		r.Post("/console/users/delete", s.consoleUserDelete)
		r.Post("/console/users/token", s.consoleUserToken)
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

// requireAdminPage gates admin-only console pages. Runs after requireSession,
// so an identity is present; non-admins get a 403.
func (s *Server) requireAdminPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.IdentityFromContext(r.Context())
		if id == nil || id.User == nil || !id.User.IsAdmin {
			http.Error(w, "forbidden — admin only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
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
	// A frozen account cannot use the console; drop all its sessions.
	if frozen, _ := s.Store.IsUserFrozen(r.Context(), u.ID); frozen {
		_ = s.Store.DeleteUserSessions(r.Context(), u.ID)
		s.clearSessionCookie(w)
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

// render executes a page template wrapped in the base layout, injecting the
// per-request translator (T) and current language.
func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	t, ok := s.tmpls[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	lang := langFromRequest(r)
	data["Lang"] = lang
	data["T"] = func(key string) string { return translate(lang, key) }
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
	Owners string // comma-joined; shown to admins
}

type addonCard struct {
	Name    string
	Engine  string
	Status  string
	Uptime  string
	Mem     string
	Exposed bool
	Owners  string // comma-joined; shown to admins
}

// ownerNames returns a comma-joined owner username list (best-effort).
func ownerNames(users []store.User) string {
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Username)
	}
	return strings.Join(names, ", ")
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

// meterHTML renders a labelled percentage/ratio bar. pct drives both the fill
// width (clamped 0..100) and the color band. num is the human text on the right.
func meterHTML(label string, pct float64, num string) template.HTML {
	w := pct
	if w < 0 {
		w = 0
	}
	if w > 100 {
		w = 100
	}
	lvl := "ok"
	if pct >= 85 {
		lvl = "bad"
	} else if pct >= 60 {
		lvl = "warn"
	}
	return template.HTML(fmt.Sprintf(
		`<div class="meter"><span class="mlbl">%s</span><span class="bar %s"><i style="width:%.0f%%"></i></span><span class="num">%s</span></div>`,
		template.HTMLEscapeString(label), lvl, w, template.HTMLEscapeString(num)))
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
