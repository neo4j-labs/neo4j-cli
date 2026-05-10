// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update — install_method.go owns "is the running neo4j-cli binary
// installed via a package manager?" detection plus the rich passthrough hint
// shown when it is.
//
// The detection function resolves the absolute path of the currently running
// binary (following symlinks via filepath.EvalSymlinks so that, e.g., a
// `~/.local/bin/neo4j-cli` symlink into a pipx venv is detected as pipx, not
// as a plain binary). The path is then matched against a small allowlist of
// known package-manager prefixes:
//
//   - homebrew: /opt/homebrew/, /usr/local/Cellar/, /home/linuxbrew/.linuxbrew/
//   - npm:      any path containing /node_modules/@neo4j-labs/cli/
//   - pipx:     ~/.local/pipx/venvs/neo4j-cli/, ~/.local/share/pipx/, or a
//     symlink at ~/.local/bin/neo4j-cli pointing into one of the above.
//   - uv tool:  ~/.local/share/uv/tools/neo4j-cli/
//
// On match, RunE prints the rich hint built by Hint and exits 0 without
// downloading anything. Pass --force to bypass.
//
// Test seams:
//   - executableFn replaces os.Executable so tests can drop a fake binary at
//     a temp path and exercise the resolution logic without touching the host
//     filesystem layout.
//   - homeDirFn replaces os.UserHomeDir so the pipx/uv prefix expansion is
//     hermetic across runners (and matches AGENTS.md "use t.Setenv(\"HOME\")"
//     pattern when callers prefer the env knob).
package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallMethod is the discovered package-manager channel for the running
// binary, or "binary" when nothing matched. The string values are stable
// because they appear verbatim in JSON output (REQ-F-018).
type InstallMethod string

const (
	InstallMethodBinary   InstallMethod = "binary"
	InstallMethodHomebrew InstallMethod = "homebrew"
	InstallMethodNpm      InstallMethod = "npm"
	InstallMethodPipx     InstallMethod = "pipx"
	InstallMethodUv       InstallMethod = "uv"
)

// installScriptCmd is the channel-agnostic install command surfaced by the
// rich passthrough hint per REQ-F-010a. Centralised so tests can assert it
// once.
const installScriptCmd = "curl -sSfL https://neo4j.sh/install.sh | bash"

// Test seams. Production fills with the real impls; tests swap via the
// withExecutable / withHomeDir helpers in install_method_test.go.
var (
	// executableFn shadows os.Executable so tests can point detection at a
	// fake path on disk (typically under t.TempDir()).
	executableFn = os.Executable
	// homeDirFn shadows os.UserHomeDir. The function falls back to $HOME
	// just like the stdlib so AGENTS.md `t.Setenv("HOME", ...)` works
	// without further wiring.
	homeDirFn = os.UserHomeDir
)

// Detect inspects the running binary's resolved path and returns the
// InstallMethod plus the resolved absolute path used for the match.
//
// The resolved path is also returned so callers can render it in
// `update check` / JSON output (REQ-F-018 channel + binary-path debug hooks).
// When os.Executable / EvalSymlinks fails, Detect returns InstallMethodBinary
// and a non-nil error — RunE treats this as "proceed with self-update"
// because the failure mode is "we can't tell, so don't block".
func Detect() (InstallMethod, string, error) {
	exe, err := executableFn()
	if err != nil {
		return InstallMethodBinary, "", fmt.Errorf("locate running binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// Symlink resolution can fail on Windows for a number of benign
		// reasons (e.g. the parent dir is on a junction the calling user
		// can't traverse). Fall back to the unresolved path; the package
		// manager prefix match below still works for a Homebrew-style
		// `/opt/homebrew/bin/neo4j-cli` direct install.
		resolved = exe
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		// Highly unlikely after EvalSymlinks succeeded; treat as "unknown".
		return InstallMethodBinary, resolved, nil
	}

	method := classify(abs)
	return method, abs, nil
}

// classify maps a resolved absolute path to an InstallMethod. Path matching
// uses OS-native separators (per AGENTS.md "Windows CI gotchas") — callers
// are expected to have already run the path through filepath.Abs /
// EvalSymlinks. No further normalisation happens here.
func classify(absPath string) InstallMethod {
	// Build prefix lists with OS-native separators. filepath.FromSlash
	// converts the portable forward-slash spellings below into the right
	// shape on Windows (where Homebrew/pipx aren't really a thing, but the
	// helper has to compile + behave deterministically across runners).
	homebrewPrefixes := []string{
		filepath.FromSlash("/opt/homebrew/"),
		filepath.FromSlash("/usr/local/Cellar/"),
		filepath.FromSlash("/home/linuxbrew/.linuxbrew/"),
	}
	for _, p := range homebrewPrefixes {
		if strings.HasPrefix(absPath, p) {
			return InstallMethodHomebrew
		}
	}

	// npm: any path containing /node_modules/@neo4j-labs/cli/. The
	// substring is portable (npm uses forward slashes inside node_modules
	// even on Windows) but we still pass it through FromSlash so any future
	// cross-platform layout difference is handled centrally.
	if strings.Contains(absPath, filepath.FromSlash("/node_modules/@neo4j-labs/cli/")) {
		return InstallMethodNpm
	}

	// pipx + uv prefixes are anchored under $HOME. Resolve once; if HOME
	// isn't set we simply skip those checks — there's nothing to match
	// against. This mirrors the AGENTS.md "$HOME unset means hermetic"
	// stance.
	home, err := homeDirFn()
	if err != nil || home == "" {
		return InstallMethodBinary
	}

	pipxPrefixes := []string{
		filepath.Join(home, ".local", "pipx", "venvs", "neo4j-cli") + string(filepath.Separator),
		filepath.Join(home, ".local", "share", "pipx") + string(filepath.Separator),
	}
	for _, p := range pipxPrefixes {
		if strings.HasPrefix(absPath, p) {
			return InstallMethodPipx
		}
	}

	uvPrefix := filepath.Join(home, ".local", "share", "uv", "tools", "neo4j-cli") + string(filepath.Separator)
	if strings.HasPrefix(absPath, uvPrefix) {
		return InstallMethodUv
	}

	return InstallMethodBinary
}

// Hint returns the user-facing passthrough message for a given install method.
// The shape follows REQ-F-010a: a top-line preamble, the channel-correct
// upgrade command, then a "switch to a self-managed install" block carrying
// the install-script command and the OPTIONAL uninstall command.
//
// Returns the empty string for InstallMethodBinary — there is no passthrough
// hint when self-update is the right answer.
func Hint(method InstallMethod) string {
	switch method {
	case InstallMethodHomebrew:
		return formatHint(
			"Installed via Homebrew.",
			"brew upgrade neo4j-cli",
			"brew uninstall neo4j-cli",
		)
	case InstallMethodNpm:
		// REQ-F-009 calls out pnpm / yarn equivalents alongside npm because
		// any of the three could have produced the @neo4j-labs/cli node_modules
		// layout. List all three so the user picks the one matching their
		// project tooling.
		return formatHintMulti(
			"Installed via npm-compatible package manager (global).",
			[]string{
				"npm i -g @neo4j-labs/cli@latest",
				"pnpm add -g @neo4j-labs/cli@latest",
				"yarn global add @neo4j-labs/cli@latest",
			},
			"npm uninstall -g @neo4j-labs/cli",
		)
	case InstallMethodPipx:
		return formatHint(
			"Installed via pipx.",
			"pipx upgrade neo4j-cli",
			"pipx uninstall neo4j-cli",
		)
	case InstallMethodUv:
		return formatHint(
			"Installed via uv tool.",
			"uv tool upgrade neo4j-cli",
			"uv tool uninstall neo4j-cli",
		)
	default:
		return ""
	}
}

// formatHint composes the three-block passthrough message documented in
// REQ-F-010a. The uninstall line is annotated with the PATH-order rationale
// so users understand why it's optional.
func formatHint(channel, upgradeCmd, uninstallCmd string) string {
	return formatHintMulti(channel, []string{upgradeCmd}, uninstallCmd)
}

// formatHintMulti is the npm-flavoured variant of formatHint that lists more
// than one upgrade command (pnpm / yarn alongside npm per REQ-F-009). The
// preamble switches to "run one of:" when more than one command is offered.
func formatHintMulti(channel string, upgradeCmds []string, uninstallCmd string) string {
	preamble := "To upgrade in place, run:"
	if len(upgradeCmds) > 1 {
		preamble = "To upgrade in place, run one of:"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", channel, preamble)
	for _, cmd := range upgradeCmds {
		fmt.Fprintf(&b, "  %s\n", cmd)
	}
	b.WriteString("\n")
	b.WriteString("To switch to a self-managed install (so 'neo4j-cli update' works directly):\n")
	fmt.Fprintf(&b, "  %s   # optional — only needed if PATH still resolves the package-manager binary\n", uninstallCmd)
	fmt.Fprintf(&b, "  %s\n", installScriptCmd)
	return b.String()
}
