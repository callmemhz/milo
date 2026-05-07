package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

func envCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage application environment variables",
	}
	cmd.AddCommand(envGetCmd(getClient))
	cmd.AddCommand(envSetCmd(getClient))
	cmd.AddCommand(envUnsetCmd(getClient))
	cmd.AddCommand(envImportCmd(getClient))
	return cmd
}

func envGetCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "get [app]",
		Short: "Show all environment variables for an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var env map[string]string
			if err := cli.Get("/v1/apps/"+args[0]+"/env", &env); err != nil {
				return err
			}
			if out.JSON {
				out.Print(env)
				return nil
			}
			rows := make([][]string, 0, len(env))
			for k, v := range env {
				rows = append(rows, []string{k, v})
			}
			out.PrintTable([]string{"KEY", "VALUE"}, rows)
			return nil
		},
	}
}

func envSetCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "set [app] KEY=VALUE [KEY=VALUE...]",
		Short: "Set one or more environment variables",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			kv := map[string]string{}
			for _, a := range args[1:] {
				i := strings.Index(a, "=")
				if i < 0 {
					return fmt.Errorf("expected KEY=VALUE: %q", a)
				}
				kv[a[:i]] = a[i+1:]
			}
			var env map[string]string
			if err := cli.Patch("/v1/apps/"+args[0]+"/env", api.EnvPatchReq{Set: kv}, &env); err != nil {
				return err
			}
			out.Print(env)
			return nil
		},
	}
}

func envUnsetCmd(getClient clientFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "unset [app] KEY [KEY...]",
		Short: "Remove one or more environment variables",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var env map[string]string
			if err := cli.Patch("/v1/apps/"+args[0]+"/env", api.EnvPatchReq{Unset: args[1:]}, &env); err != nil {
				return err
			}
			out.Print(env)
			return nil
		},
	}
}

func envImportCmd(getClient clientFactory) *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "import [app]",
		Short: "Replace all env vars from a .env file (PUT replaces entirely)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			kv, err := parseDotEnv(filePath)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", filePath, err)
			}
			var env map[string]string
			if err := cli.Put("/v1/apps/"+args[0]+"/env", kv, &env); err != nil {
				return err
			}
			out.Print(env)
			return nil
		},
	}
	cmd.Flags().StringVar(&filePath, "file", ".env", "path to .env file")
	return cmd
}

// parseDotEnv reads a simple KEY=VALUE dotenv file, skipping blank lines and
// lines starting with '#'.
func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i < 0 {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", lineNo)
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		// Strip optional surrounding quotes from value.
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		result[key] = val
	}
	return result, scanner.Err()
}
