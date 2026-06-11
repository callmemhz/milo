package deploy

import (
	"context"
	"time"

	"github.com/callmemhz/milo/internal/docker"
	"github.com/callmemhz/milo/internal/store"
	"github.com/callmemhz/milo/pkg/api"
)

// addonReadyTimeout bounds the in-container readiness probe. First boot of
// postgres includes initdb, which can take a while on slow disks.
const addonReadyTimeout = 90 * time.Second

// addonLockKey namespaces addon locks away from app locks, so an app and
// an addon sharing a name cannot deadlock or supersede each other.
func addonLockKey(name string) string { return "addon:" + name }

// ProvisionAddon pulls the engine image and (re)creates the addon
// container on its own isolated network, then waits for readiness. It is also
// the restart path: any existing container is stopped and recreated in place.
// Addons are stateful, so there is no blue/green — a restart means brief
// downtime while the volume is reattached.
func (o *Orchestrator) ProvisionAddon(ctx context.Context, addonID int64) error {
	addon, err := o.Store.GetAddonByID(ctx, addonID)
	if err != nil {
		return err
	}
	handle, err := o.LockMgr.Acquire(ctx, addonLockKey(addon.Name))
	if err != nil {
		return err
	}
	defer handle.Release()

	eng, version, err := LookupEngine(addon.Engine, addon.Version)
	if err != nil {
		return &api.Error{Code: api.ErrInvalid, Message: err.Error()}
	}
	image := eng.Images[version]
	containerName := AddonContainerName(addon.Name)
	netName := AddonNetworkName(addon.Name)

	_ = o.Store.UpdateAddonStatus(ctx, addon.ID, store.AddonProvisioning, containerName)

	if _, err := o.Docker.Pull(ctx, image, nil); err != nil {
		return o.failAddon(ctx, addon.ID, containerName, "pull_failed", err)
	}
	if err := o.Docker.EnsureNetworkNamed(ctx, netName, map[string]string{
		"milo.managed": "true",
		"milo.addon":   addon.Name,
	}); err != nil {
		return o.failAddon(ctx, addon.ID, containerName, "docker_error", err)
	}
	if err := o.Docker.EnsureVolume(ctx, AddonVolumeName(addon.Name)); err != nil {
		return o.failAddon(ctx, addon.ID, containerName, "docker_error", err)
	}

	// Recreate in place: drop any previous container, keep the volume.
	_ = o.Docker.Stop(ctx, containerName, 10)
	_ = o.Docker.Remove(ctx, containerName)

	if _, err := o.Docker.Run(ctx, docker.RunSpec{
		Name:         containerName,
		Alias:        addon.Name,
		Image:        image,
		Env:          eng.bootEnv(addon.Password),
		Cmd:          eng.bootCmd(addon.Password),
		CPULimit:     addon.CpuLimit,
		MemoryMB:     addon.MemoryLimitMb,
		VolumeSrc:    AddonVolumeName(addon.Name),
		VolumeTarget: eng.DataTarget,
		Network:      netName,
		Labels:       map[string]string{"milo.addon": addon.Name},
	}); err != nil {
		return o.failAddon(ctx, addon.ID, containerName, "docker_error", err)
	}

	if err := o.Docker.ExecProbe(ctx, containerName, eng.readyCmd(addon.Password), addonReadyTimeout); err != nil {
		_ = o.Docker.Stop(ctx, containerName, 5)
		_ = o.Docker.Remove(ctx, containerName)
		return o.failAddon(ctx, addon.ID, containerName, "readiness_failed", err)
	}

	return o.Store.UpdateAddonStatus(ctx, addon.ID, store.AddonRunning, containerName)
}

func (o *Orchestrator) failAddon(ctx context.Context, addonID int64, containerName, reason string, cause error) error {
	_ = o.Store.UpdateAddonStatus(ctx, addonID, store.AddonFailed, containerName)
	return &api.Error{Code: api.ErrDeployFailed, Message: reason, Details: cause.Error()}
}

// DeleteAddon tears down the container and network, removes link rows, and
// soft-deletes the addon. Linked apps keep running until their next deploy,
// which drops the injected env and network attachment; the caller is expected
// to trigger those redeploys. With deleteVolume the data is gone for good.
func (o *Orchestrator) DeleteAddon(ctx context.Context, addonID int64, deleteVolume bool) error {
	addon, err := o.Store.GetAddonByID(ctx, addonID)
	if err != nil {
		return err
	}
	handle, err := o.LockMgr.Acquire(ctx, addonLockKey(addon.Name))
	if err != nil {
		return err
	}
	defer handle.Release()

	containerName := AddonContainerName(addon.Name)
	_ = o.Docker.Stop(ctx, containerName, 10)
	_ = o.Docker.Remove(ctx, containerName)
	_ = o.Docker.ForceRemoveNetwork(ctx, AddonNetworkName(addon.Name))
	if deleteVolume {
		_ = o.Docker.RemoveVolume(ctx, AddonVolumeName(addon.Name), true)
	}
	if err := o.Store.DeleteLinksForAddon(ctx, addonID); err != nil {
		return err
	}
	return o.Store.SoftDeleteAddon(ctx, addonID)
}
