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

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
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
)

// NewCmd returns the `update` cobra command. It is mounted as a top-level
// subcommand on the neo4j-cli tree alongside `aura`, `credential`, `config`,
// `query`, and `skill`.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		preReleases bool
		check       bool
		version     string
		force       bool
	)

	const (
		preReleasesFlag = "pre-releases"
		checkFlag       = "check"
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
			"channel-correct upgrade command instead — pass `--force` to override.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), cmd, cfg, runOpts{
				preReleases: preReleases,
				check:       check,
				version:     version,
				force:       force,
			})
		},
	}

	cmd.Flags().BoolVar(&preReleases, preReleasesFlag, false, "Include alpha/beta/rc tags when looking up the latest release")
	cmd.Flags().BoolVar(&check, checkFlag, false, "Report whether a newer version is available without downloading or swapping")
	cmd.Flags().StringVar(&version, versionFlag, "", "Update to the named release tag instead of the latest (must be a valid semver tag, e.g. v0.1.0)")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Bypass the package-manager-managed-binary check and proceed with the in-place swap")

	return cmd
}

// runOpts groups the four user-facing flags so runUpdate's signature stays
// small as the implementation grows. Internal-only — not part of any
// exported API.
type runOpts struct {
	preReleases bool
	check       bool
	version     string
	force       bool
}

// updateResult is the structured representation of an update-command run,
// used to render JSON output per REQ-F-018. The fields are populated as the
// flow progresses; partial results (e.g. install-method passthrough,
// stable-only filter miss) still produce a well-formed JSON document.
//
// Field order matches REQ-F-018: current, latest, updated, check, channel,
// install_method. The JSON path goes through printableUpdateResult below to
// pin that order via MarshalJSON.
type updateResult struct {
	current       string
	latest        string
	updated       bool
	check         bool
	channel       string
	installMethod string
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
// required by the ResponseData interface.
func (p printableUpdateResult) AsArray() []map[string]any {
	return []map[string]any{{
		"current":        p.r.current,
		"latest":         p.r.latest,
		"updated":        p.r.updated,
		"check":          p.r.check,
		"channel":        p.r.channel,
		"install_method": p.r.installMethod,
	}}
}

// MarshalJSON emits a single object (not an array) with the documented
// REQ-F-018 field order. PrintBodyMap's JSON path calls json.MarshalIndent
// which honours this; the table/toon paths use AsArray which wraps in a
// slice for grid rendering.
func (p printableUpdateResult) MarshalJSON() ([]byte, error) {
	// json.RawMessage assembled in REQ-F-018 order. Using an ordered slice of
	// (key, value) pairs would also work; a tiny anonymous struct is the
	// idiomatic Go form.
	doc := struct {
		Current       string `json:"current"`
		Latest        string `json:"latest"`
		Updated       bool   `json:"updated"`
		Check         bool   `json:"check"`
		Channel       string `json:"channel"`
		InstallMethod string `json:"install_method"`
	}{
		Current:       p.r.current,
		Latest:        p.r.latest,
		Updated:       p.r.updated,
		Check:         p.r.check,
		Channel:       p.r.channel,
		InstallMethod: p.r.installMethod,
	}
	return json.Marshal(doc)
}

// printResult renders the structured result to cmd.OutOrStdout using the
// caller-selected output mode. Plain-text is the default; JSON kicks in when
// the user passed `--format json` (or set `format: json` in the global
// config). A "table" format request also routes to JSON because update is a
// single-document command — there's no meaningful tabular layout.
//
// The plain-text branch is implemented inline rather than via PrintBodyMap
// because the reference output (REQ-F-017) is a fixed three-line shape, not
// a generic body-map.
func printResult(cmd *cobra.Command, cfg *clicfg.Config, r updateResult, plainText func()) {
	if cfg.Global.Format() == "json" {
		output.PrintBodyMap(cmd, cfg, printableUpdateResult{r: r}, []string{"current", "latest", "updated", "check", "channel", "install_method"})
		return
	}
	plainText()
}

// runUpdate is the orchestration entry point. It implements the REQ-F-002
// through REQ-F-016 ordering documented in the PRD:
//
//  1. Dev-build short-circuit (REQ-F-002).
//  2. Resolve target version (release lookup or --version <tag>).
//  3. Compare current vs target via semver.Compare (REQ-F-008).
//  4. Install-method detection + passthrough hint (REQ-F-009/010/010a),
//     unless --force.
//  5. --check branch (REQ-F-011): report and exit without downloading.
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

	// REQ-F-011: --check mode — report and exit. exit 1 (non-nil error)
	// when newer is available so CI/scripts can branch on it; exit 0 when
	// up-to-date (handled above by the cmp == 0 fast-path).
	if opts.check {
		printResult(cmd, cfg, result, func() {
			cmd.Printf("Current version: %s\n", current)
			cmd.Printf("Latest %s version: %s\n", channel, target.TagName)
		})
		// cmp < 0 — newer available.
		return clierr.NewUsageError("a newer version is available: %s -> %s", current, target.TagName)
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
	// JSON path stays silent until success and emits the full document at the
	// end (REQ-F-018: scripts get a single deterministic blob).
	jsonMode := cfg.Global.Format() == "json"
	if !jsonMode {
		cmd.Printf("Current version: %s\n", current)
		cmd.Println("Checking for updates to latest version...")
	}

	if err := swapFn(ctx, urls, currentBinaryPath); err != nil {
		return clierr.NewFatalError("update failed: %v", err)
	}

	result.updated = true
	printResult(cmd, cfg, result, func() {
		cmd.Printf("Successfully updated from %s to %s\n", current, target.TagName)
	})
	return nil
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
