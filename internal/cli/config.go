package cli

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Context holds the endpoint and auth token for a named server context.
type Context struct {
	Endpoint string `yaml:"endpoint"`
	Token    string `yaml:"token"`
}

// Config is the top-level config file structure.
type Config struct {
	CurrentContext string             `yaml:"current_context"`
	Contexts       map[string]Context `yaml:"contexts"`
}

// ConfigPath returns the path to the config file, honoring MILO_CONFIG env override.
func ConfigPath() string {
	if v := os.Getenv("MILO_CONFIG"); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "milo", "config.yaml")
}

// LoadConfig reads the config file. Returns an empty config (not an error) when
// the file does not exist yet.
func LoadConfig() (*Config, error) {
	p := ConfigPath()
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Contexts: map[string]Context{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Contexts == nil {
		c.Contexts = map[string]Context{}
	}
	return &c, nil
}

// SaveConfig writes the config to disk, creating directories as needed.
func SaveConfig(c *Config) error {
	p := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Active returns the currently selected context and whether it exists.
func (c *Config) Active() (Context, bool) {
	ctx, ok := c.Contexts[c.CurrentContext]
	return ctx, ok
}
