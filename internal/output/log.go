package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// LogEntry represents a parsed log record for human-readable rendering.
type LogEntry struct {
	Timestamp time.Time
	Severity  string // ERROR, WARN, INFO, DEBUG, etc.
	Message   string
	Fields    map[string]string // key=value pairs for bracketed rendering
}

// LogRenderer renders human-readable log lines.
type LogRenderer struct {
	out     io.Writer
	noColor bool
}

// NewLogRenderer returns a LogRenderer writing to out.
func NewLogRenderer(out io.Writer, noColor bool) *LogRenderer {
	return &LogRenderer{out: out, noColor: noColor}
}

// Render renders a single log entry as a human-readable line.
func (r *LogRenderer) Render(entry LogEntry) error {
	ts := formatTimestamp(entry.Timestamp)
	sev := entry.Severity
	if sev == "" {
		sev = r.detectSeverity(entry.Fields)
	}

	var sevStr string
	if r.noColor {
		sevStr = fmt.Sprintf("%-5s", strings.ToUpper(sev))
	} else {
		sevStr = colorizeSeverity(sev, fmt.Sprintf("%-5s", strings.ToUpper(sev)))
	}

	// Format fields as [key=value] pairs.
	var fieldParts []string
	for k, v := range entry.Fields {
		if k == "message" || k == "msg" || k == "log" {
			continue // skip duplicate message field
		}
		fieldParts = append(fieldParts, fmt.Sprintf("%s=%s", k, v))
	}

	msg := entry.Message
	if msg == "" {
		msg = "(empty)"
	}

	line := fmt.Sprintf("[%s] %s  %s", ts, sevStr, msg)
	if len(fieldParts) > 0 {
		line += "  [" + strings.Join(fieldParts, "] [") + "]"
	}
	_, err := fmt.Fprintln(r.out, line)

	return err
}

// RenderJSON decodes a json.RawMessage log record and renders it.
// It returns nil on decode error so the caller can continue.
func (r *LogRenderer) RenderJSON(raw json.RawMessage) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		fmt.Fprintln(r.out, string(raw))
		return
	}

	entry := r.parseObject(obj)
	_ = r.Render(entry)
}

// parseObject extracts a LogEntry from a map.
func (r *LogRenderer) parseObject(obj map[string]any) LogEntry {
	entry := LogEntry{Fields: make(map[string]string)}

	// Extract timestamp — try common field names.
	for _, name := range []string{"_timestamp", "timestamp", "@timestamp", "time", "ts"} {
		if v, ok := obj[name]; ok {
			if t, err := parseTimestamp(v); err == nil {
				entry.Timestamp = t
				break
			}
		}
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Extract message.
	for _, name := range []string{"message", "msg", "log", "text"} {
		if v, ok := obj[name]; ok {
			if s, ok := v.(string); ok {
				entry.Message = s
				break
			}
		}
	}

	// Extract severity.
	for _, name := range []string{"level", "severity", "log_level", "loglevel", "lvl"} {
		if v, ok := obj[name]; ok {
			if s, ok := v.(string); ok {
				entry.Severity = s
				break
			}
		}
	}

	// Remaining string fields become key=value pairs.
	for k, v := range obj {
		if s, ok := v.(string); ok {
			entry.Fields[k] = s
		} else if f, ok := v.(float64); ok {
			entry.Fields[k] = fmt.Sprintf("%v", f)
		} else if b, ok := v.(bool); ok {
			entry.Fields[k] = fmt.Sprintf("%v", b)
		}
	}

	return entry
}

// detectSeverity looks up the severity field name in fields.
func (r *LogRenderer) detectSeverity(fields map[string]string) string {
	for _, name := range []string{"level", "severity", "log_level", "loglevel", "lvl"} {
		if v, ok := fields[name]; ok {
			return v
		}
	}

	return ""
}

// parseTimestamp tries to parse a timestamp value from various formats.
func parseTimestamp(v any) (time.Time, error) {
	switch t := v.(type) {
	case float64:
		// Microseconds Unix epoch.
		return time.UnixMicro(int64(t)), nil
	case int64:
		// Nanoseconds.
		if t > 1e15 {
			return time.UnixMicro(0).Add(time.Duration(t) / time.Microsecond), nil
		}
		// Seconds.
		return time.Unix(t, 0), nil
	case string:
		// Try RFC3339.
		return time.Parse(time.RFC3339, t)
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp type %T", v)
}

// formatTimestamp formats a timestamp for display.
func formatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000")
}

// colorizeSeverity applies ANSI color to a severity string.
// Callers must check noColor before invoking.
func colorizeSeverity(level string, s string) string {
	switch strings.ToUpper(level) {
	case "ERROR", "ERR", "FATAL", "CRITICAL":
		return "\x1b[31m" + s + "\x1b[0m" // red
	case "WARN", "WARNING":
		return "\x1b[33m" + s + "\x1b[0m" // yellow
	case "INFO":
		return "\x1b[32m" + s + "\x1b[0m" // green
	case "DEBUG", "TRACE":
		return "\x1b[2m" + s + "\x1b[0m" // dim
	default:
		return s
	}
}

// ansiRegex matches ANSI escape sequences.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TrimWidth returns the display width of s, stripping ANSI codes.
func TrimWidth(s string, maxLen int) string {
	// Strip ANSI codes for display.
	plain := ansiRegex.ReplaceAllString(s, "")
	if utf8.RuneCountInString(plain) <= maxLen {
		return s
	}
	// Truncate plain text; return original with ANSI intact.
	runes := []rune(plain)
	return string(runes[:maxLen-1]) + "…"
}

// IsNoColor returns true if the NO_COLOR environment variable is set.
func IsNoColor() bool {
	return os.Getenv("NO_COLOR") != ""
}
