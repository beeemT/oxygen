package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// CSVWriter writes records as CSV.
type CSVWriter struct {
	w      *csv.Writer
	header []string
	once   bool
}

// NewCSVWriter returns a CSVWriter writing to out.
func NewCSVWriter(out io.Writer) *CSVWriter {
	return &CSVWriter{w: csv.NewWriter(out)}
}

// SetHeader sets the CSV header row. Must be called before any Row call.
func (c *CSVWriter) SetHeader(cols []string) {
	c.header = cols
	c.once = true
	_ = c.w.Write(cols)
}

// Row writes a single record. If SetHeader has not been called, the first
// call establishes the header. Subsequent calls must have the same number
// of fields.
func (c *CSVWriter) Row(vals ...string) error {
	if !c.once {
		c.once = true
		_ = c.w.Write(vals)
		return nil
	}

	return c.w.Write(vals)
}

// Flush flushes the writer.
func (c *CSVWriter) Flush() {
	c.w.Flush()
}

// Error returns any CSV writer error.
func (c *CSVWriter) Error() error {
	return c.w.Error()
}

// WriteJSONRecords writes an array of JSON objects as CSV rows to out.
// It uses the keys of the first record as the header.
func WriteJSONRecords(out io.Writer, records []json.RawMessage) error {
	if len(records) == 0 {
		return nil
	}

	cw := NewCSVWriter(out)
	seenHeader := false
	var header []string

	for _, raw := range records {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}

		if !seenHeader {
			seenHeader = true
			// Determine column order: timestamp/message/severity first, then rest (sorted).
			header = orderedHeader(obj)
			cw.SetHeader(header)
		}

		row := make([]string, len(header))
		for i, k := range header {
			row[i] = formatJSONValue(obj[k])
		}
		if err := cw.Row(row...); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// orderedHeader returns a consistent column order for CSV export.
func orderedHeader(obj map[string]any) []string {
	priority := []string{
		"_timestamp", "timestamp", "time", "ts", "message", "msg", "log",
		"level", "severity", "service", "host", "container", "pod",
	}
	var header []string
	seen := make(map[string]bool)

	for _, k := range priority {
		if _, ok := obj[k]; ok {
			header = append(header, k)
			seen[k] = true
		}
	}
	// Non-priority keys must be sorted for deterministic output.
	var rest []string
	for k := range obj {
		if !seen[k] {
			rest = append(rest, k)
			seen[k] = true
		}
	}
	sort.Strings(rest)
	header = append(header, rest...)

	return header
}

// formatJSONValue formats a JSON value for CSV output.
func formatJSONValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	case bool:
		return fmt.Sprintf("%v", x)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}
