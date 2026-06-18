package docker

// HostDisk is filesystem capacity for the docker host data directory.
type HostDisk struct {
	Free  uint64
	Total uint64
	OK    bool
}

// ReadHostDisk reports free/total bytes for the filesystem backing path.
// It probes the given path, falling back to "/". Implementation is per-OS.
func ReadHostDisk(path string) HostDisk {
	if d := statfsBytes(path); d.OK {
		return d
	}
	return statfsBytes("/")
}
