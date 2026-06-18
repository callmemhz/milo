package docker

import "context"

// HostInfo summarises the docker host the control plane runs against.
type HostInfo struct {
	ServerVersion     string
	OperatingSystem   string
	Architecture      string
	NCPU              int
	MemTotal          int64 // bytes
	Containers        int
	ContainersRunning int
	Images            int
}

// Info returns daemon/host information.
func (c *Client) Info(ctx context.Context) (HostInfo, error) {
	in, err := c.cli.Info(ctx)
	if err != nil {
		return HostInfo{}, err
	}
	return HostInfo{
		ServerVersion:     in.ServerVersion,
		OperatingSystem:   in.OperatingSystem,
		Architecture:      in.Architecture,
		NCPU:              in.NCPU,
		MemTotal:          in.MemTotal,
		Containers:        in.Containers,
		ContainersRunning: in.ContainersRunning,
		Images:            in.Images,
	}, nil
}
