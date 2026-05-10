package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/callmemhz/milo/pkg/api"
)

func appsCmd(getClient clientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apps",
		Short: "Manage applications",
	}

	// create
	create := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			req := api.CreateAppReq{Name: args[0]}
			if v, _ := c.Flags().GetFloat64("cpu"); v > 0 {
				req.CPULimit = v
			}
			if v, _ := c.Flags().GetInt64("memory"); v > 0 {
				req.MemoryLimitMB = v
			}
			if v, _ := c.Flags().GetString("health-path"); v != "" {
				req.HealthPath = v
			}
			if v, _ := c.Flags().GetStringSlice("owner"); len(v) > 0 {
				req.Owners = v
			}
			var resp api.AppResp
			if err := cli.Post("/v1/apps", req, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	create.Flags().Float64("cpu", 0, "CPU limit (e.g. 0.5)")
	create.Flags().Int64("memory", 0, "memory limit in MB")
	create.Flags().String("health-path", "", "HTTP path for health check")
	create.Flags().StringSlice("owner", nil, "additional owners (admin only)")
	cmd.AddCommand(create)

	// list
	list := &cobra.Command{
		Use:   "list",
		Short: "List applications",
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var apps []api.AppResp
			if err := cli.Get("/v1/apps", &apps); err != nil {
				return err
			}
			if out.JSON {
				out.Print(apps)
				return nil
			}
			rows := make([][]string, 0, len(apps))
			for _, a := range apps {
				rows = append(rows, []string{
					a.Name,
					fmt.Sprintf("%d", a.Port),
					strings.Join(a.Owners, ","),
				})
			}
			out.PrintTable([]string{"NAME", "PORT", "OWNERS"}, rows)
			return nil
		},
	}
	cmd.AddCommand(list)

	// get
	get := &cobra.Command{
		Use:   "get [name]",
		Short: "Get application details",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var a api.AppResp
			if err := cli.Get("/v1/apps/"+args[0], &a); err != nil {
				return err
			}
			out.Print(a)
			return nil
		},
	}
	cmd.AddCommand(get)

	// update
	update := &cobra.Command{
		Use:   "update [name]",
		Short: "Update application configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			req := api.UpdateAppReq{}
			if c.Flags().Changed("cpu") {
				v, _ := c.Flags().GetFloat64("cpu")
				req.CPULimit = &v
			}
			if c.Flags().Changed("memory") {
				v, _ := c.Flags().GetInt64("memory")
				req.MemoryLimitMB = &v
			}
			if c.Flags().Changed("health-path") {
				v, _ := c.Flags().GetString("health-path")
				req.HealthPath = &v
			}
			var resp api.AppResp
			if err := cli.Patch("/v1/apps/"+args[0], req, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	update.Flags().Float64("cpu", 0, "CPU limit")
	update.Flags().Int64("memory", 0, "memory limit in MB")
	update.Flags().String("health-path", "", "HTTP health check path")
	cmd.AddCommand(update)

	// delete
	del := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			withVol, _ := c.Flags().GetBool("delete-volumes")
			path := "/v1/apps/" + args[0]
			if withVol {
				path += "?delete_volumes=true"
			}
			return cli.Delete(path)
		},
	}
	del.Flags().Bool("delete-volumes", false, "also delete the app's data volume")
	cmd.AddCommand(del)

	// status
	status := &cobra.Command{
		Use:   "status [name]",
		Short: "Show runtime status of an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var resp map[string]any
			if err := cli.Get("/v1/apps/"+args[0]+"/status", &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.AddCommand(status)

	// logs
	logs := &cobra.Command{
		Use:   "logs [name]",
		Short: "Stream application logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, _, err := getClient()
			if err != nil {
				return err
			}
			follow, _ := c.Flags().GetBool("follow")
			tail, _ := c.Flags().GetString("tail")
			url := fmt.Sprintf("/v1/apps/%s/logs?follow=%t&tail=%s", args[0], follow, tail)
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

	// restart
	restart := &cobra.Command{
		Use:   "restart [name]",
		Short: "Restart an application (re-deploys current image)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cli, out, err := getClient()
			if err != nil {
				return err
			}
			var resp api.DeploymentResp
			if err := cli.Post("/v1/apps/"+args[0]+"/restart", nil, &resp); err != nil {
				return err
			}
			out.Print(resp)
			return nil
		},
	}
	cmd.AddCommand(restart)

	return cmd
}
