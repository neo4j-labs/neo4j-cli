// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/afero"
)

// doctorGoosFn is a test seam so non-host OS install-detection branches are
// exercisable on a single host.
var doctorGoosFn = func() string { return runtime.GOOS }

// SetDoctorGoosFnForTest overrides the GOOS sentinel.
func SetDoctorGoosFnForTest(fn func() string) func() {
	prev := doctorGoosFn
	doctorGoosFn = fn
	return func() { doctorGoosFn = prev }
}

// CheckInstallPresentForTest is the exported wrapper around checkInstallPresent.
func CheckInstallPresentForTest() CheckResult { return checkInstallPresent() }

// CheckDataDirForTest is the exported wrapper around checkDataDir.
func CheckDataDirForTest(ctx context.Context, fs afero.Fs, probe desktopclient.ProbeResult) CheckResult {
	return checkDataDir(ctx, fs, probe)
}

// CheckAuthDataReadableForTest is the exported wrapper around checkAuthDataReadable.
func CheckAuthDataReadableForTest(fs afero.Fs, dataDir string) CheckResult {
	return checkAuthDataReadable(fs, dataDir)
}

// CheckStandardProbeForTest is the exported wrapper around checkStandardProbe.
func CheckStandardProbeForTest(ctx context.Context, port int) CheckResult {
	return checkStandardProbe(ctx, port)
}

// CheckAuthenticatedProbeForTest is the exported wrapper around checkAuthenticatedProbe.
func CheckAuthenticatedProbeForTest(ctx context.Context, client *desktopclient.Client) CheckResult {
	return checkAuthenticatedProbe(ctx, client)
}

// CheckInfoAppForTest is the exported wrapper around checkInfoApp.
func CheckInfoAppForTest(ctx context.Context, probe desktopclient.ProbeResult) CheckResult {
	return checkInfoApp(ctx, probe)
}

// CheckMDNSForTest is the exported wrapper around checkMDNS.
func CheckMDNSForTest(ctx context.Context) CheckResult { return checkMDNS(ctx) }

// checkInstallPresent reuses the install-detection helper that powers
// `desktop install`'s already-installed breadcrumb.
func checkInstallPresent() CheckResult {
	goos := doctorGoosFn()
	hit, ok := detectInstalled(goos)
	if !ok {
		return CheckResult{
			Name:   CheckInstallPresent,
			Label:  LabelInstallPresent,
			Status: StatusFail,
			Detail: fmt.Sprintf("Neo4j Desktop 2 install not found for %s.", goos),
			Hint:   "Run `neo4j-cli desktop install` or install Neo4j Desktop 2 from https://neo4j.com/download/.",
		}
	}
	detail := hit.Path
	if hit.Version != "" {
		detail = fmt.Sprintf("%s (version %s)", hit.Path, hit.Version)
	}
	return CheckResult{
		Name:   CheckInstallPresent,
		Label:  LabelInstallPresent,
		Status: StatusPass,
		Detail: detail,
	}
}

// checkMDNS browses for the Desktop relate API over mDNS / DNS-SD. Purely
// diagnostic and modelled on checkInfoApp: a responder yields PASS with the
// advertised port, and a miss (no responder, blocked multicast, timeout)
// renders as INFO so it never gates the standard-port probe or any later
// check. On macOS the INFO hint points at the Local Network permission and
// the --port escape hatch, since macOS can silently drop a bare CLI's
// multicast with no error.
func checkMDNS(ctx context.Context) CheckResult {
	probe, err := desktopclient.DiscoverViaMDNS(ctx)
	if err != nil {
		result := CheckResult{
			Name:   CheckMDNS,
			Label:  LabelMDNS,
			Status: StatusInfo,
			Detail: "No mDNS responder found; falling back to the standard port scan.",
		}
		if doctorGoosFn() == "darwin" {
			result.Hint = "If Desktop is running but undiscovered, grant the terminal app Local Network access in System Settings > Privacy & Security, or pass --port to skip discovery."
		}
		return result
	}
	return CheckResult{
		Name:   CheckMDNS,
		Label:  LabelMDNS,
		Status: StatusPass,
		Detail: fmt.Sprintf("Relate API advertised over mDNS at %s.", probe.Origin),
	}
}

// checkDataDir resolves the Desktop relate data directory using the same
// precedence Desktop itself walks: NEO4J_DESKTOP_DATA_PATH env override →
// /info/app dataPath → active env JSON → per-OS default. A successful resolve
// PASSes regardless of whether the directory exists on disk —
// checkAuthDataReadable touches the salt inside it. An empty `ProbeResult{}`
// short-circuits the /info/app step.
func checkDataDir(ctx context.Context, fs afero.Fs, probe desktopclient.ProbeResult) CheckResult {
	dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return CheckResult{
			Name:   CheckDataDir,
			Label:  LabelDataDir,
			Status: StatusFail,
			Detail: fmt.Sprintf("Could not resolve Desktop data directory: %s.", err.Error()),
			Hint:   "Ensure Neo4j Desktop 2 has run at least once so it can lay down its data directory.",
		}
	}
	return CheckResult{
		Name:   CheckDataDir,
		Label:  LabelDataDir,
		Status: StatusPass,
		Detail: dataDir,
	}
}

// checkAuthDataReadable verifies the on-disk auth payload. Label and detail
// stay generic — no "secret"/"key"/"JWT"/"salt" wording reaches users.
func checkAuthDataReadable(fs afero.Fs, dataDir string) CheckResult {
	salt, err := desktopclient.LoadSalt(fs, dataDir)
	if err != nil {
		return CheckResult{
			Name:   CheckAuthDataReadable,
			Label:  LabelAuthDataReadable,
			Status: StatusFail,
			Detail: fmt.Sprintf("Could not read Desktop auth data: %s.", err.Error()),
			Hint:   "Start Neo4j Desktop 2 once so it can write its auth data.",
		}
	}
	if strings.TrimSpace(salt) == "" {
		return CheckResult{
			Name:   CheckAuthDataReadable,
			Label:  LabelAuthDataReadable,
			Status: StatusFail,
			Detail: "Desktop auth data is empty.",
			Hint:   "Start Neo4j Desktop 2 once so it can write its auth data.",
		}
	}
	return CheckResult{
		Name:   CheckAuthDataReadable,
		Label:  LabelAuthDataReadable,
		Status: StatusPass,
		Detail: "Auth data readable at data dir.",
	}
}

// checkStandardProbe resolves the relate API exactly as the real commands do —
// mDNS first, then the 44222..44232 port-scan fallback — and reports the origin
// the CLI will authenticate against. It must use Discover (not bare ProbePort):
// a new Desktop signs its JWT with the 127.0.0.1 origin mDNS yields, so probing
// only the localhost port scan would thread the wrong origin into the
// authenticated probe and surface a false 401. `pinned != 0` confirms that port.
func checkStandardProbe(ctx context.Context, pinned int) CheckResult {
	probe, err := desktopclient.Discover(ctx, pinned)
	if err != nil {
		detail := "No relate server found via mDNS or the 44222..44232 port scan."
		hint := "Start Neo4j Desktop 2 from your applications menu, or pass --port if it's on a non-default port."
		if pinned != 0 {
			detail = fmt.Sprintf("No relate server answered on port %d.", pinned)
			hint = "Start Neo4j Desktop 2 from your applications menu, or pass a different --port."
		}
		return CheckResult{
			Name:   CheckStandardProbe,
			Label:  LabelStandardProbe,
			Status: StatusFail,
			Detail: detail,
			Hint:   hint,
		}
	}
	detail := fmt.Sprintf("Relate API reachable at %s.", probe.Origin)
	return CheckResult{
		Name:   CheckStandardProbe,
		Label:  LabelStandardProbe,
		Status: StatusPass,
		Detail: detail,
	}
}

// checkInfoApp asks Desktop's unauthenticated /info/app endpoint for its
// canonical version / appPath / dataPath. Purely diagnostic — any error
// (401 on older Desktop, 5xx, timeout, transport, decode, empty DataPath)
// renders as INFO with `unavailable (older Desktop)` so downstream checks
// keep running.
func checkInfoApp(ctx context.Context, probe desktopclient.ProbeResult) CheckResult {
	if probe.Origin == "" {
		// Defence in depth: the orchestrator already renders this row as
		// SKIP via skipResult; this guards direct callers.
		return CheckResult{
			Name:   CheckInfoApp,
			Label:  LabelInfoApp,
			Status: StatusInfo,
			Detail: "unavailable (older Desktop)",
		}
	}
	info, err := desktopclient.FetchAppInfo(ctx, probe)
	if err != nil || info.DataPath == "" {
		return CheckResult{
			Name:   CheckInfoApp,
			Label:  LabelInfoApp,
			Status: StatusInfo,
			Detail: "unavailable (older Desktop)",
		}
	}
	return CheckResult{
		Name:     CheckInfoApp,
		Label:    LabelInfoApp,
		Status:   StatusPass,
		Detail:   formatInfoAppDetail(info),
		Version:  info.Version,
		AppPath:  info.AppPath,
		DataPath: info.DataPath,
	}
}

func formatInfoAppDetail(info desktopclient.AppInfo) string {
	return fmt.Sprintf("version=%s appPath=%s dataPath=%s", info.Version, info.AppPath, info.DataPath)
}

// checkAuthenticatedProbe sends a cheap authenticated GET to `/dbmss`. A
// no-arg list route is used (rather than a per-id GET) to avoid the fastify
// route-validation 400 / handler-500 ambiguity that a synthetic id can
// produce. A 401 maps to clierr.AuthError (Code == 4).
func checkAuthenticatedProbe(ctx context.Context, client *desktopclient.Client) CheckResult {
	_, err := client.ListDbmss(ctx)
	if err == nil {
		return CheckResult{
			Name:   CheckAuthenticated,
			Label:  LabelAuthenticated,
			Status: StatusPass,
			Detail: "Authenticated call against Desktop relate API succeeded.",
		}
	}
	var cliErr *clierr.CLIError
	if errors.As(err, &cliErr) && cliErr.Code == 4 {
		return CheckResult{
			Name:   CheckAuthenticated,
			Label:  LabelAuthenticated,
			Status: StatusFail,
			Detail: "Desktop rejected the authenticated call (401).",
			Hint:   "Restart Neo4j Desktop 2 to regenerate its token state.",
		}
	}
	return CheckResult{
		Name:   CheckAuthenticated,
		Label:  LabelAuthenticated,
		Status: StatusFail,
		Detail: fmt.Sprintf("Authenticated call failed: %s.", err.Error()),
	}
}
