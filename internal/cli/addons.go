package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo/pkg/api"
)

func addonsCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "addons",
		Aliases: []string{"addon"},
		Short:   "Manage add-ons (postgres, redis)",
	}

	// create
	create := &cobra.Command{
		Use:   "create [name]",
		Short: "Create an add-on",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			engine, _ := c.Flags().GetString("engine")
			req := api.CreateAddonReq{Name: args[0], Engine: engine}
			if v, _ := c.Flags().GetString("engine-version"); v != "" {
				req.Version = v
			}
			if v, _ := c.Flags().GetFloat64("cpu"); v > 0 {
				req.CPULimit = v
			}
			if v, _ := c.Flags().GetInt64("memory"); v > 0 {
				req.MemoryLimitMB = v
			}
			if v, _ := c.Flags().GetStringSlice("owner"); len(v) > 0 {
				req.Owners = v
			}
			var resp api.AddonResp
			if err := cli.Post("/v1/addons", req, &resp); err != nil {
				return err
			}

			// --attach: link to an app right away (CLI sugar for create+link).
			if app, _ := c.Flags().GetString("attach"); app != "" {
				alias, _ := c.Flags().GetString("as")
				var link api.LinkResp
				if err := cli.Post("/v1/apps/"+app+"/links", api.CreateLinkReq{Addon: args[0], Alias: alias}, &link); err != nil {
					return fmt.Errorf("addon created but link failed: %w", err)
				}
				resp.LinkedApps = append(resp.LinkedApps, app)
			}
			out.Print(resp)
			return nil
		},
	}
	create.Flags().String("engine", "", "add-on engine: postgres or redis (required)")
	create.Flags().String("engine-version", "", "engine major version (default per engine)")
	create.Flags().Float64("cpu", 0, "CPU limit (e.g. 0.5)")
	create.Flags().Int64("memory", 0, "memory limit in MB")
	create.Flags().StringSlice("owner", nil, "additional owners (admin only)")
	create.Flags().String("attach", "", "app to link immediately after creation")
	create.Flags().String("as", "", "env prefix for --attach (e.g. CACHE → CACHE_URL)")
	_ = create.MarkFlagRequired("engine")
	cmd.AddCommand(create)

	// list
	list := &cobra.Command{
		Use:   "list",
		Short: "List add-ons",
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var addons []api.AddonResp
			if err := cli.Get("/v1/addons", &addons); err != nil {
				return err
			}
			if out.JSON {
				out.Print(addons)
				return nil
			}
			rows := make([][]string, 0, len(addons))
			for _, s := range addons {
				rows = append(rows, []string{
					s.Name,
					s.Engine + ":" + s.Version,
					s.Status,
					strings.Join(s.LinkedApps, ","),
					strings.Join(s.Owners, ","),
				})
			}
			out.PrintTable([]string{"NAME", "ENGINE", "STATUS", "LINKED APPS", "OWNERS"}, rows)
			return nil
		},
	}
	cmd.AddCommand(list)

	// get
	get := &cobra.Command{
		Use:   "get [name]",
		Short: "Get add-on details (includes connection URL)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var s api.AddonResp
			if err := cli.Get("/v1/addons/"+args[0], &s); err != nil {
				return err
			}
			out.Print(s)
			return nil
		},
	}
	cmd.AddCommand(get)

	// delete
	del := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete an add-on (admin only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			q := url.Values{}
			if v, _ := c.Flags().GetBool("delete-volumes"); v {
				q.Set("delete_volumes", "true")
			}
			if v, _ := c.Flags().GetBool("force"); v {
				q.Set("force", "true")
			}
			path := "/v1/addons/" + args[0]
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			return cli.Delete(path)
		},
	}
	del.Flags().Bool("delete-volumes", false, "also delete the add-on's data volume")
	del.Flags().Bool("force", false, "delete even if apps are linked (unlinks and redeploys them)")
	cmd.AddCommand(del)

	// restart
	restart := &cobra.Command{
		Use:   "restart [name]",
		Short: "Restart an add-on (recreates the container, keeps data)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var s api.AddonResp
			if err := cli.Post("/v1/addons/"+args[0]+"/restart", nil, &s); err != nil {
				return err
			}
			out.Print(s)
			return nil
		},
	}
	cmd.AddCommand(restart)

	// logs
	logs := &cobra.Command{
		Use:   "logs [name]",
		Short: "Stream add-on logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			follow, _ := c.Flags().GetBool("follow")
			tail, _ := c.Flags().GetString("tail")
			url := fmt.Sprintf("/v1/addons/%s/logs?follow=%t&tail=%s", args[0], follow, tail)
			rdr, err := cli.Stream(url)
			if err != nil {
				return err
			}
			defer rdr.Close()
			_, err = io.Copy(os.Stdout, rdr)
			return err
		},
	}
	logs.Flags().BoolP("follow", "f", false, "follow log output")
	logs.Flags().String("tail", "100", "number of lines from the end")
	cmd.AddCommand(logs)

	// link
	link := &cobra.Command{
		Use:   "link [addon] [app]",
		Short: "Link an add-on to an app (injects DATABASE_URL/REDIS_URL and redeploys)",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			alias, _ := c.Flags().GetString("as")
			var resp api.LinkResp
			if err := cli.Post("/v1/apps/"+args[1]+"/links", api.CreateLinkReq{Addon: args[0], Alias: alias}, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	link.Flags().String("as", "", "env prefix (e.g. CACHE → CACHE_URL); default per engine")
	cmd.AddCommand(link)

	// unlink
	unlink := &cobra.Command{
		Use:   "unlink [addon] [app]",
		Short: "Unlink an add-on from an app (removes injected env and redeploys)",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			return cli.Delete("/v1/apps/" + args[1] + "/links/" + args[0])
		},
	}
	cmd.AddCommand(unlink)

	// links — list links from the app side
	links := &cobra.Command{
		Use:   "links [app]",
		Short: "List add-ons linked to an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var resp []api.LinkResp
			if err := cli.Get("/v1/apps/"+args[0]+"/links", &resp); err != nil {
				return err
			}
			if out.JSON {
				out.Print(resp)
				return nil
			}
			rows := make([][]string, 0, len(resp))
			for _, l := range resp {
				rows = append(rows, []string{l.Addon, l.Engine, l.EnvKey})
			}
			out.PrintTable([]string{"SERVICE", "ENGINE", "ENV KEY"}, rows)
			return nil
		},
	}
	cmd.AddCommand(links)

	return cmd
}
