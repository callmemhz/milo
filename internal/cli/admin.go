package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func adminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative utilities",
	}
	bs := &cobra.Command{
		Use:   "bootstrap",
		Short: "Show how to obtain the bootstrap admin token",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Fprintln(c.OutOrStdout(), "On first server start, milod prints `BOOTSTRAP_ADMIN_TOKEN=<token>` to stderr.")
			fmt.Fprintln(c.OutOrStdout(), "Capture it from your server logs (e.g. `docker compose logs milo-control-plane`)")
			fmt.Fprintln(c.OutOrStdout(), "then run `milo auth login --endpoint=https://milo.example.com --token=<token>`.")
			return nil
		},
	}
	cmd.AddCommand(bs)
	return cmd
}
