package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

// deployCmd returns the top-level `milo-apps-kit deploy` command.
func deployCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [app]",
		Short: "Deploy an image to an application",
		Long: `Deploy an image to an application.

If the image lives in a private registry (e.g. ghcr.io/your-org/repo for an
internal repo), the server needs credentials to pull it. Three ways to supply
them, in priority order:

  --gh-auth                                 use the local 'gh' CLI's session
                                            (--registry-user defaults to the
                                            authenticated GitHub login)
  --registry-user / --registry-token        explicit one-shot credentials
  (none)                                    fall back to the server's globally
                                            configured creds (or anonymous)`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			img, _ := c.Flags().GetString("image")
			commit, _ := c.Flags().GetString("commit")
			ref, _ := c.Flags().GetString("ref")
			ruser, _ := c.Flags().GetString("registry-user")
			rtoken, _ := c.Flags().GetString("registry-token")
			ghAuth, _ := c.Flags().GetBool("gh-auth")

			body := api.CreateDeploymentReq{Image: img, Commit: commit, Ref: ref}

			switch {
			case ghAuth:
				user, tok, err := ghAuthCreds()
				if err != nil {
					return err
				}
				if ruser != "" { // explicit user wins, but token still from gh
					user = ruser
				}
				body.RegistryAuth = &api.RegistryAuth{Username: user, Password: tok}
			case ruser != "" && rtoken != "":
				body.RegistryAuth = &api.RegistryAuth{Username: ruser, Password: rtoken}
			case ruser != "" || rtoken != "":
				return fmt.Errorf("--registry-user and --registry-token must be set together (or use --gh-auth)")
			}

			var resp api.DeploymentResp
			if err := cli.Post("/v1/apps/"+args[0]+"/deployments", body, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.Flags().String("image", "", "image reference (digest preferred)")
	_ = cmd.MarkFlagRequired("image")
	cmd.Flags().String("commit", "", "commit SHA (audit only)")
	cmd.Flags().String("ref", "", "git ref (audit only)")
	cmd.Flags().String("registry-user", "", "registry username for one-shot pull auth")
	cmd.Flags().String("registry-token", "", "registry password/token for one-shot pull auth")
	cmd.Flags().Bool("gh-auth", false, "use 'gh auth token' + GitHub login as registry credentials")
	return cmd
}

// ghAuthCreds shells out to the local 'gh' CLI to grab a registry username
// (the authenticated GitHub login) and a password (an OAuth token good for
// pulling from ghcr.io for repos that user can read).
func ghAuthCreds() (user, token string, err error) {
	user, err = runGh("api", "/user", "--jq", ".login")
	if err != nil {
		return "", "", fmt.Errorf("gh api /user: %w (is `gh` installed and authenticated?)", err)
	}
	token, err = runGh("auth", "token")
	if err != nil {
		return "", "", fmt.Errorf("gh auth token: %w", err)
	}
	return user, token, nil
}

func runGh(args ...string) (string, error) {
	c := exec.Command("gh", args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// deploymentsCmd returns `milo-apps-kit deployments` with list/get/cancel subcommands.
func deploymentsCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "Manage deployments",
	}

	list := &cobra.Command{
		Use:   "list [app]",
		Short: "List deployments for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var deps []api.DeploymentResp
			if err := cli.Get("/v1/apps/"+args[0]+"/deployments", &deps); err != nil {
				return err
			}
			if out.JSON {
				out.Print(deps)
				return nil
			}
			rows := make([][]string, 0, len(deps))
			for _, d := range deps {
				rows = append(rows, []string{
					fmt.Sprintf("%d", d.ID),
					d.Status,
					d.ImageRef,
					d.CreatedAt,
				})
			}
			out.PrintTable([]string{"ID", "STATUS", "IMAGE", "CREATED"}, rows)
			return nil
		},
	}
	cmd.AddCommand(list)

	get := &cobra.Command{
		Use:   "get [app] [id]",
		Short: "Get a specific deployment",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var d api.DeploymentResp
			if err := cli.Get("/v1/apps/"+args[0]+"/deployments/"+args[1], &d); err != nil {
				return err
			}
			out.Print(d)
			return nil
		},
	}
	cmd.AddCommand(get)

	cancel := &cobra.Command{
		Use:   "cancel [app] [id]",
		Short: "Cancel an in-flight deployment",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			return cli.Post("/v1/apps/"+args[0]+"/deployments/"+args[1]+"/cancel", nil, nil)
		},
	}
	cmd.AddCommand(cancel)

	return cmd
}
