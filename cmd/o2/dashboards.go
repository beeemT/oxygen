package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/output"
)

// dashboardsCmd is the parent for all dashboards subcommands.
var dashboardsCmd = &cobra.Command{
	Use:   "dashboards",
	Short: "List and manage dashboards",
}

var dashboardsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all dashboards",
	RunE:  runDashboardsList,
}

var dashboardsGetCmd = &cobra.Command{
	Use:   "get [dashboard-id]",
	Short: "Get a dashboard by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runDashboardsGet,
}

var dashboardsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new dashboard",
	RunE:  runDashboardsCreate,
}

var dashboardsUpdateCmd = &cobra.Command{
	Use:   "update [dashboard-id]",
	Short: "Update an existing dashboard",
	Args:  cobra.ExactArgs(1),
	RunE:  runDashboardsUpdate,
}

var dashboardsDeleteCmd = &cobra.Command{
	Use:   "delete [dashboard-id]",
	Short: "Delete a dashboard",
	Args:  cobra.ExactArgs(1),
	RunE:  runDashboardsDelete,
}

func init() {
	rootCmd.AddCommand(dashboardsCmd)
	dashboardsCmd.AddCommand(dashboardsListCmd, dashboardsGetCmd, dashboardsCreateCmd, dashboardsUpdateCmd, dashboardsDeleteCmd)

	// list flags.
	lf := dashboardsListCmd.Flags()
	lf.String("folder", "", "Filter by folder ID")

	// create/update flags.
	cf := dashboardsCreateCmd.Flags()
	cf.String("file", "", "Path to JSON file containing the dashboard definition")

	uf := dashboardsUpdateCmd.Flags()
	uf.String("file", "", "Path to JSON file containing the dashboard definition")
}

func runDashboardsList(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	folder, _ := cmd.Flags().GetString("folder")

	resp, err := cli.Dashboards(context.Background(), folder)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "ID", "Title", "Folder", "Owner", "Type")
		for _, d := range resp.Dashboards {
			tw.Row(d.ID, d.Title, d.Folder, d.Owner, d.Type)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Dashboards)
}

func runDashboardsGet(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.Dashboard(context.Background(), args[0])
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.Dashboard)
}

func readDashboardFile(filePath string) (map[string]any, error) {
	if filePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading dashboard file: %w", err)
	}

	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing dashboard JSON: %w", err)
	}

	return v, nil
}

func runDashboardsCreate(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	filePath, _ := cmd.Flags().GetString("file")
	dashboardMap, err := readDashboardFile(filePath)
	if err != nil {
		return err
	}

	// If no file provided, return an error (Phase 5 will add interactive creation).
	if dashboardMap == nil {
		return fmt.Errorf("--file is required for create (interactive creation is not yet supported)")
	}

	// Extract known fields and build the request.
	title, _ := dashboardMap["title"].(string)
	description, _ := dashboardMap["description"].(string)
	folder, _ := dashboardMap["folder_id"].(string)
	owner, _ := dashboardMap["owner"].(string)

	var payload json.RawMessage
	if p, ok := dashboardMap["payload"].(map[string]any); ok {
		payload, _ = json.Marshal(p)
	}

	resp, err := cli.CreateDashboard(context.Background(), api.CreateDashboardRequest{
		Title:       title,
		Description: description,
		Folder:      folder,
		Owner:       owner,
		Payload:     payload,
	})
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.Dashboard)
}

func runDashboardsUpdate(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	filePath, _ := cmd.Flags().GetString("file")
	dashboardMap, err := readDashboardFile(filePath)
	if err != nil {
		return err
	}

	if dashboardMap == nil {
		return fmt.Errorf("--file is required for update")
	}

	title, _ := dashboardMap["title"].(string)
	description, _ := dashboardMap["description"].(string)
	folder, _ := dashboardMap["folder_id"].(string)
	owner, _ := dashboardMap["owner"].(string)

	var payload json.RawMessage
	if p, ok := dashboardMap["payload"].(map[string]any); ok {
		payload, _ = json.Marshal(p)
	}

	resp, err := cli.UpdateDashboard(context.Background(), args[0], api.UpdateDashboardRequest{
		Title:       title,
		Description: description,
		Folder:      folder,
		Owner:       owner,
		Payload:     payload,
	})
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.Dashboard)
}

func runDashboardsDelete(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	if err := cli.DeleteDashboard(context.Background(), args[0]); err != nil {
		return err
	}

	if !cfg.Quiet {
		outWriter.Info("Deleted dashboard %s", args[0])
	}

	return nil
}
