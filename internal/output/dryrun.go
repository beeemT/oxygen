package output

import (
	"encoding/json"
	"io"
	"time"
)

// DryRunRequest is the combined dry-run output shape matching the plan spec.
type DryRunRequest struct {
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers"`
	Body         any               `json:"body"`
	ResolvedTime ResolvedTime      `json:"resolved_time"`
}

// ResolvedTime describes the resolved time window.
type ResolvedTime struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	StartUs int64  `json:"start_us"`
	EndUs   int64  `json:"end_us"`
}

// WriteDryRun writes a dry-run request description to out.
func WriteDryRun(out io.Writer, req DryRunRequest, startUs int64, endUs int64) error {
	req.ResolvedTime = ResolvedTime{
		Start:   time.UnixMicro(startUs).Format(time.RFC3339Nano),
		End:     time.UnixMicro(endUs).Format(time.RFC3339Nano),
		StartUs: startUs,
		EndUs:   endUs,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(req)
}
