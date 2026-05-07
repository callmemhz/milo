package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func contextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage named server contexts",
	}
	cmd.AddCommand(contextListCmd())
	cmd.AddCommand(contextUseCmd())
	cmd.AddCommand(contextDeleteCmd())
	return cmd
}

func contextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all contexts",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			out := &Output{W: c.OutOrStdout()}
			rows := make([][]string, 0, len(cfg.Contexts))
			for name, ctx := range cfg.Contexts {
				active := ""
				if name == cfg.CurrentContext {
					active = "*"
				}
				rows = append(rows, []string{active, name, ctx.Endpoint})
			}
			out.PrintTable([]string{"", "NAME", "ENDPOINT"}, rows)
			return nil
		},
	}
}

func contextUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [name]",
		Short: "Set the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Contexts[args[0]]; !ok {
				return fmt.Errorf("context %q not found", args[0])
			}
			cfg.CurrentContext = args[0]
			if err := SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Switched to context %q\n", args[0])
			return nil
		},
	}
}

func contextDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Contexts[args[0]]; !ok {
				return fmt.Errorf("context %q not found", args[0])
			}
			delete(cfg.Contexts, args[0])
			if cfg.CurrentContext == args[0] {
				cfg.CurrentContext = ""
			}
			if err := SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Deleted context %q\n", args[0])
			return nil
		},
	}
}
