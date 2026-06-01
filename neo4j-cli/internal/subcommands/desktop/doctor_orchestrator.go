// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop

import (
	"context"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/afero"
)

// Orchestrator per-check seams. data_dir and standard_probe also produce an
// intermediate value chained into downstream checks, so they wrap their
// underlying helpers to expose it.
var (
	checkInstallPresentFn = checkInstallPresent

	checkMDNSFn = checkMDNS

	checkDataDirFn = func(ctx context.Context, fs afero.Fs, probe desktopclient.ProbeResult) (CheckResult, string) {
		dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
		result := checkDataDir(ctx, fs, probe)
		if err != nil {
			return result, ""
		}
		return result, dataDir
	}

	checkAuthDataReadableFn = checkAuthDataReadable

	checkStandardProbeFn = func(ctx context.Context, pinned int) (CheckResult, desktopclient.ProbeResult) {
		probe, err := desktopclient.ProbePort(ctx, pinned)
		result := checkStandardProbe(ctx, pinned)
		if err != nil {
			return result, desktopclient.ProbeResult{}
		}
		return result, probe
	}

	checkInfoAppFn = checkInfoApp

	buildAuthClientFn = func(fs afero.Fs, dataDir string, probe desktopclient.ProbeResult) (*desktopclient.Client, error) {
		salt, err := desktopclient.LoadSalt(fs, dataDir)
		if err != nil {
			return nil, err
		}
		return desktopclient.NewClient(probe, salt)
	}

	checkAuthenticatedProbeFn = checkAuthenticatedProbe
)

// SetCheckInstallPresentFnForTest overrides the install_present check.
func SetCheckInstallPresentFnForTest(fn func() CheckResult) func() {
	prev := checkInstallPresentFn
	checkInstallPresentFn = fn
	return func() { checkInstallPresentFn = prev }
}

// SetCheckDataDirFnForTest overrides the data_dir check and its dataDir extraction.
func SetCheckDataDirFnForTest(fn func(context.Context, afero.Fs, desktopclient.ProbeResult) (CheckResult, string)) func() {
	prev := checkDataDirFn
	checkDataDirFn = fn
	return func() { checkDataDirFn = prev }
}

// SetCheckAuthDataReadableFnForTest overrides the auth_data_readable check.
func SetCheckAuthDataReadableFnForTest(fn func(afero.Fs, string) CheckResult) func() {
	prev := checkAuthDataReadableFn
	checkAuthDataReadableFn = fn
	return func() { checkAuthDataReadableFn = prev }
}

// SetCheckStandardProbeFnForTest overrides the standard_probe check and its ProbeResult extraction.
func SetCheckStandardProbeFnForTest(fn func(context.Context, int) (CheckResult, desktopclient.ProbeResult)) func() {
	prev := checkStandardProbeFn
	checkStandardProbeFn = fn
	return func() { checkStandardProbeFn = prev }
}

// SetCheckMDNSFnForTest overrides the mdns_discovery check.
func SetCheckMDNSFnForTest(fn func(context.Context) CheckResult) func() {
	prev := checkMDNSFn
	checkMDNSFn = fn
	return func() { checkMDNSFn = prev }
}

// SetCheckInfoAppFnForTest overrides the info_app check.
func SetCheckInfoAppFnForTest(fn func(context.Context, desktopclient.ProbeResult) CheckResult) func() {
	prev := checkInfoAppFn
	checkInfoAppFn = fn
	return func() { checkInfoAppFn = prev }
}

// SetBuildAuthClientFnForTest overrides the authenticated-client builder.
func SetBuildAuthClientFnForTest(fn func(afero.Fs, string, desktopclient.ProbeResult) (*desktopclient.Client, error)) func() {
	prev := buildAuthClientFn
	buildAuthClientFn = fn
	return func() { buildAuthClientFn = prev }
}

// SetCheckAuthenticatedProbeFnForTest overrides the authenticated_probe check.
func SetCheckAuthenticatedProbeFnForTest(fn func(context.Context, *desktopclient.Client) CheckResult) func() {
	prev := checkAuthenticatedProbeFn
	checkAuthenticatedProbeFn = fn
	return func() { checkAuthenticatedProbeFn = prev }
}

// skipResult builds the `skip` row emitted in place of a dependent check when
// an upstream check FAILed.
func skipResult(name, label, dependency string) CheckResult {
	return CheckResult{
		Name:   name,
		Label:  label,
		Status: StatusSkip,
		Detail: fmt.Sprintf("(depends on %s)", dependency),
	}
}

// RunChecksForTest is the exported wrapper around runChecks.
func RunChecksForTest(ctx context.Context, cfg *clicfg.Config, pinnedPort int) DoctorReport {
	return runChecks(ctx, cfg, pinnedPort)
}

// runChecks executes the seven health checks in order:
//
//	install_present → mdns_discovery → standard_probe → info_app → data_dir
//	  → auth_data_readable → authenticated_probe
//
// mdns_discovery and info_app are purely diagnostic and never gate downstream
// checks (data-dir resolution falls back on its own); a miss/failure renders
// as INFO rather than FAIL. standard_probe runs regardless of the mDNS result.
func runChecks(ctx context.Context, cfg *clicfg.Config, pinnedPort int) DoctorReport {
	fs := cfg.Aura.Fs()

	checks := make([]CheckResult, 0, 7)

	installR := checkInstallPresentFn()
	checks = append(checks, installR)

	// mDNS discovery is purely diagnostic — it reports whether Desktop
	// advertises over mDNS but never gates the standard-port probe below.
	mdnsR := checkMDNSFn(ctx)
	checks = append(checks, mdnsR)

	// Probe runs before anything that touches the relate API so its origin
	// can be threaded into /info/app and the data-dir check.
	probeR, probe := checkStandardProbeFn(ctx, pinnedPort)
	checks = append(checks, probeR)

	// info_app SKIPs (rather than INFOs) when the probe FAILed so the
	// dependency relationship stays visible in the report.
	var infoAppR CheckResult
	if probeR.Status != StatusPass {
		infoAppR = skipResult(CheckInfoApp, LabelInfoApp, CheckStandardProbe)
	} else {
		infoAppR = checkInfoAppFn(ctx, probe)
	}
	checks = append(checks, infoAppR)

	dataDirR, dataDir := checkDataDirFn(ctx, fs, probe)
	checks = append(checks, dataDirR)

	authDataR := checkAuthDataReadableFn(fs, dataDir)
	checks = append(checks, authDataR)

	var authProbeR CheckResult
	switch {
	case probeR.Status != StatusPass:
		authProbeR = skipResult(CheckAuthenticated, LabelAuthenticated, CheckStandardProbe)
	case authDataR.Status != StatusPass:
		// Without the salt the JWT signing key is invalid, so the auth probe
		// would emit a misleading 401-style FAIL. Skip instead.
		authProbeR = skipResult(CheckAuthenticated, LabelAuthenticated, CheckAuthDataReadable)
	default:
		client, err := buildAuthClientFn(fs, dataDir, probe)
		if err != nil {
			authProbeR = CheckResult{
				Name:   CheckAuthenticated,
				Label:  LabelAuthenticated,
				Status: StatusFail,
				Detail: fmt.Sprintf("Could not build authenticated client: %s.", err.Error()),
			}
		} else {
			authProbeR = checkAuthenticatedProbeFn(ctx, client)
		}
	}
	checks = append(checks, authProbeR)

	return DoctorReport{
		Checks:  checks,
		Summary: buildSummary(checks, pinnedPort, probe),
	}
}

// buildSummary derives the one-line verdict from the per-check results.
// Reachable is true iff both standard_probe and authenticated_probe PASSed.
// Port is omitted when the probe never succeeded so consumers can
// distinguish that from "Desktop on port X". NextStep carries the first
// FAILing check's Hint.
func buildSummary(checks []CheckResult, pinnedPort int, probe desktopclient.ProbeResult) DoctorSummary {
	var (
		probeR     CheckResult
		authProbeR CheckResult
	)
	for _, c := range checks {
		switch c.Name {
		case CheckStandardProbe:
			probeR = c
		case CheckAuthenticated:
			authProbeR = c
		}
	}

	summary := DoctorSummary{
		Reachable:         probeR.Status == StatusPass && authProbeR.Status == StatusPass,
		StandardPortRange: standardPortRange(pinnedPort),
	}

	if probeR.Status == StatusPass {
		p := probe.Port
		summary.Port = &p
	}

	for _, c := range checks {
		if c.Status == StatusFail && c.Hint != "" {
			summary.NextStep = c.Hint
			break
		}
	}

	return summary
}

// standardPortRange reports whether the active port budget is the documented
// 44222..44232 scan range.
func standardPortRange(pinned int) bool {
	if pinned == 0 {
		return true
	}
	return pinned >= desktopclient.ProbePortStart && pinned <= desktopclient.ProbePortEnd
}
