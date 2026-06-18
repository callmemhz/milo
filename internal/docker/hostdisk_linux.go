//go:build linux

package docker

import "golang.org/x/sys/unix"

func statfsBytes(path string) HostDisk {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return HostDisk{}
	}
	bs := uint64(st.Bsize)
	return HostDisk{
		Free:  st.Bavail * bs,
		Total: st.Blocks * bs,
		OK:    st.Blocks > 0,
	}
}
