package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

// deployCmd returns the top-level `milo-apps-kit deploy` command.
func deployCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [app]",
		Short: "Deploy an image to an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			img, _ := c.Flags().GetString("image")
			commit, _ := c.Flags().GetString("commit")
			ref, _ := c.Flags().GetString("ref")
			var resp api.DeploymentResp
			if err := cli.Post("/v1/apps/"+args[0]+"/deployments",
				api.CreateDeploymentReq{Image: img, Commit: commit, Ref: ref}, &resp); err != nil {
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
	return cmd
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
