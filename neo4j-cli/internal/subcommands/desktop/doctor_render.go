// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
	toon "github.com/toon-format/toon-go"
)

// renderReport dispatches the doctor report to the format-resolved renderer.
// JSON / toon shapes are byte-identical regardless of TTY — TTY
// auto-detection only fires when `--format` was NOT explicit.
func renderReport(cmd *cobra.Command, cfg *clicfg.Config, report DoctorReport) error {
	switch commonoutput.ResolveOutput(cmd, cfg) {
	case "json":
		return renderDoctorJSON(cmd.OutOrStdout(), report)
	case "toon":
		return renderDoctorToon(cmd.OutOrStdout(), report)
	default:
		return renderDoctorTable(cmd.OutOrStdout(), report)
	}
}

func renderDoctorJSON(w io.Writer, report DoctorReport) error {
	b, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// renderDoctorToon marshals through JSON first so `json:"-"` field tags
// (e.g. CheckResult.Label) stay elided in the toon view.
func renderDoctorToon(w io.Writer, report DoctorReport) error {
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	out, err := toon.Marshal(v, toon.WithLengthMarkers(true))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(out))
	return err
}

// statusKeyword maps the lowercase wire status to its uppercase table
// keyword. Unknown values are upper-cased verbatim so a future extension
// surfaces in the table without a silent fallback.
func statusKeyword(s string) string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	case StatusInfo:
		return "INFO"
	default:
		return strings.ToUpper(s)
	}
}

// renderDoctorTable emits one row per check (`Name  Status  Detail`) with
// columns padded so the status keyword aligns regardless of label length.
// A trailing summary line mentions reachability, port (when known), and
// the next-step hint (when set).
func renderDoctorTable(w io.Writer, report DoctorReport) error {
	const headerName, headerStatus, headerDetail = "CHECK", "STATUS", "DETAIL"
	nameW := len(headerName)
	statusW := len(headerStatus)
	for _, c := range report.Checks {
		if l := len(c.Label); l > nameW {
			nameW = l
		}
		if l := len(statusKeyword(c.Status)); l > statusW {
			statusW = l
		}
	}

	// Detail is rendered last (no trailing pad) so long detail strings
	// don't trigger column wrapping.
	if _, err := fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, headerName, statusW, headerStatus, headerDetail); err != nil {
		return err
	}
	for _, c := range report.Checks {
		detail := commonoutput.StripControl(c.Detail)
		if _, err := fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, commonoutput.StripControl(c.Label), statusW, statusKeyword(c.Status), detail); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Summary: reachable=%t", report.Summary.Reachable); err != nil {
		return err
	}
	if report.Summary.Port != nil {
		if _, err := fmt.Fprintf(w, ", port=%d", *report.Summary.Port); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, ", standard_port_range=%t", report.Summary.StandardPortRange); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if report.Summary.NextStep != "" {
		if _, err := fmt.Fprintf(w, "Next step: %s\n", commonoutput.StripControl(report.Summary.NextStep)); err != nil {
			return err
		}
	}
	return nil
}
