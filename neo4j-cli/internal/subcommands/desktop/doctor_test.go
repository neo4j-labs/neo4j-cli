// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
)

// TestDoctor_Help_BuildsAndListsLeaf is the scaffold smoke test for the
// `neo4j-cli desktop doctor` leaf — it asserts the command builds via
// desktop.NewCmd, that `--help` renders the Long + Example without error,
// and that the leaf is registered under `desktop` so `desktop --help`
// lists it. The per-check helpers and renderers land in task-002..004; this
// test only guards the cobra wiring + Example gate.
func TestDoctor_Help_BuildsAndListsLeaf(t *testing.T) {
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	t.Run("doctor --help renders Long and Example", func(t *testing.T) {
		cmd := desktop.NewCmd(cfg)
		flags.RegisterOutputFlag(cmd, cfg)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"doctor", "--help"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		got := out.String()
		for _, want := range []string{
			"doctor",
			"Run an ordered sequence of six health checks",
			"neo4j-cli desktop doctor",
			"--port",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("expected help output to contain %q; got:\n%s", want, got)
			}
		}
	})

	t.Run("desktop --help lists doctor under Available Commands", func(t *testing.T) {
		cmd := desktop.NewCmd(cfg)
		flags.RegisterOutputFlag(cmd, cfg)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--help"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "doctor") {
			t.Errorf("expected `desktop --help` to list `doctor`; got:\n%s", got)
		}
	})

	t.Run("doctor RunE returns nil regardless of check outcomes", func(t *testing.T) {
		// Pin every check seam so the leaf's orchestrator runs without
		// touching disk / network and produces a deterministic report —
		// even a worst-case mix of FAILs still exits 0 per REQ-F-009.
		t.Cleanup(desktop.SetCheckInstallPresentFnForTest(func() desktop.CheckResult {
			return desktop.CheckResult{Name: desktop.CheckInstallPresent, Label: desktop.LabelInstallPresent, Status: desktop.StatusFail, Detail: "no install", Hint: "install hint"}
		}))
		t.Cleanup(desktop.SetCheckDataDirFnForTest(func(_ context.Context, _ afero.Fs, _ desktopclient.ProbeResult) (desktop.CheckResult, string) {
			return desktop.CheckResult{Name: desktop.CheckDataDir, Label: desktop.LabelDataDir, Status: desktop.StatusFail, Detail: "no data dir", Hint: "data dir hint"}, ""
		}))
		t.Cleanup(desktop.SetCheckAuthDataReadableFnForTest(func(_ afero.Fs, _ string) desktop.CheckResult {
			return desktop.CheckResult{Name: desktop.CheckAuthDataReadable, Label: desktop.LabelAuthDataReadable, Status: desktop.StatusFail, Detail: "no auth data", Hint: "auth data hint"}
		}))
		t.Cleanup(desktop.SetCheckStandardProbeFnForTest(func(_ context.Context, _ int) (desktop.CheckResult, desktopclient.ProbeResult) {
			return desktop.CheckResult{Name: desktop.CheckStandardProbe, Label: desktop.LabelStandardProbe, Status: desktop.StatusFail, Detail: "probe FAIL", Hint: "probe hint"}, desktopclient.ProbeResult{}
		}))

		cmd := desktop.NewCmd(cfg)
		flags.RegisterOutputFlag(cmd, cfg)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"doctor"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected Execute to return nil (exit 0) regardless of check FAILs; got %v", err)
		}
	})
}
