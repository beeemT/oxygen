package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/output"
)

// alertsCmd is the parent for all alerts subcommands.
var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Manage v2 alerts",
	Long:  "All alert commands use the /v2/ API prefix.",
}

var alertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all v2 alerts",
	RunE:  runAlertsList,
}

var alertsGetCmd = &cobra.Command{
	Use:   "get [alert-id]",
	Short: "Get a v2 alert by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runAlertsGet,
}

var alertsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new v2 alert",
	RunE:  runAlertsCreate,
}

var alertsUpdateCmd = &cobra.Command{
	Use:   "update [alert-id]",
	Short: "Update an existing v2 alert",
	Args:  cobra.ExactArgs(1),
	RunE:  runAlertsUpdate,
}

var alertsDeleteCmd = &cobra.Command{
	Use:   "delete [alert-id]",
	Short: "Delete a v2 alert",
	Args:  cobra.ExactArgs(1),
	RunE:  runAlertsDelete,
}

var alertsTriggerCmd = &cobra.Command{
	Use:   "trigger [alert-id]",
	Short: "Manually trigger a v2 alert",
	Args:  cobra.ExactArgs(1),
	RunE:  runAlertsTrigger,
}

var alertsHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show alert firing history",
	RunE:  runAlertsHistory,
}

var alertsIncidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "List firing alert incidents",
}

var alertsIncidentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all firing incidents",
	RunE:  runAlertsIncidentsList,
}

var alertsTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Manage alert templates",
}

var alertsTemplatesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all alert templates",
	RunE:  runAlertsTemplatesList,
}

func init() {
	rootCmd.AddCommand(alertsCmd)
	alertsCmd.AddCommand(
		alertsListCmd, alertsGetCmd,
		alertsCreateCmd, alertsUpdateCmd, alertsDeleteCmd,
		alertsTriggerCmd, alertsHistoryCmd,
		alertsIncidentsCmd, alertsTemplatesCmd,
	)
	alertsIncidentsCmd.AddCommand(alertsIncidentsListCmd)
	alertsTemplatesCmd.AddCommand(alertsTemplatesListCmd)

	// list flags.
	lf := alertsListCmd.Flags()
	lf.String("status", "", "Filter by alert status (e.g. firing, pending, resolved)")

	// history flags.
	hf := alertsHistoryCmd.Flags()
	hf.String("alert-id", "", "Filter history by alert ID")
	hf.Int64("limit", 50, "Maximum number of history entries")

	// incidents flags.
	isf := alertsIncidentsListCmd.Flags()
	isf.Int64("limit", 50, "Maximum number of incidents")

	// create/update flags.
	cf := alertsCreateCmd.Flags()
	cf.String("file", "", "Path to JSON file containing the alert definition")

	uf := alertsUpdateCmd.Flags()
	uf.String("file", "", "Path to JSON file containing the alert definition")
}

func runAlertsList(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	status, _ := cmd.Flags().GetString("status")

	resp, err := cli.Alerts(context.Background(), status)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "ID", "Name", "Stream", "Status", "Enabled", "Created")
		for _, a := range resp.Alerts {
			created := formatUnixTime(a.CreatedAt)
			enabled := "false"
			if a.IsEnabled {
				enabled = "true"
			}
			tw.Row(a.ID, a.Name, a.StreamName, a.Status, enabled, created)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Alerts)
}

func runAlertsGet(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.Alert(context.Background(), args[0])
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.Alert)
}

func readAlertFile(filePath string) (map[string]any, error) {
	if filePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading alert file: %w", err)
	}

	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing alert JSON: %w", err)
	}

	return v, nil
}

func buildAlertCondition(condMap map[string]any) api.AlertCondition {
	var c api.AlertCondition
	if col, ok := condMap["column"].(string); ok {
		c.Column = col
	}
	if op, ok := condMap["operator"].(string); ok {
		c.Operator = op
	}
	if val, ok := condMap["value"]; ok {
		c.Value = val
	}
	return c
}

func runAlertsCreate(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	filePath, _ := cmd.Flags().GetString("file")
	alertMap, err := readAlertFile(filePath)
	if err != nil {
		return err
	}

	if alertMap == nil {
		return fmt.Errorf("--file is required for create (interactive creation is not yet supported)")
	}

	name, _ := alertMap["name"].(string)
	streamName, _ := alertMap["stream_name"].(string)
	query, _ := alertMap["query"].(string)
	description, _ := alertMap["description"].(string)
	owner, _ := alertMap["owner"].(string)

	var duration int
	if d, ok := alertMap["duration"].(float64); ok {
		duration = int(d)
	}

	var isEnabled bool
	if e, ok := alertMap["is_enabled"].(bool); ok {
		isEnabled = e
	}

	var condition api.AlertCondition
	if cond, ok := alertMap["condition"].(map[string]any); ok {
		condition = buildAlertCondition(cond)
	}

	var threshold json.RawMessage
	if t, ok := alertMap["threshold"].(map[string]any); ok {
		threshold, _ = json.Marshal(t)
	}

	resp, err := cli.CreateAlert(context.Background(), api.CreateAlertRequest{
		Name:        name,
		StreamName:  streamName,
		Query:       query,
		Condition:   condition,
		Duration:    duration,
		Threshold:   threshold,
		IsEnabled:   isEnabled,
		Owner:       owner,
		Description: description,
	})
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.Alert)
}

func runAlertsUpdate(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	filePath, _ := cmd.Flags().GetString("file")
	alertMap, err := readAlertFile(filePath)
	if err != nil {
		return err
	}

	if alertMap == nil {
		return fmt.Errorf("--file is required for update")
	}

	name, _ := alertMap["name"].(string)
	streamName, _ := alertMap["stream_name"].(string)
	query, _ := alertMap["query"].(string)
	description, _ := alertMap["description"].(string)
	owner, _ := alertMap["owner"].(string)

	var duration int
	if d, ok := alertMap["duration"].(float64); ok {
		duration = int(d)
	}

	var isEnabledPtr *bool
	if e, ok := alertMap["is_enabled"].(bool); ok {
		isEnabledPtr = &e
	}

	var condition api.AlertCondition
	if cond, ok := alertMap["condition"].(map[string]any); ok {
		condition = buildAlertCondition(cond)
	}

	var threshold json.RawMessage
	if t, ok := alertMap["threshold"].(map[string]any); ok {
		threshold, _ = json.Marshal(t)
	}

	resp, err := cli.UpdateAlert(context.Background(), args[0], api.UpdateAlertRequest{
		Name:        name,
		StreamName:  streamName,
		Query:       query,
		Condition:   condition,
		Duration:    duration,
		Threshold:   threshold,
		IsEnabled:   isEnabledPtr,
		Owner:       owner,
		Description: description,
	})
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.Alert)
}

func runAlertsDelete(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	if err := cli.DeleteAlert(context.Background(), args[0]); err != nil {
		return err
	}

	if !cfg.Quiet {
		outWriter.Info("Deleted alert %s", args[0])
	}

	return nil
}

func runAlertsTrigger(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	if err := cli.TriggerAlert(context.Background(), args[0]); err != nil {
		return err
	}

	if !cfg.Quiet {
		outWriter.Info("Triggered alert %s", args[0])
	}

	return nil
}

func runAlertsHistory(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	alertID, _ := cmd.Flags().GetString("alert-id")
	limit, _ := cmd.Flags().GetInt64("limit")

	resp, err := cli.AlertHistory(context.Background(), alertID, limit)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Alert ID", "Name", "Stream", "Status", "Fired At", "Resolved At")
		for _, h := range resp.Alerts {
			firedAt := formatUnixTime(h.FiredAt)
			resolvedAt := formatUnixTime(h.ResolvedAt)
			tw.Row(h.AlertID, h.AlertName, h.Stream, h.Status, firedAt, resolvedAt)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Alerts)
}

func runAlertsIncidentsList(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	limit, _ := cmd.Flags().GetInt64("limit")

	resp, err := cli.AlertIncidents(context.Background(), limit)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "ID", "Alert ID", "Name", "Status", "Fired At", "Resolved At")
		for _, i := range resp.Incidents {
			firedAt := formatUnixTime(i.FiredAt)
			resolvedAt := formatUnixTime(i.ResolvedAt)
			tw.Row(i.ID, i.AlertID, i.AlertName, i.Status, firedAt, resolvedAt)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Incidents)
}

func runAlertsTemplatesList(_ *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.AlertTemplates(context.Background())
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "ID", "Name", "Description")
		for _, t := range resp.Templates {
			tw.Row(t.ID, t.Name, t.Description)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Templates)
}

// formatUnixTime converts a Unix timestamp (seconds) to RFC3339 for table display.
func formatUnixTime(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}
