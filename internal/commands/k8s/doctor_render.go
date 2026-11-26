// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
	toon "github.com/toon-format/toon-go"
)

// renderReport dispatches the doctor report to the format-resolved
// renderer. JSON and toon shapes are byte-identical regardless of TTY. TTY
// auto-detection only fires when --format was not explicit.
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
// (CheckResult.Label) stay elided in the toon view.
func renderDoctorToon(w io.Writer, report DoctorReport) error {
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	// Details carry child stderr, and toon rejects C0 control bytes.
	v = commonoutput.WalkStrings(v, commonoutput.StripControl)
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
	case StatusWarn:
		return "WARN"
	default:
		return strings.ToUpper(s)
	}
}

// renderDoctorTable emits one row per check (`Check  Status  Detail`) with
// columns padded so the status keyword aligns regardless of label length.
// A trailing summary line names readiness and the learned versions.
func renderDoctorTable(w io.Writer, report DoctorReport) error {
	const headerCheck, headerStatus, headerDetail = "CHECK", "STATUS", "DETAIL"
	checkW := len(headerCheck)
	statusW := len(headerStatus)
	for _, c := range report.Checks {
		if l := len(c.Label); l > checkW {
			checkW = l
		}
		if l := len(statusKeyword(c.Status)); l > statusW {
			statusW = l
		}
	}

	// Detail is rendered last, with no trailing pad, so long detail strings
	// do not trigger column wrapping.
	if _, err := fmt.Fprintf(w, "%-*s  %-*s  %s\n", checkW, headerCheck, statusW, headerStatus, headerDetail); err != nil {
		return err
	}
	for _, c := range report.Checks {
		if _, err := fmt.Fprintf(w, "%-*s  %-*s  %s\n", checkW, commonoutput.StripControl(c.Label), statusW, statusKeyword(c.Status), commonoutput.StripControl(c.Detail)); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	summary := report.Summary
	parts := []string{fmt.Sprintf("ready=%t", summary.Ready)}
	if summary.KubectlVersion != "" {
		parts = append(parts, "kubectl_version="+summary.KubectlVersion)
	}
	if summary.HelmVersion != "" {
		parts = append(parts, "helm_version="+summary.HelmVersion)
	}
	if summary.ServerVersion != "" {
		parts = append(parts, "server_version="+summary.ServerVersion)
	}
	if _, err := fmt.Fprintf(w, "Summary: %s\n", strings.Join(parts, ", ")); err != nil {
		return err
	}
	if summary.NextStep != "" {
		if _, err := fmt.Fprintf(w, "Next step: %s\n", commonoutput.StripControl(summary.NextStep)); err != nil {
			return err
		}
	}
	return nil
}
