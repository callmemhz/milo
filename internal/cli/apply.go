package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo/internal/manifest"
	"github.com/callmemhz/milo/pkg/api"
)

// applyCmd reads a milo.yaml manifest, upserts the app's spec, and triggers
// a deployment of the manifest's image. Each call always triggers a new
// deployment — the manifest is treated as the desired-state.
func applyCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a milo.yaml manifest: upsert app spec and deploy",
		Long: `Read a milo.yaml manifest, sync the app's long-lived spec
(port, resources, volumes), then create a deployment of the manifest's image.

Apply always triggers a new deployment, even if the manifest hasn't changed
since the last apply — the manifest describes the desired state, and apply
reconciles the live app to it.

Private registry credentials (e.g. for ghcr.io private packages) must be
passed explicitly via --registry-user/--registry-token. They are not stored
in the manifest.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			file, _ := c.Flags().GetString("file")
			if file == "" {
				file = "milo.yaml"
			}
			m, err := manifest.Load(file)
			if err != nil {
				return err
			}

			// Sync app spec (create or update).
			if err := upsertApp(cli, m); err != nil {
				return err
			}

			// Sync env vars from manifest (overlay; doesn't unset extras).
			if len(m.Env) > 0 {
				var current map[string]string
				if err := cli.Patch("/v1/apps/"+m.Name+"/env", api.EnvPatchReq{Set: m.Env}, &current); err != nil {
					return err
				}
			}

			// Build deployment request. --image overrides manifest's image
			// (useful for CI pinning to a per-commit sha tag without editing
			// milo.yaml).
			image := m.Image
			if v, _ := c.Flags().GetString("image"); v != "" {
				image = v
			}
			body := api.CreateDeploymentReq{Image: image}
			ruser, _ := c.Flags().GetString("registry-user")
			rtoken, _ := c.Flags().GetString("registry-token")
			if ruser != "" && rtoken != "" {
				body.RegistryAuth = &api.RegistryAuth{Username: ruser, Password: rtoken}
			} else if ruser != "" || rtoken != "" {
				return fmt.Errorf("--registry-user and --registry-token must be set together")
			}
			if commit, _ := c.Flags().GetString("commit"); commit != "" {
				body.Commit = commit
			}
			if ref, _ := c.Flags().GetString("ref"); ref != "" {
				body.Ref = ref
			}

			var resp api.DeploymentResp
			if err := cli.Post("/v1/apps/"+m.Name+"/deployments", body, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.Flags().String("file", "milo.yaml", "path to manifest file")
	cmd.Flags().String("image", "", "override manifest's image: field (e.g. for CI pinning to a per-commit sha)")
	cmd.Flags().String("registry-user", "", "registry username (for private image pull)")
	cmd.Flags().String("registry-token", "", "registry password/token (for private image pull)")
	cmd.Flags().String("commit", "", "commit SHA (audit only)")
	cmd.Flags().String("ref", "", "git ref (audit only)")
	return cmd
}

// upsertApp GETs the app and either creates it (404) or PATCHes its spec
// to match the manifest. It does not deploy.
func upsertApp(cli *Client, m *manifest.Manifest) error {
	var existing api.AppResp
	err := cli.Get("/v1/apps/"+m.Name, &existing)
	switch {
	case err == nil:
		return patchApp(cli, m, existing)
	case isNotFound(err):
		return createApp(cli, m)
	default:
		return err
	}
}

func createApp(cli *Client, m *manifest.Manifest) error {
	req := api.CreateAppReq{
		Name:          m.Name,
		Port:          m.Port,
		CPULimit:      m.Resources.CPU,
		MemoryLimitMB: m.Resources.Memory,
		Volumes:       m.VolumeSpecs(),
	}
	var resp api.AppResp
	return cli.Post("/v1/apps", req, &resp)
}

func patchApp(cli *Client, m *manifest.Manifest, existing api.AppResp) error {
	req := api.UpdateAppReq{}
	// PATCH only fields the manifest declares non-zero; manifest treats
	// volumes as authoritative (always sent), so cleared volumes mean
	// "remove all mounts".
	if m.Port > 0 && m.Port != existing.Port {
		p := m.Port
		req.Port = &p
	}
	if m.Resources.CPU > 0 && m.Resources.CPU != existing.CPULimit {
		v := m.Resources.CPU
		req.CPULimit = &v
	}
	if m.Resources.Memory > 0 && m.Resources.Memory != existing.MemoryLimitMB {
		v := m.Resources.Memory
		req.MemoryLimitMB = &v
	}
	specs := m.VolumeSpecs()
	if !volumesEqual(existing.Volumes, specs) {
		req.Volumes = &specs
	}
	// Nothing to patch? Skip the call.
	if req.Port == nil && req.CPULimit == nil && req.MemoryLimitMB == nil && req.Volumes == nil {
		return nil
	}
	var resp api.AppResp
	return cli.Patch("/v1/apps/"+m.Name, req, &resp)
}

func volumesEqual(a, b []api.VolumeSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isNotFound(err error) bool {
	var ae *api.Error
	if errors.As(err, &ae) {
		return ae.Code == api.ErrNotFound
	}
	return false
}
