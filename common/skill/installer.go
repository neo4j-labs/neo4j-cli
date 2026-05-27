// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/afero"
)

// ErrNoAgentsDetected is returned by Install when no agentFilter is supplied
// and no supported agent is detected on the host filesystem.
var ErrNoAgentsDetected = errors.New("skill: no supported agents detected")

// ErrUnknownAgent is returned by Install / Remove when agentFilter does not
// match any entry in the AGENTS catalog (case-insensitive).
var ErrUnknownAgent = errors.New("skill: unknown agent")

// ErrAgentNotDetected is returned by Install when agentFilter matches a
// known agent but its DetectDir is missing on the host filesystem.
var ErrAgentNotDetected = errors.New("skill: agent not detected on host")

// Source describes the on-disk bundle that Install will copy into each
// target agent. FS is rooted at the bundle (`SKILL.md` + `references/*` at
// the top level); Version is injected into the SKILL.md frontmatter
// `version:` line. An empty Version leaves frontmatter unchanged so unit
// tests can assert the upstream value is preserved.
type Source struct {
	FS      fs.FS
	Version string
}

// versionLineRe matches the frontmatter `version:` line in an installed
// SKILL.md. Tolerates leading whitespace and arbitrary trailing whitespace
// after the value.
var versionLineRe = regexp.MustCompile(`(?m)^[ \t]*version:[ \t]*([^\r\n]*?)[ \t]*$`)

// frontmatterRe matches the leading YAML frontmatter block of a SKILL.md
// body. Captures the inner body so injection can replace or append the
// `version:` line within it.
var frontmatterRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---(\r?\n|\z)`)

// AgentInstall describes the per-agent state surfaced by List / Check.
type AgentInstall struct {
	Agent            *Agent
	Detected         bool   // DetectDir exists on disk
	Installed        bool   // SKILL.md present in this agent's skills dir
	InstalledVersion string // value of the `version:` frontmatter line, "" if not installed or unparseable
}

// CheckRow is a per-agent row produced by Check. Status is "ok", "drift",
// or "unknown-version". Check returns rows only for installed agents — an
// uninstalled agent is silently omitted.
type CheckRow struct {
	Agent            *Agent
	InstalledVersion string
	CurrentVersion   string
	Status           string
}

// Install copies `src.FS` into each target agent's skills directory under
// `<skillsDir>/<skillName>/`. The SKILL.md frontmatter `version:` line is
// rewritten (or inserted) to `src.Version`; references are copied verbatim.
//
// agentFilter semantics:
//   - "" — install to every detected agent. Returns ErrNoAgentsDetected
//     when none are detected.
//   - non-empty — case-insensitive lookup in AGENTS. Unknown returns
//     ErrUnknownAgent; known-but-undetected returns ErrAgentNotDetected.
//
// Returns the list of agents the bundle was written to (in catalog order).
func Install(filesystem afero.Fs, src Source, skillName, agentFilter string) ([]*Agent, error) {
	if skillName == "" {
		return nil, errors.New("skill: empty skill name")
	}
	if src.FS == nil {
		return nil, errors.New("skill: nil bundle FS")
	}

	targets, err := resolveTargets(filesystem, agentFilter)
	if err != nil {
		return nil, err
	}

	for _, a := range targets {
		skillsRoot, ok := a.SkillsPath()
		if !ok {
			return nil, fmt.Errorf("skill: cannot resolve skills path for %s", a.Name)
		}
		dst := filepath.Join(skillsRoot, skillName)

		// Clean any prior install so removed reference files don't linger.
		if rerr := RemoveDir(filesystem, dst); rerr != nil {
			return nil, fmt.Errorf("skill: cleaning %s: %w", dst, rerr)
		}
		if cerr := copyBundleWithVersion(filesystem, dst, src); cerr != nil {
			return nil, fmt.Errorf("skill: writing %s: %w", dst, cerr)
		}
	}
	return targets, nil
}

// Remove deletes the installed bundle from each target agent's skills
// directory. Idempotent: missing install dirs return nil.
//
// agentFilter semantics:
//   - "" — remove from every detected agent (no error if none).
//   - non-empty — case-insensitive lookup; unknown returns ErrUnknownAgent.
//
// Removing from an undetected-but-known agent is a no-op (the install dir
// can't exist if the agent dir doesn't).
func Remove(filesystem afero.Fs, skillName, agentFilter string) ([]*Agent, error) {
	if skillName == "" {
		return nil, errors.New("skill: empty skill name")
	}

	var targets []*Agent
	if agentFilter == "" {
		targets = DetectAgents(filesystem)
	} else {
		a := FindAgent(agentFilter)
		if a == nil {
			return nil, fmt.Errorf("%w: %q", ErrUnknownAgent, agentFilter)
		}
		targets = []*Agent{a}
	}

	for _, a := range targets {
		skillsRoot, ok := a.SkillsPath()
		if !ok {
			continue
		}
		dst := filepath.Join(skillsRoot, skillName)
		if rerr := RemoveDir(filesystem, dst); rerr != nil {
			return nil, fmt.Errorf("skill: removing %s: %w", dst, rerr)
		}
	}
	return targets, nil
}

// List returns one AgentInstall per agent in the AGENTS catalog. Detected
// reflects DetectDir presence; Installed reflects SKILL.md presence under
// `<skillsDir>/<skillName>/`. InstalledVersion is the parsed frontmatter
// value (empty string when not installed or unparseable).
func List(filesystem afero.Fs, skillName string) ([]AgentInstall, error) {
	if skillName == "" {
		return nil, errors.New("skill: empty skill name")
	}

	out := make([]AgentInstall, 0, len(AGENTS))
	for i := range AGENTS {
		row := AgentInstall{Agent: &AGENTS[i]}

		dp, ok := AGENTS[i].DetectPath()
		if ok {
			exists, _ := afero.DirExists(filesystem, dp)
			row.Detected = exists
		}

		sp, ok := AGENTS[i].SkillsPath()
		if ok {
			skillFile := filepath.Join(sp, skillName, "SKILL.md")
			if exists, _ := afero.Exists(filesystem, skillFile); exists {
				row.Installed = true
				if data, err := afero.ReadFile(filesystem, skillFile); err == nil {
					row.InstalledVersion = parseVersion(data)
				}
			}
		}

		out = append(out, row)
	}
	return out, nil
}

// Check parses each installed SKILL.md frontmatter `version:` and compares
// to currentVersion. Returns one row per *installed* agent and a drift
// bool that is true when at least one row's Status != "ok".
//
// Status values: "ok" (matches), "drift" (mismatch), "unknown-version"
// (frontmatter missing/unparseable).
func Check(filesystem afero.Fs, skillName, currentVersion string) ([]CheckRow, bool, error) {
	rows, err := List(filesystem, skillName)
	if err != nil {
		return nil, false, err
	}

	var out []CheckRow
	drift := false
	for _, r := range rows {
		if !r.Installed {
			continue
		}
		status := "ok"
		switch {
		case r.InstalledVersion == "":
			status = "unknown-version"
			drift = true
		case r.InstalledVersion != currentVersion:
			status = "drift"
			drift = true
		}
		out = append(out, CheckRow{
			Agent:            r.Agent,
			InstalledVersion: r.InstalledVersion,
			CurrentVersion:   currentVersion,
			Status:           status,
		})
	}
	return out, drift, nil
}

// resolveTargets is the install-time agent filter. Mirrors Remove's logic
// but with stricter semantics: an unknown filter or undetected single
// target is an error (Remove tolerates both).
func resolveTargets(filesystem afero.Fs, agentFilter string) ([]*Agent, error) {
	if agentFilter == "" {
		targets := DetectAgents(filesystem)
		if len(targets) == 0 {
			return nil, ErrNoAgentsDetected
		}
		return targets, nil
	}
	a := FindAgent(agentFilter)
	if a == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAgent, agentFilter)
	}
	dp, ok := a.DetectPath()
	if !ok {
		return nil, fmt.Errorf("%w: %s (cannot resolve HOME)", ErrAgentNotDetected, a.Name)
	}
	exists, _ := afero.DirExists(filesystem, dp)
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotDetected, a.Name)
	}
	return []*Agent{a}, nil
}

// copyBundleWithVersion copies src.FS into dstDir, rewriting the SKILL.md
// frontmatter `version:` line to src.Version (or inserting one when
// upstream has none). References (references/*.md) are copied verbatim.
func copyBundleWithVersion(dst afero.Fs, dstDir string, src Source) error {
	if dstDir == "" {
		return errors.New("skill: empty destination dir")
	}
	return fs.WalkDir(src.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, rerr := fs.ReadFile(src.FS, p)
		if rerr != nil {
			return rerr
		}
		if p == "SKILL.md" {
			data = injectVersion(data, src.Version)
		}
		destPath := filepath.Join(dstDir, filepath.FromSlash(p))
		if mkerr := dst.MkdirAll(filepath.Dir(destPath), 0755); mkerr != nil {
			return mkerr
		}
		return afero.WriteFile(dst, destPath, data, 0600)
	})
}

// injectVersion rewrites the SKILL.md frontmatter `version:` line to the
// supplied value, or inserts one immediately before the closing `---`
// fence when absent. An empty `version` is a no-op so tests can verify
// upstream content survives when no version is supplied. If `data` has no
// frontmatter block, it is returned unchanged.
func injectVersion(data []byte, version string) []byte {
	if version == "" {
		return data
	}
	m := frontmatterRe.FindSubmatchIndex(data)
	if m == nil {
		return data
	}
	innerStart, innerEnd := m[2], m[3]
	inner := string(data[innerStart:innerEnd])

	newLine := "version: " + version
	var newInner string
	if versionLineRe.MatchString(inner) {
		newInner = versionLineRe.ReplaceAllString(inner, newLine)
	} else {
		trimmed := strings.TrimRight(inner, "\r\n")
		if trimmed == "" {
			newInner = newLine
		} else {
			newInner = trimmed + "\n" + newLine
		}
	}

	var out strings.Builder
	out.Grow(len(data) + len(newLine))
	out.Write(data[:innerStart])
	out.WriteString(newInner)
	out.Write(data[innerEnd:])
	return []byte(out.String())
}

// parseVersion extracts the frontmatter `version:` value from a SKILL.md
// body. Returns "" when the line is missing or empty.
func parseVersion(data []byte) string {
	m := versionLineRe.FindSubmatch(data)
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}
