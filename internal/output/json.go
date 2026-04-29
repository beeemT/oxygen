package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// Format represents an output format.
type Format string

const (
	FormatJSON   Format = "json"
	FormatPretty Format = "pretty"
	FormatTable  Format = "table"
	FormatLog    Format = "log"
	FormatCSV    Format = "csv"
)

// Writer writes formatted output to stdout/stderr.
type Writer struct {
	format Format
	quiet  bool
	stdout io.Writer
	stderr io.Writer
}

// NewWriter returns a Writer configured for the given format.
func NewWriter(format Format, quiet bool) *Writer {
	return &Writer{
		format: format,
		quiet:  quiet,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// SetStdout sets the stdout writer (useful for testing).
func (w *Writer) SetStdout(w2 io.Writer) { w.stdout = w2 }

// SetStderr sets the stderr writer (useful for testing).
func (w *Writer) SetStderr(w2 io.Writer) { w.stderr = w2 }

// Stdout returns the configured stdout writer.
func (w *Writer) Stdout() io.Writer { return w.stdout }

// Stderr returns the configured stderr writer.
func (w *Writer) Stderr() io.Writer { return w.stderr }

// IsTerminal returns true if stdout is connected to a terminal.
func (w *Writer) IsTerminal() bool {
	fd := fileDescriptor(w.stdout)

	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// fileDescriptor returns the OS file descriptor for an io.Writer, or 0 if unknown.
func fileDescriptor(w io.Writer) uintptr {
	if f, ok := w.(*os.File); ok {
		return f.Fd()
	}

	return 0
}

// WriteJSON writes v in the configured JSON format.
func (w *Writer) WriteJSON(v any) error {
	switch w.format {
	case FormatPretty:
		return w.jsonIndent(v)
	default:
		return w.json(v)
	}
}

func (w *Writer) json(v any) error {
	enc := json.NewEncoder(w.stdout)
	enc.SetEscapeHTML(false)

	return enc.Encode(v)
}

func (w *Writer) jsonIndent(v any) error {
	enc := json.NewEncoder(w.stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	return enc.Encode(v)
}

// Error prints a message to stderr with the "Error:" prefix.
// In JSON mode it outputs a structured error object.
func (w *Writer) Error(code int, message string) {
	if w.format == FormatJSON {
		err := w.json(struct {
			Error    string `json:"error"`
			Code     int    `json:"code"`
			ExitCode int    `json:"exit_code"`
		}{
			Error:    message,
			Code:     code,
			ExitCode: code,
		})
		if err != nil {
			fmt.Fprintf(w.stderr, "Error: %s [exit %d]\n", message, code)
		}

		return
	}
	fmt.Fprintf(w.stderr, "Error: %s [exit %d]\n", message, code)
}

// Info prints an informational message to stderr, suppressed in quiet mode.
func (w *Writer) Info(format string, args ...any) {
	if w.quiet {
		return
	}
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintln(w.stderr, msg)
}

// Warning prints a warning to stderr.
func (w *Writer) Warning(format string, args ...any) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintln(w.stderr, "WARNING: "+msg)
}

// Prompt reads a non-empty value from stdin with the given prompt.
func Prompt(message string) (string, error) {
	fmt.Print(message)
	var input string
	_, err := fmt.Fscanln(os.Stdin, &input)

	return input, err
}
