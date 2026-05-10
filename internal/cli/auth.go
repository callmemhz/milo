package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}
	cmd.AddCommand(authLoginCmd())
	cmd.AddCommand(authWhoamiCmd())
	return cmd
}

func authLoginCmd() *cobra.Command {
	var endpoint, token, ctxName string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save credentials and set the active context",
		RunE: func(c *cobra.Command, args []string) error {
			if endpoint == "" {
				return fmt.Errorf("--endpoint is required")
			}
			if token == "" {
				return fmt.Errorf("--token is required")
			}
			if ctxName == "" {
				ctxName = "default"
			}
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			cfg.Contexts[ctxName] = Context{Endpoint: endpoint, Token: token}
			cfg.CurrentContext = ctxName
			if err := SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Logged in to %s as context %q\n", endpoint, ctxName)
			return nil
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "server base URL (e.g. https://milo.example.com)")
	cmd.Flags().StringVar(&token, "token", "", "bearer token")
	cmd.Flags().StringVar(&ctxName, "context-name", "", "name for this context (default: \"default\")")
	_ = cmd.MarkFlagRequired("endpoint")
	_ = cmd.MarkFlagRequired("token")
	return cmd
}

func authWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the identity of the current token",
		RunE: func(c *cobra.Command, args []string) error {
			// Resolve context and client without using getClient factory
			// (auth commands are wired before getClient is available).
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			ctx, ok := cfg.Active()
			if !ok || ctx.Endpoint == "" {
				return fmt.Errorf("no active context — run `milo auth login`")
			}
			cli := &Client{Endpoint: ctx.Endpoint, Token: ctx.Token}
			var resp map[string]any
			if err := cli.Get("/v1/auth/whoami", &resp); err != nil {
				return err
			}
			out := &Output{W: c.OutOrStdout()}
			out.Print(resp)
			return nil
		},
	}
}
