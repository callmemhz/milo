package server

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/deploy"
	"github.com/callmemhz/milo/internal/store"
)

// Deployer is the interface the server uses to trigger deploys. The concrete
// *deploy.Orchestrator satisfies this interface; tests inject a fakeDeployer.
type Deployer interface {
	Deploy(ctx context.Context, req deploy.DeployRequest) (store.Deployment, error)
	Restart(ctx context.Context, appID int64) (store.Deployment, error)
	DeleteApp(ctx context.Context, appID int64, deleteVolume bool) error
	ProvisionAddon(ctx context.Context, addonID int64) error
	DeleteAddon(ctx context.Context, addonID int64, deleteVolume bool) error
	Locks() *deploy.LockManager
}

// LogStreamer is the interface the server uses to stream container logs.
// The concrete *docker.Client satisfies this; tests inject a fakeLogStreamer.
type LogStreamer interface {
	Logs(ctx context.Context, name string, follow bool, tail string) (io.ReadCloser, error)
}

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	Store    *store.Store
	Auth     *auth.Authenticator
	Deployer Deployer    // set by main; nil in unit tests
	Docker   LogStreamer // set by main; nil in unit tests
	Version  string
	// RootDomain is the public DNS root for apps and exposed addons
	// (e.g. app.example.com). Used to build exposed-addon external URLs.
	RootDomain string

	// Runtime backs the web console's live uptime/load views. Set by main to the
	// docker client; nil in unit tests (console degrades to state-only).
	Runtime ContainerRuntime
	// CookieSecure marks session cookies Secure (set in production behind TLS).
	CookieSecure bool

	tmpls map[string]*template.Template // parsed lazily by initConsole
}

func New(s *store.Store, version string) *Server {
	return &Server{Store: s, Auth: &auth.Authenticator{Store: s}, Version: version}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Get("/v1/healthz", s.handleHealthz)
	r.Get("/v1/version", s.handleVersion)

	s.registerConsoleRoutes(r)

	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Get("/v1/auth/whoami", s.handleWhoami)
		s.registerUsersRoutes(r)
		s.registerAppsRoutes(r)
		s.registerEnvRoutes(r)
		s.registerDeploymentsRoutes(r)
		s.registerRuntimeRoutes(r)
		s.registerAppTokensRoutes(r)
		s.registerAddonsRoutes(r)
	})

	return r
}

// maybeRedeployCurrent triggers a best-effort rolling redeploy in the background.
// If the app has no current deploy, or Deployer is not wired, this is a no-op.
func (s *Server) maybeRedeployCurrent(ctx context.Context, appID int64) {
	if s.Deployer == nil {
		return
	}
	a, err := s.Store.GetAppByID(ctx, appID)
	if err != nil || a.CurrentDeployID == nil {
		return
	}
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_, _ = s.Deployer.Restart(bg, appID)
	}()
}
