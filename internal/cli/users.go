package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo/pkg/api"
)

func usersCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage users (admin only)",
	}
	cmd.AddCommand(usersCreateCmd(getClient))
	cmd.AddCommand(usersListCmd(getClient))
	cmd.AddCommand(usersDeleteCmd(getClient))
	cmd.AddCommand(usersTokenCmd(getClient))
	return cmd
}

func usersCreateCmd(getClient clientFactory) *cobra.Command {
	var isAdmin bool
	cmd := &cobra.Command{
		Use:   "create [username]",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var resp api.UserResp
			if err := cli.Post("/v1/users", api.CreateUserReq{Username: args[0], IsAdmin: isAdmin}, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.Flags().BoolVar(&isAdmin, "admin", false, "create as admin user")
	return cmd
}

func usersListCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var users []api.UserResp
			if err := cli.Get("/v1/users", &users); err != nil {
				return err
			}
			if out.JSON {
				out.Print(users)
				return nil
			}
			rows := make([][]string, 0, len(users))
			for _, u := range users {
				adminStr := ""
				if u.IsAdmin {
					adminStr = "yes"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%d", u.ID),
					u.Username,
					adminStr,
				})
			}
			out.PrintTable([]string{"ID", "USERNAME", "ADMIN"}, rows)
			return nil
		},
	}
}

func usersDeleteCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [username]",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			return cli.Delete("/v1/users/" + args[0])
		},
	}
}

func usersTokenCmd(getClient clientFactory) *cobra.Command {
	var tokenName string
	cmd := &cobra.Command{
		Use:   "token [username]",
		Short: "Create a new API token for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var resp api.CreateTokenResp
			if err := cli.Post("/v1/users/"+args[0]+"/tokens", api.CreateTokenReq{Name: tokenName}, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenName, "name", "", "optional label for the token")
	return cmd
}
