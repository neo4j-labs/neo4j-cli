// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"io/fs"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/skill/catalog"
)

const (
	sourceEmbedded = "embedded"
	sourceCatalog  = "catalog"

	statusInstalled      = "installed"
	statusNotInstalled   = "not-installed"
	statusDrift          = "drift"
	statusUnknownVersion = "unknown-version"
	statusPartial        = "partial"
	statusOk             = "ok"
)

// catalogSummary is the per-skill aggregated view consumed by the catalog
// section renderer. Status follows the worst-wins fold (see
// aggregateCatalog).
type catalogSummary struct {
	Skill            string
	AvailableVersion string
	Status           string
	InstalledCount   int
	InstalledAgents  []string
}

// InventoryRow is the shared per-(skill × agent) record that powers both
// `skill list` (renders all rows) and `skill check` (filters to installed
// rows and gates exit on drift/unknown-version).
type InventoryRow struct {
	Skill            string
	Source           string
	Agent            *Agent
	Detected         bool
	Installed        bool
	InstalledVersion string
	AvailableVersion string
	Status           string
}

// BuildInventory enumerates one row per (skill × agent). Self-skill rows
// always come first (one per agent in AGENTS order); catalog rows follow
// in plugin.json order. Reserved catalog entries (collisions with self /
// binary-name) are skipped. When `cat` is nil only self-skill rows are
// returned.
func BuildInventory(filesystem afero.Fs, binaryName, binaryVersion string, cat *catalog.Catalog) []InventoryRow {
	out := make([]InventoryRow, 0, len(AGENTS))
	out = append(out, inventoryRowsForSkill(filesystem, binaryName, sourceEmbedded, binaryVersion)...)
	if cat == nil {
		return out
	}
	for _, entry := range cat.Skills {
		if catalog.IsReserved(entry.Name, binaryName) {
			continue
		}
		out = append(out, inventoryRowsForSkill(filesystem, entry.Name, sourceCatalog, catalogSkillVersion(filesystem, cat, entry.Name, binaryName))...)
	}
	return out
}

// catalogSkillVersion reads the available version from a catalog skill's
// cached remote SKILL.md — sourced per skill, not from the catalog-wide
// plugin.json. Any lookup or read failure resolves to "" so the row falls
// into the unknown-version path.
func catalogSkillVersion(filesystem afero.Fs, cat *catalog.Catalog, name, binaryName string) string {
	_, sub, err := cat.Lookup(filesystem, name, binaryName)
	if err != nil {
		return ""
	}
	data, err := fs.ReadFile(sub, "SKILL.md")
	if err != nil {
		return ""
	}
	return parseVersion(data)
}

// inventoryRowsForSkill returns one row per agent for `skillName`. detected/
// installed/installed_version are read from `filesystem`; status is
// classified by statusFor against `availableVersion`.
func inventoryRowsForSkill(filesystem afero.Fs, skillName, source, availableVersion string) []InventoryRow {
	rows := make([]InventoryRow, 0, len(AGENTS))
	for i := range AGENTS {
		row := InventoryRow{
			Skill:            skillName,
			Source:           source,
			Agent:            &AGENTS[i],
			AvailableVersion: availableVersion,
		}
		row.Detected = agentDetected(filesystem, &AGENTS[i])
		row.Installed, row.InstalledVersion = readInstalledSkill(filesystem, &AGENTS[i], skillName)
		row.Status = statusFor(row.Installed, row.InstalledVersion, row.AvailableVersion)
		rows = append(rows, row)
	}
	return rows
}

// statusFor classifies an inventory row. `not-installed` wins over version
// checks because an absent install has no meaningful version to compare.
// `unknown-version` fires when the installed SKILL.md frontmatter lacks a
// parseable `version:` line — distinct from drift so users can see the
// difference between "wrong version" and "no version at all". For the
// `check` leaf, `installed` collapses to `ok` because check only sees
// installed rows (it filters non-installed rows out before rendering).
func statusFor(installed bool, installedVersion, availableVersion string) string {
	if !installed {
		return statusNotInstalled
	}
	if installedVersion == "" {
		return statusUnknownVersion
	}
	if installedVersion != availableVersion {
		return statusDrift
	}
	return statusInstalled
}

// boolStr renders bools as user-facing yes/no for table output. JSON and
// toon paths bypass this and emit real booleans via MarshalJSON.
func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// aggregateCatalog folds per-agent rows for a single catalog skill into one
// summary. Skill and AvailableVersion are taken from the first row (all
// rows for one skill share these). InstalledAgents preserves input order
// (which is AGENTS catalog order by BuildInventory's construction).
//
// Priority: drift > unknown-version > partial > installed > not-installed.
func aggregateCatalog(rows []InventoryRow) catalogSummary {
	if len(rows) == 0 {
		return catalogSummary{Status: statusNotInstalled}
	}
	summary := catalogSummary{
		Skill:            rows[0].Skill,
		AvailableVersion: rows[0].AvailableVersion,
	}
	hasDrift := false
	hasUnknown := false
	for _, r := range rows {
		if !r.Installed {
			continue
		}
		summary.InstalledCount++
		summary.InstalledAgents = append(summary.InstalledAgents, r.Agent.Name)
		switch r.Status {
		case statusDrift:
			hasDrift = true
		case statusUnknownVersion:
			hasUnknown = true
		}
	}
	switch {
	case hasDrift:
		summary.Status = statusDrift
	case hasUnknown:
		summary.Status = statusUnknownVersion
	case summary.InstalledCount == 0:
		summary.Status = statusNotInstalled
	case summary.InstalledCount < len(rows):
		summary.Status = statusPartial
	default:
		summary.Status = statusInstalled
	}
	return summary
}
