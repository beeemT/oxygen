package output

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/beeemt/oxygen/internal/api"
)

// MetricsRenderer renders Prometheus-style metric results.
type MetricsRenderer struct {
	out     io.Writer
	noColor bool
}

// NewMetricsRenderer returns a MetricsRenderer writing to out.
func NewMetricsRenderer(out io.Writer, noColor bool) *MetricsRenderer {
	return &MetricsRenderer{out: out, noColor: noColor}
}

// RenderInstant writes a PromQL instant query result to out.
func (r *MetricsRenderer) RenderInstant(results []api.InstantResult, query string, ts int64) error {
	if len(results) == 0 {
		fmt.Fprintln(r.out, "No results.")
		return nil
	}

	labelWidth := r.maxLabelWidth(results)
	if labelWidth == 0 {
		labelWidth = 40
	}

	fmt.Fprintf(r.out, "Query: %s at %d\n\n", query, ts)

	tw := NewTable(r.out, "Labels", "Value")
	for _, res := range results {
		labelStr := formatLabels(res.Labels, labelWidth)
		valStr := formatValue(res.Value)
		tw.Row(labelStr, valStr)
	}
	return tw.Flush()
}

// RenderRange writes a PromQL range query result to out.
func (r *MetricsRenderer) RenderRange(results []api.RangeResult, query string) error {
	if len(results) == 0 {
		fmt.Fprintln(r.out, "No results.")
		return nil
	}

	fmt.Fprintf(r.out, "Query: %s\n\n", query)
	fmt.Fprintf(r.out, "%d series, %d total samples\n\n", len(results), totalSamples(results))

	// Show a sample of the first series.
	if len(results) > 0 && len(results[0].Samples) > 0 {
		s := results[0].Samples
		fmt.Fprintf(r.out, "First series (%s):\n", formatLabels(results[0].Labels, 40))
		tw := NewTable(r.out, "Timestamp", "Value")
		limit := 10
		if len(s) < limit {
			limit = len(s)
		}
		for _, sm := range s[:limit] {
			tw.Row(fmt.Sprintf("%d", sm.Timestamp), formatValue(sm.Value))
		}
		if len(s) > limit {
			fmt.Fprintf(r.out, "... (%d more samples)\n", len(s)-limit)
		}
		_ = tw.Flush()
	}

	return nil
}

func (r *MetricsRenderer) maxLabelWidth(results []api.InstantResult) int {
	max := 0
	for _, res := range results {
		l := len(formatLabels(res.Labels, 0))
		if l > max {
			max = l
		}
	}
	return max
}

func totalSamples(results []api.RangeResult) int {
	n := 0
	for _, r := range results {
		n += len(r.Samples)
	}
	return n
}

// formatLabels formats a label map as "key=value,key=value" with consistent sort.
func formatLabels(labels map[string]string, maxWidth int) string {
	if len(labels) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	s := strings.Join(parts, ", ")
	if maxWidth > 0 && len(s) > maxWidth {
		return s[:maxWidth-1] + "…"
	}
	return s
}

// formatValue formats a float64 for display.
func formatValue(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.4f", v)
}
