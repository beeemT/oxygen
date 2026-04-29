package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// Table writes a formatted table using text/tabwriter.
type Table struct {
	w       *tabwriter.Writer
	headers []string
	widths  []int
}

// NewTable returns a Table that writes to out. headers is the column header row.
func NewTable(out io.Writer, headers ...string) *Table {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	t := &Table{w: tw, headers: headers, widths: make([]int, len(headers))}
	for i, h := range headers {
		t.widths[i] = len(h)
	}
	io.WriteString(t.w, strings.Join(headers, "\t")+"\n")
	for i := range headers {
		io.WriteString(t.w, strings.Repeat("─", t.widths[i])+"\t")
	}
	io.WriteString(t.w, "\n")

	return t
}

// Row appends a row of string values. The number of values must match the
// number of headers passed to NewTable.
func (t *Table) Row(vals ...string) {
	// Track max widths.
	for i, v := range vals {
		if i >= len(t.widths) {
			break
		}
		l := visibleWidth(v)
		if l > t.widths[i] {
			t.widths[i] = l
		}
	}
	// Pad vals to header count.
	for i := len(vals); i < len(t.headers); i++ {
		vals = append(vals, "")
	}
	_, _ = fmt.Fprintln(t.w, strings.Join(vals[:len(t.headers)], "\t"))
}

// Flush flushes the table to the underlying writer.
func (t *Table) Flush() error {
	return t.w.Flush()
}

// visibleWidth returns the display width of s (handles ANSI escape codes).
func visibleWidth(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

// stripANSI removes all ANSI escape sequences from s.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip 'm'
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
