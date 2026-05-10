// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update implements the `neo4j-cli update` self-update command.
//
// The command compares the running binary's baked-in version against the
// latest GitHub release at neo4j-labs/neo4j-cli and, when newer, downloads
// + atomically swaps the binary in place. When the running binary lives
// under a known package-manager prefix (Homebrew, npm-global, pipx, uv
// tool), the command refuses to overwrite and prints the channel-correct
// upgrade command instead.
//
// The package is intentionally split across files for testability:
//
//   - update.go        — cobra wiring + RunE flow that ties the modules
//     together. The flow follows REQ-F-002…REQ-F-016 in
//     prd-self-update-command.md.
//   - release.go       — GitHub release discovery + asset URL builder.
//   - install_method.go— is the running binary under a known package-manager
//     prefix? + the rich passthrough hint.
//   - swap.go          — download + verify + extract + atomic rename.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	commonskill "github.com/neo4j/cli/common/skill"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

// devVersion is the sentinel returned by `neo4j-cli --version` for a
// non-released local build (set by `make build` when ldflags don't bake in
// a real tag). REQ-F-002: the update flow short-circuits with a friendly
// message when this is the running version, never contacting GitHub.
const devVersion = "dev"

// Test seams. Production fills with real impls; tests swap via the
// withLatest / withGetByTag / withDetect / withSwap helpers in update_test.go.
//
// The seams are exposed as package-level vars (per AGENTS.md "Package-level
// test seams") so the RunE flow can be unit-tested without standing up a
// real httptest server, fake binary on disk, AND a fake archive simultaneously.
// The release / install_method / swap modules each have their own
// finer-grained seams (httpDoFn, executableFn, etc.) for module-level tests;
// the seams here let RunE assert the orchestration without re-testing the
// modules' internals.
var (
	latestFn   = Latest
	getByTagFn = GetByTag
	detectFn   = Detect
	swapFn     = Swap
	// listSkillsFn enumerates per-agent skill install state. Production
	// uses common/skill.List; tests swap to seed installed/uninstalled
	// agents without touching disk.
	listSkillsFn = commonskill.List
	// installSkillFn refreshes a single agent's bundle. Production uses
	// common/skill.Install with a one-agent filter; tests swap to assert
	// per-agent invocation order and simulate refresh failures.
	installSkillFn = commonskill.Install
)

// NewCmd returns the `update` cobra command. It is mounted as a top-level
// subcommand on the neo4j-cli tree alongside `aura`, `credential`, `config`,
// `query`, and `skill`.
//
// `bundle` and `skillName` are the embedded skill bundle and its on-disk
// install dir (e.g. "neo4j-cli"); after a successful in-place swap the
// command refreshes any installed agents' bundles so AI assistants pick up
// the new binary's surface without a manual `skill install` step.
func NewCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	var (
		preReleases bool
		version     string
		force       bool
	)

	const (
		preReleasesFlag = "pre-releases"
		versionFlag     = "version"
		forceFlag       = "force"
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Self-update the neo4j-cli binary",
		Long: "Self-update the neo4j-cli binary by downloading the latest GitHub release and atomically " +
			"swapping it in place. By default only stable semver tags are considered; pass `--pre-releases` " +
			"to opt into alpha/beta/rc tags. When the running binary lives under a known package-manager " +
			"prefix (Homebrew, npm-global, pipx, uv tool), the command refuses to overwrite and prints the " +
			"channel-correct upgrade command instead — pass `--force` to override. " +
			"After a successful swap, any installed agent skill bundles are refreshed automatically.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), cmd, cfg, runOpts{
				preReleases: preReleases,
				version:     version,
				force:       force,
				bundle:      bundle,
				skillName:   skillName,
			})
		},
	}

	cmd.Flags().BoolVar(&preReleases, preReleasesFlag, false, "Include alpha/beta/rc tags when looking up the latest release")
	cmd.Flags().StringVar(&version, versionFlag, "", "Update to the named release tag instead of the latest (must be a valid semver tag, e.g. v0.1.0)")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Bypass the package-manager-managed-binary check and proceed with the in-place swap")

	cmd.AddCommand(newCheckCmd(cfg, bundle, skillName))

	return cmd
}

// runOpts groups the four user-facing flags plus the embedded skill bundle
// (so post-swap skill refresh has access to it) so runUpdate's signature
// stays small as the implementation grows. Internal-only — not part of any
// exported API.
type runOpts struct {
	preReleases bool
	check       bool
	version     string
	force       bool
	// bundle / skillName are nil/empty for unit tests that don't exercise
	// the post-swap skill-refresh path. Production wires both via NewCmd.
	bundle    fs.FS
	skillName string
}

// updateResult is the structured representation of an update-command run,
// used to render JSON output per REQ-F-018. The fields are populated as the
// flow progresses; partial results (e.g. install-method passthrough,
// stable-only filter miss) still produce a well-formed JSON document.
//
// Field order matches REQ-F-018: current, latest, updated, check, channel,
// install_method, then post-swap-only updated_skills + skill_install_suggested.
// The JSON path goes through printableUpdateResult below to pin that order
// via MarshalJSON.
type updateResult struct {
	current       string
	latest        string
	updated       bool
	check         bool
	channel       string
	installMethod string
	// updatedSkills lists agent names whose skill bundle was refreshed
	// after a successful swap. omitempty in JSON.
	updatedSkills []string
	// skillInstallSuggested is true when no agent was detected as having
	// the skill installed, so the user is hinted to run
	// `neo4j-cli skill install`. omitempty in JSON.
	skillInstallSuggested bool
}

// printableUpdateResult wraps updateResult and satisfies output.ResponseData
// (via AsArray) plus json.Marshaler. The custom MarshalJSON path emits the
// fields in the REQ-F-018 documented order so downstream scripts can rely on
// stable JSON regardless of map iteration randomness.
type printableUpdateResult struct {
	r updateResult
}

// AsArray returns a single-row slice for table rendering. Update output is
// document-shaped (not list-shaped), so we wrap the one row in a slice as
// required by the ResponseData interface. Post-swap-only fields
// (updated_skills, skill_install_suggested) are included only when set so
// table renders stay tight on the more common no-swap branches.
func (p printableUpdateResult) AsArray() []map[string]any {
	row := map[string]any{
		"current":        p.r.current,
		"latest":         p.r.latest,
		"updated":        p.r.updated,
		"check":          p.r.check,
		"channel":        p.r.channel,
		"install_method": p.r.installMethod,
	}
	if len(p.r.updatedSkills) > 0 {
		row["updated_skills"] = p.r.updatedSkills
	}
	if p.r.skillInstallSuggested {
		row["skill_install_suggested"] = true
	}
	return []map[string]any{row}
}

// MarshalJSON emits a single object (not an array) with the documented
// REQ-F-018 field order. PrintBodyMap's JSON path calls json.MarshalIndent
// which honours this; the table/toon paths use AsArray which wraps in a
// slice for grid rendering.
//
// The first six fields keep their REQ-F-018 order; updated_skills /
// skill_install_suggested follow with omitempty so the JSON shape stays
// stable for callers on the no-swap branches (passthrough hint, --check).
func (p printableUpdateResult) MarshalJSON() ([]byte, error) {
	doc := struct {
		Current               string   `json:"current"`
		Latest                string   `json:"latest"`
		Updated               bool     `json:"updated"`
		Check                 bool     `json:"check"`
		Channel               string   `json:"channel"`
		InstallMethod         string   `json:"install_method"`
		UpdatedSkills         []string `json:"updated_skills,omitempty"`
		SkillInstallSuggested bool     `json:"skill_install_suggested,omitempty"`
	}{
		Current:               p.r.current,
		Latest:                p.r.latest,
		Updated:               p.r.updated,
		Check:                 p.r.check,
		Channel:               p.r.channel,
		InstallMethod:         p.r.installMethod,
		UpdatedSkills:         p.r.updatedSkills,
		SkillInstallSuggested: p.r.skillInstallSuggested,
	}
	return json.Marshal(doc)
}

// printResult renders the structured result to cmd.OutOrStdout using the
// caller-selected output mode. Any explicit `--format` value among the
// structured set ("json", "table", "toon") routes through PrintBodyMap;
// the default ("default" or empty) falls through to the plain-text path.
//
// The default explicitly does NOT call ResolveOutput — it must stay plain-text
// even on a TTY, otherwise the running narrative ("Current version → Checking
// for updates → Successfully updated from X to Y") gets clobbered by an
// auto-detected table render.
//
// The plain-text branch is implemented inline rather than via PrintBodyMap
// because the reference output (REQ-F-017) is a fixed three-line shape, not
// a generic body-map.
func printResult(cmd *cobra.Command, cfg *clicfg.Config, r updateResult, plainText func()) {
	if isStructuredFormat(cfg.Global.Format()) {
		output.PrintBodyMap(cmd, cfg, printableUpdateResult{r: r}, fieldOrder(r))
		return
	}
	plainText()
}

// fieldOrder produces the column / key ordering passed to PrintBodyMap. The
// first six entries match REQ-F-018; updated_skills and
// skill_install_suggested are appended only when populated so table/toon
// renders stay tight on the no-swap branches (passthrough hint, --check).
func fieldOrder(r updateResult) []string {
	keys := []string{"current", "latest", "updated", "check", "channel", "install_method"}
	if len(r.updatedSkills) > 0 {
		keys = append(keys, "updated_skills")
	}
	if r.skillInstallSuggested {
		keys = append(keys, "skill_install_suggested")
	}
	return keys
}

// isStructuredFormat reports whether the caller asked for one of the explicit
// structured output modes (json/table/toon). The "default" / empty value is
// NOT structured — it stays plain-text so the running narrative survives a
// TTY (auto-detection in ResolveOutput would otherwise pick "table").
func isStructuredFormat(format string) bool {
	switch format {
	case "json", "table", "toon":
		return true
	}
	return false
}

// runUpdate is the orchestration entry point. It implements the REQ-F-002
// through REQ-F-016 ordering documented in the PRD:
//
//  1. Dev-build short-circuit (REQ-F-002).
//  2. Resolve target version (release lookup or --version <tag>).
//  3. Compare current vs target via semver.Compare (REQ-F-008).
//  4. Install-method detection + passthrough hint (REQ-F-009/010/010a),
//     unless --force.
//  5. `update check` branch (REQ-F-011): report and exit without downloading.
//  6. Download + verify + swap (REQ-F-012/013/014/015/016).
//
// The package-manager check is intentionally placed AFTER target resolution
// so the user gets a meaningful "you're on vX, vY is available — run brew
// upgrade neo4j-cli" message rather than a content-free "we won't update,
// run brew upgrade neo4j-cli".
func runUpdate(ctx context.Context, cmd *cobra.Command, cfg *clicfg.Config, opts runOpts) error {
	current := cfg.Version
	if current == devVersion || current == "" {
		// REQ-F-002: dev build — no network call. Plain-text only; the JSON
		// shape requires a `latest` field which we never resolve in this
		// branch, so emitting partial JSON here would be more confusing than
		// the friendly text.
		cmd.Println("running a dev build, nothing to update")
		return nil
	}

	// Validate --version up front so a bad value is rejected before we hit
	// the network. ValidateVersionTag rejects empty too — guard here.
	if opts.version != "" {
		if err := ValidateVersionTag(opts.version); err != nil {
			return clierr.NewUsageError("invalid --version: %v", err)
		}
	}

	// REQ-F-006: silence the cobra Usage block on RunE error AFTER flag
	// validation has run. Genuine flag misuse (`update --bogus`) still
	// surfaces the help via cobra's normal pre-RunE flag-parse path; from
	// here on, any error is a runtime failure (network, swap, sudo) and
	// dumping `--help` over the failure adds noise without helping the user.
	cmd.SilenceUsage = true

	// Resolve the target release.
	target, channel, err := resolveTarget(ctx, opts)
	if err != nil {
		// REQ-F-006: friendly hint when stable-only filter excludes everything.
		if errors.Is(err, ErrNoStableRelease) {
			cmd.Println("no stable release published yet — pass `--pre-releases` to track alpha/beta/rc tags.")
			return nil
		}
		if errors.Is(err, ErrTagNotFound) {
			return clierr.NewUsageError("release tag %q not found", opts.version)
		}
		return clierr.NewUpstreamError("look up release: %v", err)
	}

	// Compare current vs target. semver.Compare requires both to be valid;
	// app.Version is generated by the release pipeline so it is, but be
	// defensive — a malformed current value should produce a clear error
	// rather than a misleading downgrade refusal.
	if !semver.IsValid(current) {
		return clierr.NewFatalError("running binary version %q is not valid semver; cannot compare", current)
	}
	cmp := semver.Compare(current, target.TagName)

	// Initial result skeleton. Specific branches below fill in `updated`,
	// `check`, and `install_method` as appropriate.
	result := updateResult{
		current:       current,
		latest:        target.TagName,
		check:         opts.check,
		channel:       channel,
		installMethod: string(InstallMethodBinary),
	}

	// REQ-F-008: same version is "already up-to-date", current > target is
	// "downgrade" (rejected unless --version explicitly set).
	if cmp == 0 {
		printResult(cmd, cfg, result, func() {
			cmd.Printf("Already on %s. No update needed.\n", current)
		})
		return nil
	}
	if cmp > 0 && opts.version == "" {
		return clierr.NewUsageError(
			"running binary (%s) is newer than the latest %s release (%s); pass --version explicitly to downgrade",
			current, channel, target.TagName,
		)
	}

	// REQ-F-009/010: detect install method and bail out with the rich hint
	// unless the user passed --force. We do this AFTER discovering the
	// target so the user knows whether there's anything new before we tell
	// them to use brew/pipx/uv/npm.
	if !opts.force {
		method, _, detectErr := detectFn()
		// Treat detection error as "fall through to swap" — the function
		// already returned InstallMethodBinary in that case. We do NOT
		// surface the err to the user; install_method.go documents this.
		_ = detectErr
		if hint := Hint(method); hint != "" {
			result.installMethod = string(method)
			printResult(cmd, cfg, result, func() {
				cmd.Printf("%s already on %s; %s available.\n", method, current, target.TagName)
				cmd.Print(hint)
			})
			return nil
		}
	}

	// REQ-F-001/002: `update check` mode — report and exit 0 (no error)
	// regardless of whether a newer version exists. Finding a new version
	// is the success case for `check`; CI/scripts that want to branch on
	// drift compare `current != latest` from the JSON output. The plain-text
	// branch prints both the existing two-line "Current/Latest" header AND
	// the new "New version available" + install-command hint so users see
	// exactly what to run next.
	if opts.check {
		printResult(cmd, cfg, result, func() {
			cmd.Printf("Current version: %s\n", current)
			cmd.Printf("Latest %s version: %s\n", channel, target.TagName)
			// cmp < 0 — newer available.
			cmd.Printf("New version available: %s -> %s\n", current, target.TagName)
			cmd.Printf("Run `%s` to install.\n", buildUpdateCommand(opts))
		})
		return nil
	}

	// REQ-F-012/013/014/015/016: download → verify → extract → atomic
	// swap. Build the asset URLs, resolve the running binary's resolved
	// (post-EvalSymlinks) absolute path, hand off to Swap.
	urls, err := BuildAssetURLs(target.TagName)
	if err != nil {
		return clierr.NewFatalError("build asset URL: %v", err)
	}

	method, currentBinaryPath, _ := detectFn()
	if currentBinaryPath == "" {
		// detectFn returns the resolved exe path even when classification
		// is "binary"; an empty value here means the executable lookup
		// failed entirely. Surface a clear error rather than swap a
		// guessed path.
		return clierr.NewFatalError("could not locate running binary on disk")
	}
	// Reflect the detected channel in the JSON output even when --force
	// proceeded past the passthrough hint (so users can audit "I forced an
	// update on top of a homebrew binary"). Plain-text path is unaffected.
	result.installMethod = string(method)

	// Plain-text path emits the running narrative ("Current version", "Checking
	// for updates...") inline so the user sees progress before swap completes.
	// Any structured-output mode (json/table/toon) stays silent until success
	// and emits the full document at the end (REQ-F-018: scripts get a single
	// deterministic blob).
	if !isStructuredFormat(cfg.Global.Format()) {
		cmd.Printf("Current version: %s\n", current)
		cmd.Println("Checking for updates to latest version...")
	}

	if err := swapFn(ctx, urls, currentBinaryPath, cmd.ErrOrStderr()); err != nil {
		// REQ-F-014: surface friendly, actionable hints for the two
		// permission-class sentinels. Both turn into FatalError so the
		// exit code stays non-zero while the printed shape is stable.
		var sudoErr *errSudoUnavailable
		if errors.As(err, &sudoErr) {
			return clierr.NewFatalError(
				"cannot write to %s (permission denied).\nRe-run with sudo:\n\n    sudo %s",
				sudoErr.Dir(), buildReRunCommand(cmd, opts),
			)
		}
		var winErr *errPermissionWindows
		if errors.As(err, &winErr) {
			return clierr.NewFatalError(
				"cannot write to %s (permission denied).\nRe-run from an Administrator shell.",
				winErr.Dir(),
			)
		}
		return clierr.NewFatalError("update failed: %v", err)
	}

	result.updated = true

	// Post-swap: refresh installed agent skills so AI assistants pick up the
	// new binary's surface without a manual `skill install` step. Failures
	// are non-fatal — the binary update already succeeded; surface a stderr
	// warning and keep `updated: true`.
	refreshed, suggestInstall := refreshSkillBundles(cmd, cfg, opts.bundle, opts.skillName, target.TagName)
	result.updatedSkills = refreshed
	result.skillInstallSuggested = suggestInstall

	printResult(cmd, cfg, result, func() {
		cmd.Printf("Successfully updated from %s to %s\n", current, target.TagName)
		switch {
		case len(refreshed) > 0:
			cmd.Printf("Refreshed skill bundle for: %s\n", strings.Join(refreshed, ", "))
		case suggestInstall:
			cmd.Println("Tip: install the agent skill so AI assistants pick up the new commands — run `neo4j-cli skill install`.")
		}
	})
	return nil
}

// buildUpdateCommand reconstructs the install command suggested by the
// `update check` newer-available hint. Reads the active flags out of opts so
// the suggestion mirrors what the user passed to `check` — e.g. a check run
// with `--pre-releases` produces `neo4j-cli update --pre-releases`.
//
// Maintenance note: if a new install-time flag is added to runOpts, mirror it
// here so the hint stays accurate. `--force` is intentionally NOT included
// because `update check` does not register `--force` (the install-method
// passthrough is irrelevant when nothing is being installed).
func buildUpdateCommand(opts runOpts) string {
	parts := []string{"neo4j-cli update"}
	if opts.preReleases {
		parts = append(parts, "--pre-releases")
	}
	if opts.version != "" {
		parts = append(parts, "--version "+opts.version)
	}
	return strings.Join(parts, " ")
}

// buildReRunCommand reconstructs the FULL command the user originally typed —
// used inside the "Re-run with sudo:" hint surfaced from the `*errSudoUnavailable`
// branch of runUpdate. Uses cmd.CommandPath() so the hint reflects however the
// user invoked the binary (e.g. `/usr/local/bin/neo4j-cli update` or just
// `neo4j-cli update`).
//
// Unlike buildUpdateCommand, this DOES include `--force` because the failing
// invocation is the install path (`update`, not `update check`) where force is
// a valid flag and the user may have set it.
func buildReRunCommand(cmd *cobra.Command, opts runOpts) string {
	parts := []string{cmd.CommandPath()}
	if opts.preReleases {
		parts = append(parts, "--pre-releases")
	}
	if opts.version != "" {
		parts = append(parts, "--version "+opts.version)
	}
	if opts.force {
		parts = append(parts, "--force")
	}
	return strings.Join(parts, " ")
}

// refreshSkillBundles enumerates installed agents and re-runs Install for
// each so their bundle reflects the new binary version. Returns the list of
// refreshed agent names and a "suggest skill install" flag (true when no
// agent was detected as having the skill installed).
//
// All errors are non-fatal: a per-agent refresh failure emits a single stderr
// warning and the loop continues. A nil bundle (e.g. unit tests that don't
// thread one through NewCmd) skips the entire refresh path silently.
//
// The pkg-mgr passthrough and `update check` branches don't reach this
// function because no swap occurred — the call site is gated on a successful
// swap.
func refreshSkillBundles(cmd *cobra.Command, cfg *clicfg.Config, bundle fs.FS, skillName, version string) ([]string, bool) {
	if bundle == nil || skillName == "" {
		return nil, false
	}
	rows, err := listSkillsFn(cfg.Aura.Fs(), skillName)
	if err != nil {
		// Listing failed (e.g. afero.Fs error) — non-fatal. Don't suggest
		// install either since we couldn't tell.
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not enumerate installed skills (%s); skipping skill refresh.\n", err) //nolint:errcheck // warning to stderr; write errors are not actionable
		return nil, false
	}
	var installed []*commonskill.Agent
	for i := range rows {
		if rows[i].Installed {
			installed = append(installed, rows[i].Agent)
		}
	}
	if len(installed) == 0 {
		return nil, true
	}

	refreshed := make([]string, 0, len(installed))
	for _, a := range installed {
		if _, rerr := installSkillFn(cfg.Aura.Fs(), bundle, skillName, version, a.Name); rerr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to refresh skill bundle for %s (%s); continuing.\n", a.Name, rerr) //nolint:errcheck // warning to stderr; write errors are not actionable
			continue
		}
		refreshed = append(refreshed, a.Name)
	}
	return refreshed, false
}

// resolveTarget discovers the release the user wants to land on and the
// channel ("stable" / "pre-release") that produced the match. The channel
// label feeds the JSON output (REQ-F-018) and the user-facing messages.
func resolveTarget(ctx context.Context, opts runOpts) (*Release, string, error) {
	if opts.version != "" {
		r, err := getByTagFn(ctx, opts.version)
		if err != nil {
			return nil, "", err
		}
		channel := "stable"
		if semver.Prerelease(r.TagName) != "" {
			channel = "pre-release"
		}
		return r, channel, nil
	}

	r, err := latestFn(ctx, opts.preReleases)
	if err != nil {
		return nil, "", err
	}
	channel := "stable"
	if opts.preReleases && semver.Prerelease(r.TagName) != "" {
		channel = "pre-release"
	}
	return r, channel, nil
}
