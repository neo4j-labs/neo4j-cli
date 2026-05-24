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

// versionLineRe matches the frontmatter `version:` line in an installed
// SKILL.md. Tolerates leading whitespace and arbitrary trailing whitespace
// after the value.
var versionLineRe = regexp.MustCompile(`(?m)^[ \t]*version:[ \t]*([^\r\n]*?)[ \t]*$`)

// versionPlaceholder mirrors render.versionPlaceholder. Duplicated here to
// avoid an import cycle (render imports nothing from skill).
const versionPlaceholder = "{{VERSION}}"

// AgentInstall describes the per-agent state surfaced by List / Check.
type AgentInstall struct {
	Agent            *Agent
	Detected         bool   // DetectDir exists on disk
	Installed        bool   // SKILL.md present in this agent's skills dir
	InstalledVersion string // value of the `version:` frontmatter line, "" if not installed or unparseable
	Details          []AgentInstallDetail
}

// AgentInstallDetail describes a concrete backing install used by meta-agents.
type AgentInstallDetail struct {
	Agent            string
	DisplayName      string
	SkillsPath       string
	Installed        bool
	InstalledVersion string
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

// Install copies `bundle` into each target agent's skills directory under
// `<skillsDir>/<skillName>/`. The {{VERSION}} placeholder in SKILL.md is
// substituted with `version` before writing; references are copied
// verbatim.
//
// agentFilter semantics:
//   - "" — install to every detected agent. Returns ErrNoAgentsDetected
//     when none are detected.
//   - non-empty — case-insensitive lookup in AGENTS. Unknown returns
//     ErrUnknownAgent; known-but-undetected returns ErrAgentNotDetected.
//
// Returns the list of agents the bundle was written to (in catalog order).
func Install(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*Agent, error) {
	if skillName == "" {
		return nil, errors.New("skill: empty skill name")
	}
	if bundle == nil {
		return nil, errors.New("skill: nil bundle FS")
	}

	targets, err := resolveTargets(filesystem, agentFilter)
	if err != nil {
		return nil, err
	}

	for _, a := range targets {
		skillTargets, ok := a.SkillTargets()
		if !ok {
			return nil, fmt.Errorf("skill: cannot resolve skills path for %s", a.Name)
		}
		for _, target := range skillTargets {
			dst := filepath.Join(target.SkillsRoot, skillName)

			// Clean any prior install so removed reference files don't linger.
			if rerr := RemoveDir(filesystem, dst); rerr != nil {
				return nil, fmt.Errorf("skill: cleaning %s: %w", dst, rerr)
			}
			if cerr := copyBundleWithVersion(filesystem, dst, bundle, version); cerr != nil {
				return nil, fmt.Errorf("skill: writing %s: %w", dst, cerr)
			}
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
		skillTargets, ok := a.SkillTargets()
		if !ok {
			continue
		}
		for _, target := range skillTargets {
			dst := filepath.Join(target.SkillsRoot, skillName)
			if rerr := RemoveDir(filesystem, dst); rerr != nil {
				return nil, fmt.Errorf("skill: removing %s: %w", dst, rerr)
			}
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

		row.Detected = agentDetected(filesystem, &AGENTS[i])

		targets, ok := AGENTS[i].SkillTargets()
		if ok {
			for _, target := range targets {
				skillFile := filepath.Join(target.SkillsRoot, skillName, "SKILL.md")
				detail := AgentInstallDetail{
					Agent:       target.AgentName,
					DisplayName: target.DisplayName,
					SkillsPath:  filepath.Join(target.SkillsRoot, skillName),
				}
				if exists, _ := afero.Exists(filesystem, skillFile); exists {
					row.Installed = true
					detail.Installed = true
					if data, err := afero.ReadFile(filesystem, skillFile); err == nil {
						detail.InstalledVersion = parseVersion(data)
					}
				}
				row.Details = append(row.Details, detail)
			}
			if len(row.Details) > 1 && !row.Detected {
				row.Installed = false
				row.InstalledVersion = ""
			} else if len(row.Details) == 1 {
				row.InstalledVersion = row.Details[0].InstalledVersion
			} else if len(row.Details) > 1 {
				row.InstalledVersion = summarizeInstallDetails(row.Details)
				if !row.Installed {
					row.InstalledVersion = ""
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
		status, rowDrift := checkStatus(r, currentVersion)
		drift = drift || rowDrift
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
	if !agentDetected(filesystem, a) {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotDetected, a.Name)
	}
	return []*Agent{a}, nil
}

func summarizeInstallDetails(details []AgentInstallDetail) string {
	parts := make([]string, 0, len(details))
	for _, detail := range details {
		version := "missing"
		if detail.Installed {
			version = detail.InstalledVersion
			if version == "" {
				version = "unknown-version"
			}
		}
		parts = append(parts, detail.Agent+":"+version)
	}
	return strings.Join(parts, ",")
}

func checkStatus(row AgentInstall, currentVersion string) (string, bool) {
	if len(row.Details) > 1 {
		partial := false
		unknown := false
		drift := false
		for _, detail := range row.Details {
			if !detail.Installed {
				partial = true
				continue
			}
			switch {
			case detail.InstalledVersion == "":
				unknown = true
			case detail.InstalledVersion != currentVersion:
				drift = true
			}
		}
		switch {
		case partial:
			return "partial", true
		case unknown:
			return "unknown-version", true
		case drift:
			return "drift", true
		default:
			return "ok", false
		}
	}

	switch {
	case row.InstalledVersion == "":
		return "unknown-version", true
	case row.InstalledVersion != currentVersion:
		return "drift", true
	default:
		return "ok", false
	}
}

// copyBundleWithVersion is CopyBundle plus a single-file substitution: the
// SKILL.md file at the bundle root has its {{VERSION}} placeholder replaced
// with the runtime version before writing. Other files (references/*.md)
// are copied verbatim.
func copyBundleWithVersion(dst afero.Fs, dstDir string, bundle fs.FS, version string) error {
	if dstDir == "" {
		return errors.New("skill: empty destination dir")
	}
	return fs.WalkDir(bundle, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, rerr := fs.ReadFile(bundle, p)
		if rerr != nil {
			return rerr
		}
		if p == "SKILL.md" {
			data = substituteVersion(data, version)
		}
		destPath := filepath.Join(dstDir, filepath.FromSlash(p))
		if mkerr := dst.MkdirAll(filepath.Dir(destPath), 0755); mkerr != nil {
			return mkerr
		}
		return afero.WriteFile(dst, destPath, data, 0600)
	})
}

// substituteVersion replaces every occurrence of {{VERSION}} in `data`
// with `version`. Used only on SKILL.md. An empty version is a no-op so
// tests can verify the placeholder survives when no version is supplied.
func substituteVersion(data []byte, version string) []byte {
	if version == "" {
		return data
	}
	return []byte(strings.ReplaceAll(string(data), versionPlaceholder, version))
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
