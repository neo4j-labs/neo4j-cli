// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop — install.go owns the `desktop install` orchestration:
// already-installed detection → manifest fetch → artifact pick → SHA-512
// verify → per-OS install action delegation.
//
// The Linux installer filename uses `_lin` (not `_linux`) so Go's
// filename-based GOOS constraint doesn't restrict it to GOOS=linux — the
// seam-mocked cross-platform tests need to build it on every host.
//
// macOS picks `.dmg` (NOT `.zip` — that's the Squirrel.Mac auto-update
// vehicle), Linux picks `.AppImage`, Windows picks the NSIS `.exe`.
// SHA-512 is base64-encoded in the manifest (NOT hex).
package desktop

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

// installManifestURLs are the electron-builder publish-feed manifests.
var installManifestURLs = map[string]string{
	"darwin":  "https://dist.neo4j.org/neo4j-desktop-2/mac/latest-mac.yml",
	"linux":   "https://dist.neo4j.org/neo4j-desktop-2/linux/latest-linux.yml",
	"windows": "https://dist.neo4j.org/neo4j-desktop-2/win/latest.yml",
}

const installHTTPTimeout = 10 * time.Minute

// installMaxArtifactBytes caps the artifact download against an adversarial
// server. Desktop 2 DMG/AppImage/EXE are ~250-300 MiB; 600 MiB is headroom.
const installMaxArtifactBytes = 600 << 20

// installMaxManifestBytes caps the manifest download. Real manifests are a
// few KiB; a multi-megabyte "manifest" is a hard error rather than parsed.
const installMaxManifestBytes = 64 << 10

var (
	installGoosFn = func() string { return runtime.GOOS }

	installGoarchFn = func() string { return runtime.GOARCH }

	// installHTTPDoFn refuses any redirect that crosses scheme or host. The
	// same-host pin in resolveArtifactURL guards URLs the CLI computes; this
	// policy closes the gap where a compromised `dist.neo4j.org` could 30x to
	// an attacker host (http.Client transparently follows redirects).
	installHTTPDoFn = func(req *http.Request) (*http.Response, error) {
		client := &http.Client{
			Timeout: installHTTPTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("desktop install: too many redirects")
				}
				origin := via[0].URL
				if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
					return fmt.Errorf("desktop install: refusing redirect from %s://%s to %s://%s",
						origin.Scheme, origin.Host, req.URL.Scheme, req.URL.Host)
				}
				return nil
			},
		}
		return client.Do(req)
	}

	installTempDirFn = os.TempDir

	runInstallActionFn = realRunInstallAction
)

// SetInstallGoosFnForTest overrides the GOOS sentinel.
func SetInstallGoosFnForTest(fn func() string) func() {
	prev := installGoosFn
	installGoosFn = fn
	return func() { installGoosFn = prev }
}

// SetInstallGoarchFnForTest overrides the GOARCH sentinel.
func SetInstallGoarchFnForTest(fn func() string) func() {
	prev := installGoarchFn
	installGoarchFn = fn
	return func() { installGoarchFn = prev }
}

// SetInstallHTTPDoFnForTest overrides the HTTP transport.
func SetInstallHTTPDoFnForTest(fn func(*http.Request) (*http.Response, error)) func() {
	prev := installHTTPDoFn
	installHTTPDoFn = fn
	return func() { installHTTPDoFn = prev }
}

// SetInstallTempDirFnForTest overrides the temp-dir resolver.
func SetInstallTempDirFnForTest(fn func() string) func() {
	prev := installTempDirFn
	installTempDirFn = fn
	return func() { installTempDirFn = prev }
}

// SetRunInstallActionFnForTest overrides the per-OS install action seam.
func SetRunInstallActionFnForTest(fn func(context.Context, InstallPlan) error) func() {
	prev := runInstallActionFn
	runInstallActionFn = fn
	return func() { runInstallActionFn = prev }
}

// SetInstallManifestURLsForTest overrides the per-OS manifest URL map.
func SetInstallManifestURLsForTest(urls map[string]string) func() {
	prev := installManifestURLs
	installManifestURLs = urls
	return func() { installManifestURLs = prev }
}

// installManifest mirrors the subset of electron-builder's YAML manifest the
// CLI consumes. Unused fields are omitted so a schema change in them cannot
// break parsing.
type installManifest struct {
	Version     string                 `yaml:"version"`
	Files       []installManifestEntry `yaml:"files"`
	Path        string                 `yaml:"path"`
	SHA512      string                 `yaml:"sha512"`
	ReleaseDate string                 `yaml:"releaseDate"`
}

// installManifestEntry is one row in the manifest's `files[]`. SHA512 is
// base64-encoded raw 64-byte digest (NOT hex). URL is relative to the
// manifest URL's directory.
type installManifestEntry struct {
	URL    string `yaml:"url"`
	SHA512 string `yaml:"sha512"`
	Size   int64  `yaml:"size"`
}

// InstallPlan is the resolved per-OS install spec handed off to the per-OS
// install action.
type InstallPlan struct {
	ArtifactPath string
	TargetDir    string
	Version      string
	ManifestURL  string
	ArtifactURL  string
	Force        bool
}

// realRunInstallAction routes by `installGoosFn()` to the per-OS install action.
func realRunInstallAction(ctx context.Context, plan InstallPlan) error {
	switch installGoosFn() {
	case "darwin":
		return installMacOS(ctx, plan)
	case "linux":
		return installLinux(ctx, plan)
	case "windows":
		return installWindows(ctx, plan)
	default:
		return clierr.NewFatalError(
			"desktop install: unsupported OS %q (supported: darwin, linux, windows)",
			installGoosFn())
	}
}

func newInstallCmd(_ *clicfg.Config) *cobra.Command {
	var (
		force     bool
		targetDir string
		dryRun    bool
	)

	const (
		forceFlag     = "force"
		targetDirFlag = "target-dir"
		dryRunFlag    = "dry-run"
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Neo4j Desktop 2 from the public publish feed",
		Long: "Install Neo4j Desktop 2 on the local machine. " +
			"Already-installed detection (macOS `.app` + `Info.plist`, Linux AppImage glob under `~/Applications`, Windows `%LOCALAPPDATA%\\Programs\\neo4j-desktop`) runs BEFORE any network call: " +
			"on a hit the command prints `Neo4j Desktop 2 already installed at <path> (version <X>). Pass --force to re-install.` and exits 0 unless `--force` is supplied. " +
			"On a clean system the command fetches the per-OS electron-builder manifest from `dist.neo4j.org/neo4j-desktop-2/...`, picks the platform artifact (DMG / AppImage / NSIS .exe), downloads it to a tempfile, " +
			"verifies its base64-decoded SHA-512 against the manifest entry, and then dispatches to the per-OS install action. " +
			"The command does NOT prompt for license acceptance (REQ-F-021) and does NOT auto-launch Desktop on success (REQ-F-022) — a stderr next-step hint is printed instead. " +
			"Linux arm64 hard-errors with a deployment-center URL because no upstream arm64 build is published.",
		Example: `# Install Neo4j Desktop 2 using the per-OS default target dir
neo4j-cli desktop install --rw

# Re-install over an existing on-disk Desktop install
neo4j-cli desktop install --force --rw

# Resolve the manifest + artifact URL without downloading or installing
neo4j-cli desktop install --dry-run --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			goos := installGoosFn()
			goarch := installGoarchFn()

			// Already-installed detection BEFORE any HTTP I/O. `--force`
			// skips it so users can intentionally redownload.
			if !force {
				if hit, ok := detectInstalled(goos); ok {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(),
						"Neo4j Desktop 2 already installed at %s (version %s). Pass --force to re-install.\n",
						hit.Path, displayVersion(hit.Version))
					return nil
				}
			}

			// Linux arm64 has no upstream build. Hard-error before fetching
			// the manifest so users see the right hint even when network is down.
			if goos == "linux" && goarch == "arm64" {
				return clierr.NewFatalError(
					"Neo4j Desktop 2 does not publish an arm64 Linux build; " +
						"visit https://neo4j.com/deployment-center/?desktop-gdb")
			}

			manifestURL, ok := installManifestURLs[goos]
			if !ok {
				return clierr.NewFatalError(
					"desktop install: unsupported OS %q (supported: darwin, linux, windows)", goos)
			}

			manifest, err := fetchManifest(ctx, manifestURL)
			if err != nil {
				return err
			}

			entry, artifactURL, err := pickArtifact(manifest, manifestURL, goos)
			if err != nil {
				return err
			}

			if dryRun {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"Would install Neo4j Desktop 2 (version %s).\n  Manifest: %s\n  Artifact: %s\n",
					manifest.Version, manifestURL, artifactURL)
				return nil
			}

			defaultDir, err := defaultTargetDir(goos)
			if err != nil {
				return err
			}
			finalTargetDir := defaultDir
			if targetDir != "" {
				finalTargetDir = targetDir
			}

			tempPath, err := downloadAndVerify(ctx, artifactURL, entry, manifest.Version, goos)
			if err != nil {
				return err
			}

			plan := InstallPlan{
				ArtifactPath: tempPath,
				TargetDir:    finalTargetDir,
				Version:      manifest.Version,
				ManifestURL:  manifestURL,
				ArtifactURL:  artifactURL,
				Force:        force,
			}

			// Reset the resolved-target-dir cell so a stale value from a
			// previous in-process run cannot leak into the breadcrumb.
			clearLastInstalledTargetDir()

			actionErr := runInstallActionFn(ctx, plan)
			// Always clean up the verified tempfile even on partial failure
			// so a fresh DMG/AppImage/EXE doesn't linger in /tmp.
			_ = os.Remove(tempPath)
			if actionErr != nil {
				return actionErr
			}

			// Per-OS actions may land in a fallback location (e.g. macOS
			// ~/Applications); surface the actual on-disk path.
			breadcrumbDir := finalTargetDir
			if actual := lastInstalledTargetDir; actual != "" {
				breadcrumbDir = actual
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Installed Neo4j Desktop 2 (version %s) at %s.\n", manifest.Version, breadcrumbDir)
			// Stderr next-step hint — do NOT auto-launch.
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
				"Run Neo4j Desktop 2 from your applications menu to start using it.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, forceFlag, false, "Re-install regardless of already-installed detection")
	cmd.Flags().StringVar(&targetDir, targetDirFlag, "", "Override the per-OS default install directory")
	cmd.Flags().BoolVar(&dryRun, dryRunFlag, false, "Resolve the manifest + artifact URL and print them; skip download and install")
	return cmd
}

// fetchManifest downloads the per-OS YAML manifest, capped at
// installMaxManifestBytes against adversarial servers.
func fetchManifest(ctx context.Context, manifestURL string) (installManifest, error) {
	body, err := installDownloadCapped(ctx, manifestURL, installMaxManifestBytes)
	if err != nil {
		return installManifest{}, clierr.NewFatalError(
			"desktop install: failed to fetch manifest %s: %s", manifestURL, err.Error())
	}
	var manifest installManifest
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		return installManifest{}, clierr.NewFatalError(
			"desktop install: failed to parse manifest %s: %s", manifestURL, err.Error())
	}
	if manifest.Version == "" || len(manifest.Files) == 0 {
		return installManifest{}, clierr.NewFatalError(
			"desktop install: manifest %s is missing required fields (version, files)", manifestURL)
	}
	return manifest, nil
}

// pickArtifact picks the platform artifact from `files[]`. macOS picks
// `.dmg` (NOT `.zip` — that's the Squirrel.Mac auto-update vehicle),
// Linux picks `.AppImage`, Windows picks the NSIS `.exe`.
func pickArtifact(manifest installManifest, manifestURL, goos string) (installManifestEntry, string, error) {
	var extWant []string
	switch goos {
	case "darwin":
		extWant = []string{".dmg"}
	case "linux":
		extWant = []string{".appimage"}
	case "windows":
		extWant = []string{".exe"}
	default:
		return installManifestEntry{}, "", clierr.NewFatalError(
			"desktop install: unsupported OS %q", goos)
	}

	for _, f := range manifest.Files {
		name := strings.ToLower(path.Base(f.URL))
		for _, ext := range extWant {
			if strings.HasSuffix(name, ext) {
				resolved, err := resolveArtifactURL(manifestURL, f.URL)
				if err != nil {
					return installManifestEntry{}, "", clierr.NewFatalError(
						"desktop install: failed to resolve artifact URL %q: %s", f.URL, err.Error())
				}
				return f, resolved, nil
			}
		}
	}

	return installManifestEntry{}, "", clierr.NewFatalError(
		"desktop install: no %s artifact found in manifest %s", strings.Join(extWant, "/"), manifestURL)
}

// resolveArtifactURL resolves a manifest's relative `files[].url` against
// the manifest URL's directory. Absolute URLs are honoured as-is.
func resolveArtifactURL(manifestURL, relative string) (string, error) {
	base, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("parse manifest url: %w", err)
	}
	rel, err := url.Parse(relative)
	if err != nil {
		return "", fmt.Errorf("parse relative url: %w", err)
	}
	// Defence-in-depth: a compromised manifest could point `files[].url` at
	// an arbitrary origin (the SHA-512 check passes because the attacker
	// controls both the body and the hash field in the same manifest). Pin
	// any URL that names a host to the same host + scheme as the manifest
	// so the hardcoded `installManifestURLs` trust root is the only origin
	// we'll download from.
	//
	// A protocol-relative URL like `//attacker.example/evil.dmg` parses
	// with Scheme="" + Host="attacker.example"; `url.IsAbs()` returns false
	// for it, so this check has to look at Host independent of IsAbs.
	if rel.Host != "" && rel.Host != base.Host {
		return "", fmt.Errorf("manifest artifact url %q is on a different host than the manifest (%q vs %q)", rel.String(), rel.Host, base.Host)
	}
	if rel.Scheme != "" && rel.Scheme != base.Scheme {
		return "", fmt.Errorf("manifest artifact url %q downgrades scheme from manifest (%q -> %q)", rel.String(), base.Scheme, rel.Scheme)
	}
	if rel.IsAbs() {
		return rel.String(), nil
	}
	// url.ResolveReference treats the path as a file, not a directory, so
	// we have to strip the trailing filename off the base before resolving.
	base.Path = path.Dir(base.Path) + "/"
	return base.ResolveReference(rel).String(), nil
}

// downloadAndVerify streams the artifact to a tempfile, hashes the on-disk
// bytes with crypto/sha512, and compares against the base64-decoded manifest
// digest. Mismatch removes the tempfile and returns a fatal error. On
// success the caller owns the returned tempfile path.
func downloadAndVerify(ctx context.Context, artifactURL string, entry installManifestEntry, version, goos string) (string, error) {
	expected, err := base64.StdEncoding.DecodeString(strings.TrimSpace(entry.SHA512))
	if err != nil {
		return "", clierr.NewFatalError(
			"desktop install: manifest sha512 for artifact %s is not valid base64: %s", artifactURL, err.Error())
	}
	if len(expected) != sha512.Size {
		return "", clierr.NewFatalError(
			"desktop install: manifest sha512 for artifact %s has wrong byte length (%d, expected %d)",
			artifactURL, len(expected), sha512.Size)
	}

	tempDir := installTempDirFn()
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", clierr.NewFatalError("desktop install: prepare temp dir %s: %s", tempDir, err.Error())
	}

	// os.CreateTemp's random suffix avoids the predictable-path
	// DOS/symlink-TOCTOU class (a peer could wedge a symlink between
	// os.Remove and O_EXCL). `version` comes from the downloaded manifest;
	// sanitise so a crafted value (`2.0/../../usr/local/bin/evil`) can't
	// escape tempDir via filepath.Join `..` normalisation.
	safeVersion := filepath.Base(strings.NewReplacer("/", "_", "\\", "_").Replace(version))
	pattern := fmt.Sprintf("neo4j-desktop-2-%s-%s-*%s", goos, safeVersion, artifactExt(goos))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return "", clierr.NewFatalError("desktop install: build artifact request: %s", err.Error())
	}
	req.Header.Set("User-Agent", "neo4j-cli-desktop-install")

	resp, err := installHTTPDoFn(req)
	if err != nil {
		return "", clierr.NewFatalError("desktop install: download %s: %s", artifactURL, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", clierr.NewFatalError(
			"desktop install: download %s returned status %d", artifactURL, resp.StatusCode)
	}

	f, err := os.CreateTemp(tempDir, pattern)
	if err != nil {
		return "", clierr.NewFatalError("desktop install: create temp artifact in %s: %s", tempDir, err.Error())
	}
	tempPath := f.Name()

	hasher := sha512.New()
	capped := io.LimitReader(resp.Body, installMaxArtifactBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(f, hasher), capped)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return "", clierr.NewFatalError("desktop install: write artifact %s: %s", tempPath, copyErr.Error())
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return "", clierr.NewFatalError("desktop install: close artifact %s: %s", tempPath, closeErr.Error())
	}
	if written > installMaxArtifactBytes {
		_ = os.Remove(tempPath)
		return "", clierr.NewFatalError(
			"desktop install: artifact %s exceeds cap of %d bytes", artifactURL, installMaxArtifactBytes)
	}

	gotSum := hasher.Sum(nil)
	if !equalBytes(gotSum, expected) {
		_ = os.Remove(tempPath)
		return "", clierr.NewFatalError(
			"desktop install: sha512 mismatch for %s (manifest expected %s, got %s)",
			artifactURL,
			base64.StdEncoding.EncodeToString(expected),
			base64.StdEncoding.EncodeToString(gotSum))
	}

	return tempPath, nil
}

// installDownloadCapped GETs rawURL and returns the body bytes capped at
// maxBytes. The artifact is streamed straight to disk so its full body
// never lives in memory.
func installDownloadCapped(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "neo4j-cli-desktop-install")

	resp, err := installHTTPDoFn(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.CopyN(io.Discard, resp.Body, 1<<10)
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds cap of %d bytes", maxBytes)
	}
	return body, nil
}

// artifactExt returns the per-OS artifact extension used for the tempfile
// name; matches the suffixes pickArtifact looks for.
func artifactExt(goos string) string {
	switch goos {
	case "darwin":
		return ".dmg"
	case "linux":
		return ".AppImage"
	case "windows":
		return ".exe"
	default:
		return ""
	}
}

// equalBytes is a constant-time-ish bytes compare. Not cryptographically
// required here, but the stable shape avoids future short-circuit-compare
// temptations on prefix-matching bytes.
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// defaultTargetDir returns the per-OS default install directory before the
// `--target-dir` override is applied. Routes through the detection seams so
// one override drives both detection and orchestration's default picker.
func defaultTargetDir(goos string) (string, error) {
	switch goos {
	case "darwin":
		// Default `/Applications`; on EACCES the macOS install action falls
		// back to `~/Applications`. Orchestration picks the canonical default
		// without probing writability.
		return "/Applications", nil
	case "linux":
		home, err := detectHomeDirFn()
		if err != nil || home == "" {
			return "", clierr.NewFatalError(
				"desktop install: could not resolve home directory for default target")
		}
		return filepath.Join(home, "Applications"), nil
	case "windows":
		localAppData := detectLocalAppDataFn()
		if localAppData == "" {
			return "", clierr.NewFatalError(
				"desktop install: %%LOCALAPPDATA%% not set; pass --target-dir explicitly")
		}
		return filepath.Join(localAppData, "Programs", "neo4j-desktop"), nil
	default:
		return "", clierr.NewFatalError(
			"desktop install: unsupported OS %q (supported: darwin, linux, windows)", goos)
	}
}

// displayVersion renders a version string for the "already installed"
// breadcrumb, returning "unknown" when detection didn't recover a value.
func displayVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}
