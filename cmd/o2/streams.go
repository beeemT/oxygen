package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/output"
)

// streamsCmd is the parent for all streams subcommands.
var streamsCmd = &cobra.Command{
	Use:   "streams",
	Short: "List and describe streams",
}

var streamsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all streams",
	RunE:  runStreamsList,
}

var streamsDescribeCmd = &cobra.Command{
	Use:   "describe [stream-name]",
	Short: "Show stream schema and statistics",
	Args:  cobra.ExactArgs(1),
	RunE:  runStreamsDescribe,
}

func init() {
	rootCmd.AddCommand(streamsCmd)
	streamsCmd.AddCommand(streamsListCmd, streamsDescribeCmd)

	lf := streamsListCmd.Flags()
	lf.String("type", "", "Filter by stream type: logs, metrics, traces")
}

func runStreamsList(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.Streams(context.Background())
	if err != nil {
		return err
	}

	streamType, _ := cmd.Flags().GetString("type")

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Name", "Type", "Docs", "Size (MB)")
		for _, s := range resp.Streams {
			if streamType != "" && !strings.EqualFold(s.StreamType, streamType) {
				continue
			}
			sizeMB := float64(s.StorageMB) / (1024 * 1024)
			tw.Row(s.Name, s.StreamType, fmt.Sprintf("%d", s.DocCount), fmt.Sprintf("%.2f", sizeMB))
		}
		return tw.Flush()
	}

	// JSON mode: filter client-side.
	if streamType != "" {
		var filtered []api.StreamInfo
		for _, s := range resp.Streams {
			if strings.EqualFold(s.StreamType, streamType) {
				filtered = append(filtered, s)
			}
		}
		return outWriter.WriteJSON(filtered)
	}

	return outWriter.WriteJSON(resp.Streams)
}

func runStreamsDescribe(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	name := args[0]
	resp, err := cli.StreamSchema(context.Background(), name)
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp)
}
