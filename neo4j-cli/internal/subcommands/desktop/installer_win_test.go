// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
)

// winInstallerRecorder captures every exec invocation so a single test
// can assert on the NSIS argv shape and the optional `/D=<path>` switch.
type winInstallerRecorder struct {
	name string
	args []string
	err  error
}

func (r *winInstallerRecorder) record(_ context.Context, name string, args ...string) error {
	r.name = name
	r.args = append([]string{}, args...)
	return r.err
}

func TestInstallerWin_HappyPath_RunsNSISWithSilentFlag(t *testing.T) {
	t.Cleanup(desktop.ClearLastInstalledTargetDirForTest)
	rec := &winInstallerRecorder{}
	t.Cleanup(desktop.SetInstallerWinRunCmdFnForTest(rec.record))

	plan := desktop.InstallPlan{
		ArtifactPath: `C:\Users\foo\AppData\Local\Temp\neo4j-desktop-2-windows-2.0.23.exe`,
		TargetDir:    `C:\Users\foo\AppData\Local\Programs\neo4j-desktop`,
		Version:      "2.0.23",
	}
	if err := desktop.RunInstallActionForTest_Windows(t, plan); err != nil {
		t.Fatalf("installWindows: %v", err)
	}

	if rec.name != plan.ArtifactPath {
		t.Fatalf("NSIS exec must point at the artifact path; got %q want %q", rec.name, plan.ArtifactPath)
	}
	if len(rec.args) == 0 || rec.args[0] != "/S" {
		t.Fatalf("NSIS argv must start with /S; got %v", rec.args)
	}
}

func TestInstallerWin_TargetDir_AppendsDSwitch(t *testing.T) {
	t.Cleanup(desktop.ClearLastInstalledTargetDirForTest)
	rec := &winInstallerRecorder{}
	t.Cleanup(desktop.SetInstallerWinRunCmdFnForTest(rec.record))

	plan := desktop.InstallPlan{
		ArtifactPath: `C:\Users\foo\AppData\Local\Temp\neo4j-desktop-2-windows-2.0.24.exe`,
		TargetDir:    `D:\Apps\Neo4jDesktop`,
		Version:      "2.0.24",
	}
	if err := desktop.RunInstallActionForTest_Windows(t, plan); err != nil {
		t.Fatalf("installWindows: %v", err)
	}

	// `/D=<path>` must be the LAST argv entry per NSIS convention.
	if len(rec.args) < 2 {
		t.Fatalf("expected at least /S and /D=<path>; got %v", rec.args)
	}
	last := rec.args[len(rec.args)-1]
	wantSwitch := "/D=" + plan.TargetDir
	if last != wantSwitch {
		t.Fatalf("/D=<path> must be the last argv entry; got last=%q want %q (full argv: %v)",
			last, wantSwitch, rec.args)
	}
}

func TestInstallerWin_ExecFailure_Surfaces(t *testing.T) {
	t.Cleanup(desktop.ClearLastInstalledTargetDirForTest)
	rec := &winInstallerRecorder{err: errors.New("installer exit status 1")}
	t.Cleanup(desktop.SetInstallerWinRunCmdFnForTest(rec.record))

	plan := desktop.InstallPlan{
		ArtifactPath: `C:\bad.exe`,
		TargetDir:    "",
		Version:      "bad",
	}
	err := desktop.RunInstallActionForTest_Windows(t, plan)
	if err == nil {
		t.Fatalf("expected NSIS exec failure to surface")
	}
	if !strings.Contains(err.Error(), "NSIS installer") || !strings.Contains(err.Error(), "C:\\bad.exe") {
		t.Fatalf("error must mention NSIS + the artifact path; got %v", err)
	}
}

// TestInstallerDispatch_UnsupportedOS_Errors_AtOrchestrationLayer asserts the
// realRunInstallAction dispatcher rejects an unknown OS — important so a
// future port that forgets to wire a per-OS branch fails loud rather
// than silent.
func TestInstallerDispatch_UnsupportedOS_Errors_AtOrchestrationLayer(t *testing.T) {
	h := newInstallHelper(t)
	desktop.SetInstallGoosFnForTest(func() string { return "plan9" })

	err := h.run("install")
	if err == nil {
		t.Fatalf("expected unsupported-OS error on plan9")
	}
	if !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("error must mention the unsupported OS; got %v", err)
	}
}
