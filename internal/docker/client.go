package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"

	dockerclient "github.com/docker/docker/client"
)

// Config holds configuration for the Docker client wrapper.
type Config struct {
	// Network is the name of the milo bridge network (default: "milo-net").
	Network string

	// RegistryUser and RegistryPassword are optional credentials for private
	// registries. If empty, public images are pulled without auth.
	RegistryUser     string
	RegistryPassword string
}

// Client wraps the Docker SDK client with Milo-specific helpers.
type Client struct {
	cli      *dockerclient.Client
	network  string
	pullAuth string // base64-encoded JSON {"username":…,"password":…}
}

// New creates a new Client using settings from the environment
// (DOCKER_HOST, DOCKER_TLS_VERIFY, DOCKER_CERT_PATH, DOCKER_API_VERSION)
// and negotiates the API version automatically.
func New(cfg Config) (*Client, error) {
	if cfg.Network == "" {
		cfg.Network = "milo-net"
	}

	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}

	c := &Client{
		cli:     cli,
		network: cfg.Network,
	}

	if cfg.RegistryUser != "" || cfg.RegistryPassword != "" {
		c.pullAuth = encodeAuth(cfg.RegistryUser, cfg.RegistryPassword)
	}

	return c, nil
}

// Close releases the underlying Docker client.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Network returns the configured bridge network name.
func (c *Client) Network() string {
	return c.network
}

// Ping verifies that the Docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// encodeAuth base64-encodes Docker registry credentials for API calls.
func encodeAuth(user, pass string) string {
	auth := map[string]string{"username": user, "password": pass}
	b, _ := json.Marshal(auth)
	return base64.URLEncoding.EncodeToString(b)
}
