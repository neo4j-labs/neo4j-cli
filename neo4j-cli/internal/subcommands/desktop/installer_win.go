// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop — installer_win.go owns the Windows install action:
// invoke the verified NSIS installer with `/S` for silent install. The
// default target is `%LOCALAPPDATA%\Programs\neo4j-desktop\`; `--target-dir`
// maps to NSIS's `/D=<path>` switch (which must be the LAST argv entry
// and does not accept paths with spaces).
package desktop

import (
	"context"
	"os/exec"

	"github.com/neo4j/cli/common/clierr"
)

var (
	installerWinRunCmdFn = func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}
)

// SetInstallerWinRunCmdFnForTest overrides the exec runner.
func SetInstallerWinRunCmdFnForTest(fn func(context.Context, string, ...string) error) func() {
	prev := installerWinRunCmdFn
	installerWinRunCmdFn = fn
	return func() { installerWinRunCmdFn = prev }
}

// installWindows runs the NSIS installer with `/S`. exec.CommandContext
// passes args separately so the no-shell-no-quoting contract holds and
// NSIS sees `/D=<path>` as a single argv entry. `/D=` must be the LAST
// argv entry and does not accept paths containing spaces.
func installWindows(ctx context.Context, plan InstallPlan) error {
	args := []string{"/S"}
	if plan.TargetDir != "" {
		// `/D=<path>` MUST be the last argv entry; future callers adding
		// args must not append after it.
		args = append(args, "/D="+plan.TargetDir)
	}

	if err := installerWinRunCmdFn(ctx, plan.ArtifactPath, args...); err != nil {
		return clierr.NewFatalError(
			"desktop install (windows): NSIS installer %s failed: %s",
			plan.ArtifactPath, err.Error())
	}
	setLastInstalledTargetDir(plan.TargetDir)
	return nil
}
