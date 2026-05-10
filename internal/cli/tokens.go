package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo/pkg/api"
)

func tokensCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Manage per-app deploy tokens",
	}
	cmd.AddCommand(tokensListCmd(getClient))
	cmd.AddCommand(tokensCreateCmd(getClient))
	cmd.AddCommand(tokensDeleteCmd(getClient))
	return cmd
}

func tokensListCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "list [app]",
		Short: "List deploy tokens for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var tokens []api.TokenResp
			if err := cli.Get("/v1/apps/"+args[0]+"/tokens", &tokens); err != nil {
				return err
			}
			if out.JSON {
				out.Print(tokens)
				return nil
			}
			rows := make([][]string, 0, len(tokens))
			for _, t := range tokens {
				rows = append(rows, []string{
					fmt.Sprintf("%d", t.ID),
					t.Name,
					t.Kind,
					t.LastUsedAt,
				})
			}
			out.PrintTable([]string{"ID", "NAME", "KIND", "LAST_USED"}, rows)
			return nil
		},
	}
}

func tokensCreateCmd(getClient clientFactory) *cobra.Command {
	var tokenName string
	cmd := &cobra.Command{
		Use:   "create [app]",
		Short: "Create a deploy token for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var resp api.CreateTokenResp
			if err := cli.Post("/v1/apps/"+args[0]+"/tokens", api.CreateTokenReq{Name: tokenName}, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenName, "name", "", "optional label for the token")
	return cmd
}

func tokensDeleteCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [app] [id]",
		Short: "Revoke a deploy token",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			return cli.Delete("/v1/apps/" + args[0] + "/tokens/" + args[1])
		},
	}
}
