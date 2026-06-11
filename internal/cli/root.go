package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version and Commit are set at build time via -ldflags; see .goreleaser.yml.
var (
	Version = "dev"
	Commit  = "unknown"
)

// clientFactory is the signature for the lazily-evaluated client+output constructor
// that every subcommand receives. Evaluated at command run time so that --context
// and --json flags are already parsed.
type clientFactory = func() (*Client, *Output, error)

// RootCmd builds and returns the cobra root command with all subcommands attached.
func RootCmd() *cobra.Command {
	var contextName string
	var jsonOutput bool

	root := &cobra.Command{
		Use:           "milo",
		Short:         "Milo CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&contextName, "context", "", "config context to use (default: current_context)")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output JSON")

	getClient := func() (*Client, *Output, error) {
		cfg, err := LoadConfig()
		if err != nil {
			return nil, nil, err
		}
		name := contextName
		if name == "" {
			name = cfg.CurrentContext
		}
		ctx, ok := cfg.Contexts[name]
		if !ok || ctx.Endpoint == "" {
			return nil, nil, fmt.Errorf("no active context — run `milo auth login`")
		}
		return &Client{Endpoint: ctx.Endpoint, Token: ctx.Token},
			&Output{JSON: jsonOutput, W: os.Stdout},
			nil
	}

	root.AddCommand(authCmd())
	root.AddCommand(contextCmd())
	root.AddCommand(appsCmd(getClient))
	root.AddCommand(addonsCmd(getClient))
	root.AddCommand(envCmd(getClient))
	root.AddCommand(deployCmd(getClient))
	root.AddCommand(deploymentsCmd(getClient))
	root.AddCommand(usersCmd(getClient))
	root.AddCommand(tokensCmd(getClient))
	root.AddCommand(adminCmd())
	root.AddCommand(versionCmd())

	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version and exit",
		Run: func(c *cobra.Command, args []string) {
			fmt.Printf("milo %s (%s)\n", Version, Commit)
		},
	}
}
