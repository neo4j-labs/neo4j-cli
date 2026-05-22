// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
)

// --- doctor orchestrator (task-003) ----------------------------------------
//
// These tests exercise `runChecks` via `RunChecksForTest` with every per-check
// helper stubbed via the `SetCheck*FnForTest` seams. The stubs return canned
// results so the orchestration logic (dependency-skip, summary build) is the
// only thing under test — the per-check unit tests in
// doctor_checks_test.go cover the helpers themselves.

// pin is a shorthand for installing a full set of check-seam stubs with
// individual control over each check's outcome. Any nil field leaves the
// production helper in place — useful when a test only wants to swap one
// check and let the others run.
type pin struct {
	install    *desktop.CheckResult
	dataDir    *desktop.CheckResult
	dataDirRes string
	authData   *desktop.CheckResult
	probe      *desktop.CheckResult
	probeRes   desktopclient.ProbeResult
	infoApp    *desktop.CheckResult
	clientErr  error
	authProbe  *desktop.CheckResult
}

func pinSeams(t *testing.T, p pin) {
	t.Helper()
	if p.install != nil {
		install := *p.install
		t.Cleanup(desktop.SetCheckInstallPresentFnForTest(func() desktop.CheckResult { return install }))
	}
	if p.dataDir != nil {
		dataDirR := *p.dataDir
		dir := p.dataDirRes
		t.Cleanup(desktop.SetCheckDataDirFnForTest(func(_ context.Context, _ afero.Fs, _ desktopclient.ProbeResult) (desktop.CheckResult, string) {
			return dataDirR, dir
		}))
	}
	if p.authData != nil {
		authR := *p.authData
		t.Cleanup(desktop.SetCheckAuthDataReadableFnForTest(func(_ afero.Fs, _ string) desktop.CheckResult { return authR }))
	}
	if p.probe != nil {
		probeR := *p.probe
		probeRes := p.probeRes
		t.Cleanup(desktop.SetCheckStandardProbeFnForTest(func(_ context.Context, _ int) (desktop.CheckResult, desktopclient.ProbeResult) {
			return probeR, probeRes
		}))
	}
	if p.infoApp != nil {
		infoAppR := *p.infoApp
		t.Cleanup(desktop.SetCheckInfoAppFnForTest(func(_ context.Context, _ desktopclient.ProbeResult) desktop.CheckResult {
			return infoAppR
		}))
	} else {
		// Default to a neutral INFO so orchestrator suites that don't care about
		// /info/app aren't forced to seed it. Production checkInfoApp would
		// attempt the live HTTP call, which the test must NOT trigger.
		t.Cleanup(desktop.SetCheckInfoAppFnForTest(func(_ context.Context, _ desktopclient.ProbeResult) desktop.CheckResult {
			return desktop.CheckResult{Name: desktop.CheckInfoApp, Label: desktop.LabelInfoApp, Status: desktop.StatusInfo, Detail: "unavailable (older Desktop)"}
		}))
	}
	// Stub the client builder so the orchestrator never reaches real disk
	// / network when running the auth probe. clientErr controls whether the
	// builder fails (FAIL on auth probe) or succeeds (the supplied
	// authProbe stub runs).
	t.Cleanup(desktop.SetBuildAuthClientFnForTest(func(_ afero.Fs, _ string, _ desktopclient.ProbeResult) (*desktopclient.Client, error) {
		if p.clientErr != nil {
			return nil, p.clientErr
		}
		// Returning a nil client is fine — the auth-probe helper is also
		// stubbed below, so the nil pointer is never dereferenced.
		return nil, nil
	}))
	if p.authProbe != nil {
		authProbeR := *p.authProbe
		t.Cleanup(desktop.SetCheckAuthenticatedProbeFnForTest(func(_ context.Context, _ *desktopclient.Client) desktop.CheckResult {
			return authProbeR
		}))
	}
}

func newDoctorCfg(t *testing.T) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
}

func passResult(name, label, detail string) *desktop.CheckResult {
	return &desktop.CheckResult{Name: name, Label: label, Status: desktop.StatusPass, Detail: detail}
}

func failResult(name, label, detail, hint string) *desktop.CheckResult {
	return &desktop.CheckResult{Name: name, Label: label, Status: desktop.StatusFail, Detail: detail, Hint: hint}
}

// findCheck pulls a check row by name out of the report — tests assert on
// individual rows rather than positional indexes so reordering the
// orchestrator's run sequence doesn't silently break assertions.
func findCheck(t *testing.T, report desktop.DoctorReport, name string) desktop.CheckResult {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("expected report to contain check %q; got %+v", name, report.Checks)
	return desktop.CheckResult{}
}

// TestDoctor_RunChecks_AllPass — happy path: every check PASSes, summary
// reports reachable=true with the probe's port surfaced.
func TestDoctor_RunChecks_AllPass(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44222."),
		probeRes:   desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"},
		infoApp:    passResult(desktop.CheckInfoApp, desktop.LabelInfoApp, "version=2.0.0 appPath=/Applications/Neo4j Desktop 2.app dataPath=/data"),
		authProbe:  passResult(desktop.CheckAuthenticated, desktop.LabelAuthenticated, "Authenticated call against Desktop relate API succeeded."),
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	if len(got.Checks) != 6 {
		t.Fatalf("expected 6 checks, got %d (%+v)", len(got.Checks), got.Checks)
	}
	for _, c := range got.Checks {
		if c.Status != desktop.StatusPass {
			t.Errorf("expected all checks PASS, got %+v", c)
		}
	}
	if !got.Summary.Reachable {
		t.Errorf("expected summary.reachable=true on all-PASS, got %+v", got.Summary)
	}
	if got.Summary.Port == nil || *got.Summary.Port != 44222 {
		t.Errorf("expected summary.port=44222, got %+v", got.Summary.Port)
	}
	if !got.Summary.StandardPortRange {
		t.Errorf("expected summary.standard_port_range=true with default scan, got %+v", got.Summary)
	}
	if got.Summary.NextStep != "" {
		t.Errorf("expected empty next_step when all PASS, got %q", got.Summary.NextStep)
	}
}

// TestDoctor_RunChecks_ProbeFails_SkipsInfoApp — probe FAIL also cascades
// into a SKIP on the info_app row (its dependency in the chain is the
// probe). The info_app helper itself must NOT be invoked when the probe
// FAILed.
func TestDoctor_RunChecks_ProbeFails_SkipsInfoApp(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      failResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "No relate server answered on the standard port range (44222..44232).", "Start Neo4j Desktop 2 from your applications menu, or pass --port if it's on a non-default port."),
		probeRes:   desktopclient.ProbeResult{},
	})
	// Install a guard AFTER pinSeams so this seam wins over pinSeams's default
	// info_app stub. The orchestrator must NOT invoke checkInfoAppFn at all
	// when the probe FAILed — the SKIP row is produced inline.
	t.Cleanup(desktop.SetCheckInfoAppFnForTest(func(_ context.Context, _ desktopclient.ProbeResult) desktop.CheckResult {
		t.Errorf("checkInfoAppFn should not be invoked when probe FAILed")
		return desktop.CheckResult{}
	}))

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	infoR := findCheck(t, got, desktop.CheckInfoApp)
	if infoR.Status != desktop.StatusSkip {
		t.Fatalf("expected info_app status=skip when probe FAILs, got %+v", infoR)
	}
	if !strings.Contains(infoR.Detail, desktop.CheckStandardProbe) {
		t.Fatalf("expected info_app skip detail to name standard_probe, got %q", infoR.Detail)
	}
}

// TestDoctor_RunChecks_ProbeFails_SkipsAuthProbe — probe FAIL surfaces as a
// `skip` on the authenticated probe with `(depends on standard_probe)`
// detail; summary.reachable=false, port absent, next_step carries the probe
// hint so the table renderer (task-004) can surface it as a one-liner.
func TestDoctor_RunChecks_ProbeFails_SkipsAuthProbe(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      failResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "No relate server answered on the standard port range (44222..44232).", "Start Neo4j Desktop 2 from your applications menu, or pass --port if it's on a non-default port."),
		probeRes:   desktopclient.ProbeResult{},
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	authProbeR := findCheck(t, got, desktop.CheckAuthenticated)
	if authProbeR.Status != desktop.StatusSkip {
		t.Fatalf("expected auth_probe status=skip when probe FAILs, got %+v", authProbeR)
	}
	if !strings.Contains(authProbeR.Detail, "(depends on ") {
		t.Fatalf("expected skip detail to name dependency, got %q", authProbeR.Detail)
	}
	if !strings.Contains(authProbeR.Detail, desktop.CheckStandardProbe) {
		t.Fatalf("expected skip detail to name standard_probe, got %q", authProbeR.Detail)
	}
	if got.Summary.Reachable {
		t.Errorf("expected summary.reachable=false when probe FAILs, got %+v", got.Summary)
	}
	if got.Summary.Port != nil {
		t.Errorf("expected summary.port to be absent when probe FAILs, got %+v", got.Summary.Port)
	}
	if !strings.Contains(got.Summary.NextStep, "Start Neo4j Desktop 2") {
		t.Fatalf("expected next_step to surface probe hint, got %q", got.Summary.NextStep)
	}
}

// TestDoctor_RunChecks_AuthDataFails_SkipsAuthProbe — auth_data FAIL skips
// the auth probe because the salt-derived JWT signing key would be
// unverifiable. Skip detail names auth_data_readable as the prerequisite,
// not standard_probe (which actually PASSed).
func TestDoctor_RunChecks_AuthDataFails_SkipsAuthProbe(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   failResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, `Could not read Desktop auth data under "/data".`, "Start Neo4j Desktop 2 once so it can write its auth data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44222."),
		probeRes:   desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"},
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	authProbeR := findCheck(t, got, desktop.CheckAuthenticated)
	if authProbeR.Status != desktop.StatusSkip {
		t.Fatalf("expected auth_probe status=skip when auth_data FAILs, got %+v", authProbeR)
	}
	if !strings.Contains(authProbeR.Detail, desktop.CheckAuthDataReadable) {
		t.Fatalf("expected skip detail to name auth_data_readable, got %q", authProbeR.Detail)
	}
	if got.Summary.Reachable {
		t.Errorf("expected summary.reachable=false when auth_probe skipped, got %+v", got.Summary)
	}
}

// TestDoctor_RunChecks_AuthProbeFails — probe PASSes, auth_data PASSes,
// auth_probe FAILs (e.g. 401). Summary.reachable must be false because
// reachable requires BOTH probe AND auth_probe to PASS.
func TestDoctor_RunChecks_AuthProbeFails(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44222."),
		probeRes:   desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"},
		authProbe:  failResult(desktop.CheckAuthenticated, desktop.LabelAuthenticated, "Desktop rejected the authenticated call (401).", "Restart Neo4j Desktop 2 to regenerate its token state."),
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	authProbeR := findCheck(t, got, desktop.CheckAuthenticated)
	if authProbeR.Status != desktop.StatusFail {
		t.Fatalf("expected auth_probe status=fail, got %+v", authProbeR)
	}
	if got.Summary.Reachable {
		t.Errorf("expected summary.reachable=false when auth_probe FAILs, got %+v", got.Summary)
	}
	// reachable requires probe AND auth_probe to PASS — probe alone is not
	// enough, so port may still be set (the probe DID PASS) but reachable
	// must be false.
	if got.Summary.Port == nil || *got.Summary.Port != 44222 {
		t.Errorf("expected summary.port=44222 when probe PASSed, got %+v", got.Summary.Port)
	}
}

// TestDoctor_RunChecks_ClientBuilderFails — client construction fails (e.g.
// salt is unreadable mid-flight). The auth probe rolls up as a FAIL with
// the underlying error in its detail, not a SKIP — distinguishing
// "couldn't build client" from "didn't run because upstream failed".
func TestDoctor_RunChecks_ClientBuilderFails(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44222."),
		probeRes:   desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"},
		clientErr:  errors.New("salt read after auth_data check race-disappeared"),
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	authProbeR := findCheck(t, got, desktop.CheckAuthenticated)
	if authProbeR.Status != desktop.StatusFail {
		t.Fatalf("expected auth_probe status=fail when client build fails, got %+v", authProbeR)
	}
	if !strings.Contains(authProbeR.Detail, "race-disappeared") {
		t.Fatalf("expected detail to include underlying error, got %q", authProbeR.Detail)
	}
}

// TestDoctor_RunChecks_PortInRange_StandardPortRangeTrue — when --port is
// inside 44222..44232, standard_port_range=true.
func TestDoctor_RunChecks_PortInRange_StandardPortRangeTrue(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44225."),
		probeRes:   desktopclient.ProbeResult{Port: 44225, Origin: "http://localhost:44225"},
		authProbe:  passResult(desktop.CheckAuthenticated, desktop.LabelAuthenticated, "Authenticated call against Desktop relate API succeeded."),
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 44225)

	if !got.Summary.StandardPortRange {
		t.Errorf("expected standard_port_range=true for --port 44225, got %+v", got.Summary)
	}
}

// TestDoctor_RunChecks_PortOutsideRange_StandardPortRangeFalse — when
// --port is outside 44222..44232, standard_port_range=false even if the
// probe somehow PASSes against that port.
func TestDoctor_RunChecks_PortOutsideRange_StandardPortRangeFalse(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      failResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "No relate server answered on port 12345.", "Start Neo4j Desktop 2 from your applications menu, or pass a different --port."),
		probeRes:   desktopclient.ProbeResult{},
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 12345)

	if got.Summary.StandardPortRange {
		t.Errorf("expected standard_port_range=false for --port 12345, got %+v", got.Summary)
	}
}

// TestDoctor_RunChecks_InstallFails_OthersStillRun — install FAIL does NOT
// skip data_dir / auth_data / probe / auth_probe per REQ-F-003 (independent
// checks all run). Each downstream check is exercised on its own merits.
func TestDoctor_RunChecks_InstallFails_OthersStillRun(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    failResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "Neo4j Desktop 2 install not found for linux.", "Run `neo4j-cli desktop install` or install Neo4j Desktop 2 from https://neo4j.com/download/."),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44222."),
		probeRes:   desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"},
		authProbe:  passResult(desktop.CheckAuthenticated, desktop.LabelAuthenticated, "Authenticated call against Desktop relate API succeeded."),
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	if findCheck(t, got, desktop.CheckInstallPresent).Status != desktop.StatusFail {
		t.Errorf("expected install_present=fail")
	}
	if findCheck(t, got, desktop.CheckDataDir).Status != desktop.StatusPass {
		t.Errorf("expected data_dir=pass even when install fails (independent checks)")
	}
	if findCheck(t, got, desktop.CheckAuthDataReadable).Status != desktop.StatusPass {
		t.Errorf("expected auth_data_readable=pass even when install fails (independent checks)")
	}
	if findCheck(t, got, desktop.CheckStandardProbe).Status != desktop.StatusPass {
		t.Errorf("expected standard_probe=pass even when install fails (independent checks)")
	}
	if findCheck(t, got, desktop.CheckAuthenticated).Status != desktop.StatusPass {
		t.Errorf("expected authenticated_probe=pass even when install fails")
	}
	// reachable=true here despite install FAIL — install_present is not
	// part of the reachable verdict (only probe + auth_probe are).
	if !got.Summary.Reachable {
		t.Errorf("expected summary.reachable=true when probe+auth_probe PASS, got %+v", got.Summary)
	}
	// next_step still surfaces the install FAIL's hint so the user has an
	// actionable line — first FAILing check's hint wins.
	if !strings.Contains(got.Summary.NextStep, "neo4j-cli desktop install") {
		t.Fatalf("expected next_step to surface install hint, got %q", got.Summary.NextStep)
	}
}

// TestDoctor_RunChecks_ChecksInOrder — sanity-check that the rows come out
// in the documented order (REQ-F-207): install, standard_probe, info_app,
// data_dir, auth_data, authenticated_probe.
func TestDoctor_RunChecks_ChecksInOrder(t *testing.T) {
	cfg := newDoctorCfg(t)
	pinSeams(t, pin{
		install:    passResult(desktop.CheckInstallPresent, desktop.LabelInstallPresent, "/Applications/Neo4j Desktop 2.app"),
		dataDir:    passResult(desktop.CheckDataDir, desktop.LabelDataDir, "/data"),
		dataDirRes: "/data",
		authData:   passResult(desktop.CheckAuthDataReadable, desktop.LabelAuthDataReadable, "Auth data readable at /data."),
		probe:      passResult(desktop.CheckStandardProbe, desktop.LabelStandardProbe, "Relate API reachable at http://localhost:44222."),
		probeRes:   desktopclient.ProbeResult{Port: 44222, Origin: "http://localhost:44222"},
		infoApp:    passResult(desktop.CheckInfoApp, desktop.LabelInfoApp, "version=2.0.0 appPath=/Applications/Neo4j Desktop 2.app dataPath=/data"),
		authProbe:  passResult(desktop.CheckAuthenticated, desktop.LabelAuthenticated, "Authenticated call against Desktop relate API succeeded."),
	})

	got := desktop.RunChecksForTest(context.Background(), cfg, 0)

	want := []string{
		desktop.CheckInstallPresent,
		desktop.CheckStandardProbe,
		desktop.CheckInfoApp,
		desktop.CheckDataDir,
		desktop.CheckAuthDataReadable,
		desktop.CheckAuthenticated,
	}
	if len(got.Checks) != len(want) {
		t.Fatalf("expected %d checks, got %d", len(want), len(got.Checks))
	}
	for i, w := range want {
		if got.Checks[i].Name != w {
			t.Errorf("check[%d] = %q, want %q", i, got.Checks[i].Name, w)
		}
	}
}
