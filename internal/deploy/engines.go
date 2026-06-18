package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Engine describes a platform-managed addon kind (postgres, redis). The
// platform owns the image catalog: users pick an engine + major version, never
// a raw image ref.
type Engine struct {
	Name           string
	DefaultVersion string
	Images         map[string]string // major version → pinned image ref
	Port           int
	DataTarget     string // where the data volume is mounted
	DefaultEnvKey  string // env var injected into linked apps when alias is empty
}

// addonUser and addonDB are the fixed credentials namespace inside a
// postgres addon. v1 keeps one user/db per addon; per-link credentials are
// a future iteration.
const (
	addonUser = "app"
	addonDB   = "app"
)

var engines = map[string]Engine{
	"postgres": {
		Name:           "postgres",
		DefaultVersion: "16",
		Images: map[string]string{
			"14": "postgres:14-alpine",
			"15": "postgres:15-alpine",
			"16": "postgres:16-alpine",
			"17": "postgres:17-alpine",
		},
		Port:          5432,
		DataTarget:    "/var/lib/postgresql/data",
		DefaultEnvKey: "DATABASE_URL",
	},
	"redis": {
		Name:           "redis",
		DefaultVersion: "7",
		Images: map[string]string{
			"6": "redis:6-alpine",
			"7": "redis:7-alpine",
			"8": "redis:8-alpine",
		},
		Port:          6379,
		DataTarget:    "/data",
		DefaultEnvKey: "REDIS_URL",
	},
}

// LookupEngine returns the engine catalog entry, resolving an empty version to
// the default. Errors on unknown engine or unsupported version.
func LookupEngine(name, version string) (Engine, string, error) {
	e, ok := engines[name]
	if !ok {
		return Engine{}, "", fmt.Errorf("unknown engine %q (supported: %s)", name, strings.Join(EngineNames(), ", "))
	}
	if version == "" {
		version = e.DefaultVersion
	}
	if _, ok := e.Images[version]; !ok {
		versions := make([]string, 0, len(e.Images))
		for v := range e.Images {
			versions = append(versions, v)
		}
		sort.Strings(versions)
		return Engine{}, "", fmt.Errorf("unsupported %s version %q (supported: %s)", name, version, strings.Join(versions, ", "))
	}
	return e, version, nil
}

// EngineNames returns the supported engine names, sorted.
func EngineNames() []string {
	names := make([]string, 0, len(engines))
	for n := range engines {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ConnectionURL builds the URL injected into linked apps. The host is the
// addon name, which resolves via Docker DNS on the per-addon network.
func ConnectionURL(engineName, addonName, password string) string {
	switch engineName {
	case "postgres":
		return fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", addonUser, password, addonName, addonDB)
	case "redis":
		return fmt.Sprintf("redis://:%s@%s:6379/0", password, addonName)
	}
	return ""
}

// ExternalConnectionURL builds the connection URL for reaching an exposed
// addon from outside the host. The host is <addon>.<rootDomain> (covered by
// the same wildcard DNS record as apps) and hostPort is the published host
// port. Unlike ConnectionURL, the port is the host port, not the engine's
// in-container port. Returns "" if the addon is not exposed (hostPort == 0).
func ExternalConnectionURL(engineName, addonName, password, rootDomain string, hostPort int64) string {
	if hostPort == 0 || rootDomain == "" {
		return ""
	}
	host := fmt.Sprintf("%s.%s", addonName, rootDomain)
	switch engineName {
	case "postgres":
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", addonUser, password, host, hostPort, addonDB)
	case "redis":
		return fmt.Sprintf("redis://:%s@%s:%d/0", password, host, hostPort)
	}
	return ""
}

// LinkEnvKey computes the env var name a link injects: <ALIAS>_URL when an
// alias is set, otherwise the engine default (DATABASE_URL / REDIS_URL).
func LinkEnvKey(engineName, alias string) string {
	if alias != "" {
		return alias + "_URL"
	}
	if e, ok := engines[engineName]; ok {
		return e.DefaultEnvKey
	}
	return ""
}

// bootEnv returns the env the addon container is started with.
func (e Engine) bootEnv(password string) map[string]string {
	switch e.Name {
	case "postgres":
		return map[string]string{
			"POSTGRES_USER":     addonUser,
			"POSTGRES_DB":       addonDB,
			"POSTGRES_PASSWORD": password,
		}
	}
	return nil
}

// bootCmd returns the command override for the addon container.
func (e Engine) bootCmd(password string) []string {
	switch e.Name {
	case "redis":
		return []string{"redis-server", "--requirepass", password, "--appendonly", "yes"}
	}
	return nil
}

// readyCmd returns the in-container readiness probe command.
func (e Engine) readyCmd(password string) []string {
	switch e.Name {
	case "postgres":
		return []string{"pg_isready", "-U", addonUser, "-d", addonDB}
	case "redis":
		return []string{"redis-cli", "-a", password, "--no-auth-warning", "ping"}
	}
	return nil
}

// GeneratePassword returns a 32-char hex password, URL-safe by construction.
func GeneratePassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AddonNetworkName is the per-addon isolated bridge network.
func AddonNetworkName(addonName string) string {
	return "milo-addon-" + addonName
}

// AddonVolumeName is the named volume holding the addon's data.
func AddonVolumeName(addonName string) string {
	return fmt.Sprintf("milo-addon-%s-data", addonName)
}

// AddonContainerName is the stable container name for an addon. Unlike
// apps, addons are recreated in place (stateful — no blue/green), so the
// name does not embed a timestamp.
func AddonContainerName(addonName string) string {
	return "addon-" + addonName
}
