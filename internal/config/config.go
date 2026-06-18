package config

import (
	"github.com/kelseyhightower/envconfig"
)

// Server holds runtime configuration loaded from environment variables.
type Server struct {
	HTTPAddr   string `envconfig:"HTTP_ADDR"    default:":8000"`
	StateDir   string `envconfig:"STATE_DIR"    default:"/var/lib/milo"`
	RootDomain string `envconfig:"ROOT_DOMAIN"  required:"true"`
	APIDomain  string `envconfig:"API_DOMAIN"   required:"true"`
	Network    string `envconfig:"MILO_NETWORK" default:"milo-net"`
	GHCRUser   string `envconfig:"GHCR_USER"`
	GHCRToken  string `envconfig:"GHCR_TOKEN"`
	Version    string `envconfig:"MILO_VERSION" default:"dev"`
	// CookieSecure marks console session cookies Secure. Enable in production
	// (served over TLS via Caddy); leave off for local plain-HTTP testing.
	CookieSecure bool `envconfig:"COOKIE_SECURE" default:"false"`
}

func Load() (Server, error) {
	var c Server
	err := envconfig.Process("", &c)
	return c, err
}
