//go:build !linux

package docker

// statfsBytes is unavailable off-Linux (e.g. local macOS dev builds). The
// control plane runs in a Linux container in production, where the linux build
// applies.
func statfsBytes(string) HostDisk { return HostDisk{} }
