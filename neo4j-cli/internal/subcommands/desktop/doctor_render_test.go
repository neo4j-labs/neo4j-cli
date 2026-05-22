// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
)

// pinAllPassSeams installs check seams that produce an all-PASS report with
// `port=44222` set so renderer tests can assert the summary line without
// requiring real network / disk I/O.
func pinAllPassSeams(t *testing.T) {
	t.Helper()
	t.Cleanup(desktop.SetCheckInstallPresentFnForTest(func() desktop.CheckResult {
		return desktop.CheckResult{Name: desktop.CheckInstallPresent, Label: desktop.LabelInstallPresent, Status: desktop.StatusPass, Detail: "/Applications/Neo4j Desktop.app (version 2.0.0)"}
	}))
	t.Cleanup(desktop.SetCheckDataDirFnForTest(func(_ context.Context, _ afero.Fs, _ desktopclient.ProbeResult) (desktop.CheckResult, string) {
		return desktop.CheckResult{Name: desktop.CheckDataDir, Label: desktop.LabelDataDir, Status: desktop.StatusPass, Detail: "/home/test/.config/Neo4j Desktop"}, "/home/test/.config/Neo4j Desktop"
	}))
	t.Cleanup(desktop.SetCheckAuthDataReadableFnForTest(func(_ afero.Fs, _ string) desktop.CheckResult {
		return desktop.CheckResult{Name: desktop.CheckAuthDataReadable, Label: desktop.LabelAuthDataReadable, Status: desktop.StatusPass, Detail: "Auth data readable."}
	}))
	t.Cleanup(desktop.SetCheckStandardProbeFnForTest(func(_ context.Context, _ int) (desktop.CheckResult, desktopclient.ProbeResult) {
		return desktop.CheckResult{Name: desktop.CheckStandardProbe, Label: desktop.LabelStandardProbe, Status: desktop.StatusPass, Detail: "Relate API reachable at http://localhost:44222"}, desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"}
	}))
	t.Cleanup(desktop.SetCheckInfoAppFnForTest(func(_ context.Context, _ desktopclient.ProbeResult) desktop.CheckResult {
		return desktop.CheckResult{
			Name:     desktop.CheckInfoApp,
			Label:    desktop.LabelInfoApp,
			Status:   desktop.StatusPass,
			Detail:   "version=2.0.0 appPath=/Applications/Neo4j Desktop.app dataPath=/home/test/.config/Neo4j Desktop",
			Version:  "2.0.0",
			AppPath:  "/Applications/Neo4j Desktop.app",
			DataPath: "/home/test/.config/Neo4j Desktop",
		}
	}))
	t.Cleanup(desktop.SetBuildAuthClientFnForTest(func(_ afero.Fs, _ string, _ desktopclient.ProbeResult) (*desktopclient.Client, error) {
		return nil, nil
	}))
	t.Cleanup(desktop.SetCheckAuthenticatedProbeFnForTest(func(_ context.Context, _ *desktopclient.Client) desktop.CheckResult {
		return desktop.CheckResult{Name: desktop.CheckAuthenticated, Label: desktop.LabelAuthenticated, Status: desktop.StatusPass, Detail: "Authenticated call against Desktop relate API succeeded."}
	}))
}

// pinProbeFailSeams installs check seams that produce a probe-FAIL report —
// install / data_dir / auth_data PASS, standard_probe FAILs with a hint, the
// auth probe is SKIPped by the orchestrator, and the summary carries
// `reachable=false` + a NextStep.
func pinProbeFailSeams(t *testing.T, pinnedPort int) {
	t.Helper()
	t.Cleanup(desktop.SetCheckInstallPresentFnForTest(func() desktop.CheckResult {
		return desktop.CheckResult{Name: desktop.CheckInstallPresent, Label: desktop.LabelInstallPresent, Status: desktop.StatusPass, Detail: "/Applications/Neo4j Desktop.app"}
	}))
	t.Cleanup(desktop.SetCheckDataDirFnForTest(func(_ context.Context, _ afero.Fs, _ desktopclient.ProbeResult) (desktop.CheckResult, string) {
		return desktop.CheckResult{Name: desktop.CheckDataDir, Label: desktop.LabelDataDir, Status: desktop.StatusPass, Detail: "/home/test/.config/Neo4j Desktop"}, "/home/test/.config/Neo4j Desktop"
	}))
	t.Cleanup(desktop.SetCheckAuthDataReadableFnForTest(func(_ afero.Fs, _ string) desktop.CheckResult {
		return desktop.CheckResult{Name: desktop.CheckAuthDataReadable, Label: desktop.LabelAuthDataReadable, Status: desktop.StatusPass, Detail: "Auth data readable."}
	}))
	t.Cleanup(desktop.SetCheckStandardProbeFnForTest(func(_ context.Context, _ int) (desktop.CheckResult, desktopclient.ProbeResult) {
		return desktop.CheckResult{
			Name:   desktop.CheckStandardProbe,
			Label:  desktop.LabelStandardProbe,
			Status: desktop.StatusFail,
			Detail: "No relate server answered on the standard port range (44222..44232).",
			Hint:   "Start Neo4j Desktop 2 from your applications menu, or pass --port if it's on a non-default port.",
		}, desktopclient.ProbeResult{}
	}))
	_ = pinnedPort // present so callers can document the leaf's --port arg
}

// invokeDoctor assembles the desktop tree, points it at the in-memory fs cfg,
// drives the leaf with the supplied args, and returns its captured stdout +
// the error from Execute. flags.RegisterOutputFlag installs the persistent
// --format flag and its PreRunE so `cfg.Global.Format()` reflects the value.
func invokeDoctor(t *testing.T, cfg *clicfg.Config, args ...string) (string, error) {
	t.Helper()
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"doctor"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

// TestDoctor_FormatJSON_EmitsParseableDocument exercises the json renderer
// end-to-end through the leaf's RunE and asserts the wire shape matches
// REQ-F-006: `{checks: [...], summary: {reachable, port?, standard_port_range, next_step?}}`.
func TestDoctor_FormatJSON_EmitsParseableDocument(t *testing.T) {
	pinAllPassSeams(t)
	cfg := newDoctorCfg(t)

	out, err := invokeDoctor(t, cfg, "--format", "json")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got desktop.DoctorReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout was:\n%s", err, out)
	}

	if len(got.Checks) != 6 {
		t.Fatalf("expected 6 checks; got %d (rows=%+v)", len(got.Checks), got.Checks)
	}
	wantNames := []string{
		desktop.CheckInstallPresent,
		desktop.CheckStandardProbe,
		desktop.CheckInfoApp,
		desktop.CheckDataDir,
		desktop.CheckAuthDataReadable,
		desktop.CheckAuthenticated,
	}
	for i, name := range wantNames {
		if got.Checks[i].Name != name {
			t.Errorf("check[%d].Name = %q; want %q", i, got.Checks[i].Name, name)
		}
		if got.Checks[i].Status != desktop.StatusPass {
			t.Errorf("check[%d].Status = %q; want pass", i, got.Checks[i].Status)
		}
	}
	if !got.Summary.Reachable {
		t.Errorf("Summary.Reachable = false; want true (all-pass)")
	}
	if got.Summary.Port == nil || *got.Summary.Port != 44222 {
		t.Errorf("Summary.Port = %v; want *int(44222)", got.Summary.Port)
	}
	if !got.Summary.StandardPortRange {
		t.Errorf("Summary.StandardPortRange = false; want true")
	}
	if got.Summary.NextStep != "" {
		t.Errorf("Summary.NextStep = %q; want empty for all-pass", got.Summary.NextStep)
	}
}

// TestDoctor_FormatJSON_ElidesLabelField guards REQ-F-008: the human-readable
// label (which deliberately could contain row wording) MUST stay out of the
// JSON payload — only the neutral `name` field carries the row identity on
// the wire. The Label field has a `json:"-"` tag for this reason.
func TestDoctor_FormatJSON_ElidesLabelField(t *testing.T) {
	pinAllPassSeams(t)
	cfg := newDoctorCfg(t)

	out, err := invokeDoctor(t, cfg, "--format", "json")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, `"label"`) || strings.Contains(out, `"Label"`) {
		t.Errorf("JSON output unexpectedly contains a label key:\n%s", out)
	}
}

// TestDoctor_FormatToon_EmitsDocument exercises the toon renderer end-to-end
// and asserts the document mentions every check name and the summary keys
// (reachable / port / standard_port_range). The exact toon shape is asserted
// loosely (substring match) because toon-go's formatting choices are not the
// SUT here — the wire shape is.
func TestDoctor_FormatToon_EmitsDocument(t *testing.T) {
	pinAllPassSeams(t)
	cfg := newDoctorCfg(t)

	out, err := invokeDoctor(t, cfg, "--format", "toon")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{
		"checks",
		"summary",
		"reachable",
		"port",
		"standard_port_range",
		desktop.CheckInstallPresent,
		desktop.CheckAuthenticated,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("toon output missing %q; got:\n%s", want, out)
		}
	}
}

// TestDoctor_FormatTable_RendersAlignedColumns_AllPass asserts the table
// renderer emits the three header columns, every label, PASS keywords,
// and a trailing `Summary:` line carrying reachable=true + port=44222.
func TestDoctor_FormatTable_RendersAlignedColumns_AllPass(t *testing.T) {
	pinAllPassSeams(t)
	cfg := newDoctorCfg(t)

	out, err := invokeDoctor(t, cfg, "--format", "table")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for _, want := range []string{
		"CHECK", "STATUS", "DETAIL",
		desktop.LabelInstallPresent,
		desktop.LabelStandardProbe,
		desktop.LabelInfoApp,
		desktop.LabelDataDir,
		desktop.LabelAuthDataReadable,
		desktop.LabelAuthenticated,
		"PASS",
		"version=2.0.0",
		"appPath=/Applications/Neo4j Desktop.app",
		"Summary: reachable=true",
		"port=44222",
		"standard_port_range=true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}

	// REQ-F-008: the auth_data row label must not mention secret/JWT/key/salt
	// in the rendered table.
	for _, banned := range []string{"secret", "JWT", " salt", "private key"} {
		if strings.Contains(out, banned) {
			t.Errorf("table output unexpectedly contains forbidden word %q (REQ-F-008); got:\n%s", banned, out)
		}
	}
}

// TestDoctor_FormatTable_ProbeFail_RendersSkipAndNextStep covers the
// failure path: probe FAILs, auth-probe is SKIPped by the orchestrator,
// summary carries reachable=false + NextStep mentioning the hint.
func TestDoctor_FormatTable_ProbeFail_RendersSkipAndNextStep(t *testing.T) {
	pinProbeFailSeams(t, 0)
	cfg := newDoctorCfg(t)

	out, err := invokeDoctor(t, cfg, "--format", "table")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{
		"FAIL",
		"SKIP",
		"Summary: reachable=false",
		"Next step:",
		"Start Neo4j Desktop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q; got:\n%s", want, out)
		}
	}
	// On a probe FAIL the orchestrator never saw a successful probe, so
	// `Port` should be unset (no `port=` substring in the summary line).
	if strings.Contains(out, "port=") {
		t.Errorf("table summary should omit port= when probe FAILed; got:\n%s", out)
	}
}

// TestDoctor_FormatExplicit_IdenticalAcrossTTYContexts verifies the
// acceptance-criterion: stdout is byte-identical between TTY and non-TTY
// stdout when `--format` is explicit (auto-table never sneaks in). We swap
// commonoutput.StdoutIsTerminal in both directions and assert byte equality.
func TestDoctor_FormatExplicit_IdenticalAcrossTTYContexts(t *testing.T) {
	cases := []struct {
		name   string
		format string
	}{
		{"json", "json"},
		{"toon", "toon"},
		{"table", "table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pinAllPassSeams(t)
			cfg := newDoctorCfg(t)

			// Force TTY=true.
			prev := commonoutput.StdoutIsTerminal
			commonoutput.StdoutIsTerminal = func(_ io.Writer) bool { return true }
			ttyOut, err := invokeDoctor(t, cfg, "--format", tc.format)
			commonoutput.StdoutIsTerminal = prev
			if err != nil {
				t.Fatalf("TTY Execute: %v", err)
			}

			// Force TTY=false (piped).
			cfg2 := newDoctorCfg(t)
			pinAllPassSeams(t)
			prev2 := commonoutput.StdoutIsTerminal
			commonoutput.StdoutIsTerminal = func(_ io.Writer) bool { return false }
			pipedOut, err := invokeDoctor(t, cfg2, "--format", tc.format)
			commonoutput.StdoutIsTerminal = prev2
			if err != nil {
				t.Fatalf("piped Execute: %v", err)
			}

			if ttyOut != pipedOut {
				t.Errorf("stdout differs between TTY and non-TTY for explicit --format %s:\nTTY:\n%s\nPIPED:\n%s", tc.format, ttyOut, pipedOut)
			}
		})
	}
}

// newDoctorCfgDefaultFormat builds a Config whose persisted format is the
// sentinel `default`, so `cfg.Global.Format()` returns "default" and the
// renderer's auto-detection (TTY → table, piped → JSON) actually fires.
// `newDoctorCfg` from `doctor_orchestrator_test.go` pins format to "json"
// for the orchestrator suite, which would short-circuit auto-detect.
func newDoctorCfgDefaultFormat(t *testing.T) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
}

// TestDoctor_DefaultFormat_TTYIsTable verifies the auto-detection path:
// no --format flag, TTY stdout → table renderer (PASS keyword present).
func TestDoctor_DefaultFormat_TTYIsTable(t *testing.T) {
	pinAllPassSeams(t)
	cfg := newDoctorCfgDefaultFormat(t)

	prev := commonoutput.StdoutIsTerminal
	commonoutput.StdoutIsTerminal = func(_ io.Writer) bool { return true }
	t.Cleanup(func() { commonoutput.StdoutIsTerminal = prev })

	out, err := invokeDoctor(t, cfg)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "PASS") || !strings.Contains(out, "Summary:") {
		t.Errorf("expected table output on TTY default; got:\n%s", out)
	}
}

// TestDoctor_DefaultFormat_NonTTYIsJSON verifies the auto-detection path:
// no --format flag, non-TTY stdout → JSON renderer (parseable shape).
func TestDoctor_DefaultFormat_NonTTYIsJSON(t *testing.T) {
	pinAllPassSeams(t)
	cfg := newDoctorCfgDefaultFormat(t)

	prev := commonoutput.StdoutIsTerminal
	commonoutput.StdoutIsTerminal = func(_ io.Writer) bool { return false }
	t.Cleanup(func() { commonoutput.StdoutIsTerminal = prev })

	out, err := invokeDoctor(t, cfg)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got desktop.DoctorReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("default non-TTY output not JSON: %v\nstdout:\n%s", err, out)
	}
	if len(got.Checks) != 6 {
		t.Errorf("default non-TTY JSON: expected 6 checks; got %d", len(got.Checks))
	}
}
