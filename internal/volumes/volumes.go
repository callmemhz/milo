// Package volumes parses, validates, and translates the volume URIs declared
// in milo.yaml manifests.
//
// Phase 1 supports a single scheme:
//
//	cpfs://<fs-id>/<sub-path>
//
// which translates to a host bind mount at /mnt/cpfs/<fs-id>/<sub-path>.
// The platform admin is expected to pre-mount each fs-id on the host as a
// regular directory tree under that root.
//
// The full manifest string form is "<source-uri>:<target>[:opts]", e.g.
//
//	cpfs://train-fs/datasets/foo:/data:ro
package volumes

import (
	"fmt"
	"path"
	"strings"

	"github.com/callmemhz/milo/pkg/api"
)

const (
	SchemeCPFS = "cpfs"

	// CPFSMountRoot is the platform-reserved host path under which CPFS file
	// systems are pre-mounted (one subdirectory per fs-id).
	CPFSMountRoot = "/mnt/cpfs"
)

// ParseString parses a manifest-form volume string ("source-uri:target[:opts]")
// into a VolumeSpec.
func ParseString(s string) (api.VolumeSpec, error) {
	if s == "" {
		return api.VolumeSpec{}, fmt.Errorf("volume: empty string")
	}
	schemeEnd := strings.Index(s, "://")
	if schemeEnd == -1 {
		return api.VolumeSpec{}, fmt.Errorf("volume %q: missing scheme (expected e.g. cpfs://...)", s)
	}
	scheme := s[:schemeEnd]
	rest := s[schemeEnd+3:]

	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 2 {
		return api.VolumeSpec{}, fmt.Errorf("volume %q: missing target path (form: scheme://...:/container/path[:ro])", s)
	}

	spec := api.VolumeSpec{
		Source: scheme + "://" + parts[0],
		Target: parts[1],
	}

	if len(parts) == 3 && parts[2] != "" {
		for _, opt := range strings.Split(parts[2], ",") {
			switch strings.TrimSpace(opt) {
			case "ro":
				spec.ReadOnly = true
			case "rw":
				spec.ReadOnly = false
			case "":
				// trailing comma; ignore
			default:
				return api.VolumeSpec{}, fmt.Errorf("volume %q: unknown option %q (allowed: ro, rw)", s, opt)
			}
		}
	}

	if err := Validate(spec); err != nil {
		return api.VolumeSpec{}, err
	}
	return spec, nil
}

// Validate checks a VolumeSpec for structural errors. It is safe to call on
// specs received over the wire — server handlers should call this before
// persisting.
func Validate(spec api.VolumeSpec) error {
	if spec.Source == "" {
		return fmt.Errorf("volume: empty source")
	}
	if spec.Target == "" {
		return fmt.Errorf("volume %q: empty target", spec.Source)
	}
	if !path.IsAbs(spec.Target) {
		return fmt.Errorf("volume %q: target %q must be an absolute path", spec.Source, spec.Target)
	}
	if hasDotDot(spec.Target) {
		return fmt.Errorf("volume %q: target %q must not contain '..'", spec.Source, spec.Target)
	}

	scheme, body, err := splitScheme(spec.Source)
	if err != nil {
		return err
	}
	switch scheme {
	case SchemeCPFS:
		return validateCPFS(body)
	default:
		return fmt.Errorf("volume %q: unsupported scheme %q (supported: cpfs)", spec.Source, scheme)
	}
}

// HostPath returns the host filesystem path that the spec's source resolves
// to. The returned path is what should be passed as the bind-mount source to
// Docker.
func HostPath(spec api.VolumeSpec) (string, error) {
	scheme, body, err := splitScheme(spec.Source)
	if err != nil {
		return "", err
	}
	switch scheme {
	case SchemeCPFS:
		fsID, sub, err := parseCPFSBody(body)
		if err != nil {
			return "", err
		}
		return path.Join(CPFSMountRoot, fsID, sub), nil
	default:
		return "", fmt.Errorf("volume %q: unsupported scheme %q", spec.Source, scheme)
	}
}

func splitScheme(uri string) (scheme, body string, err error) {
	i := strings.Index(uri, "://")
	if i == -1 {
		return "", "", fmt.Errorf("volume %q: missing scheme", uri)
	}
	return uri[:i], uri[i+3:], nil
}

// validateCPFS enforces: <fs-id>/<sub-path>. fs-id must be a single
// non-empty path segment without separators or traversal. sub-path may be
// empty (mount the fs root) but must not contain '..'.
func validateCPFS(body string) error {
	fsID, sub, err := parseCPFSBody(body)
	if err != nil {
		return err
	}
	if fsID == "" {
		return fmt.Errorf("cpfs:// requires an fs-id (cpfs://<fs-id>/<sub-path>)")
	}
	if strings.ContainsAny(fsID, "/\\") || fsID == "." || fsID == ".." {
		return fmt.Errorf("cpfs:// fs-id %q must be a single path segment", fsID)
	}
	if hasDotDot(sub) {
		return fmt.Errorf("cpfs:// sub-path %q must not contain '..'", sub)
	}
	return nil
}

func parseCPFSBody(body string) (fsID, sub string, err error) {
	if body == "" {
		return "", "", fmt.Errorf("cpfs:// requires an fs-id (cpfs://<fs-id>/<sub-path>)")
	}
	if strings.HasPrefix(body, "/") {
		return "", "", fmt.Errorf("cpfs://<fs-id>... — fs-id must precede any '/' (got %q)", body)
	}
	slash := strings.Index(body, "/")
	if slash == -1 {
		return body, "", nil
	}
	return body[:slash], body[slash+1:], nil
}

func hasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}
