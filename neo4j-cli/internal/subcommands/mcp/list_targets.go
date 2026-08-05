// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clievents"
	commonoutput "github.com/neo4j/cli/common/output"
)

// Per-source timeouts for the discovery fan-out. Each source gets its own
// context deadline so a single slow or hanging source (mDNS probe for Desktop,
// network call for Aura) does not delay the others past its timeout.
const (
	dockerListTimeout     = 10 * time.Second
	desktopListTimeout    = 15 * time.Second
	credentialListTimeout = 5 * time.Second
	auraListTimeout       = 15 * time.Second
)

// sourceDef defines one discovery source to query.
type sourceDef struct {
	name    string
	command string
	timeout time.Duration
}

// discoverySources is the ordered list of sources.
var discoverySources = []sourceDef{
	{name: "docker", command: "docker list", timeout: dockerListTimeout},
	{name: "desktop", command: "desktop dbms list", timeout: desktopListTimeout},
	{name: "credential", command: "credential dbms list", timeout: credentialListTimeout},
	{name: "aura", command: "aura instance list", timeout: auraListTimeout},
}

// targetRow is a unified row in the discovery table. Every field is
// snake_case per the repo OUTPUT casing rule (CLI-127).
type targetRow struct {
	Source       string `json:"source"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Version      string `json:"version"`
	Connection   string `json:"connection"`
	Organization string `json:"organization"`
}

// HandleListTargets implements the neo4j_cli_list_targets tool. It fans out
// over the four discovery sources (docker, desktop, credential, aura) and
// returns a single merged table of reachable Neo4j targets. Individual source
// failures degrade to a noted omission rather than failing the tool — a laptop
// without Docker or without Aura credentials is the common case, not a fault.
func HandleListTargets(ctx context.Context, exec *Executor) (*mcpsdk.CallToolResult, error) {
	var allRows []targetRow
	var omissions []string

	for _, src := range discoverySources {
		srcCtx, cancel := context.WithTimeout(ctx, src.timeout)

		tokens := strings.Fields(src.command)
		args := append(tokens, "--format", "json")
		result := exec.Execute(srcCtx, args)

		cancel()

		if result.Err != nil || result.Stdout == "" {
			omissions = append(omissions, src.name)
			continue
		}

		rows, err := parseSourceOutput(src.name, result.Stdout)
		if err != nil {
			omissions = append(omissions, src.name)
			continue
		}
		allRows = append(allRows, rows...)
	}

	// Build output. Start with the table body; if no targets found at all
	// (whether from errors or empty results), give a clear message.
	body := formatTargetsTable(allRows)
	if len(allRows) == 0 {
		body = "No Neo4j targets found from any source."
	}

	// Append omission note when at least one source was unavailable.
	if len(omissions) > 0 {
		body += fmt.Sprintf("\n\nSources not available: %s.", strings.Join(omissions, ", "))
	}

	// Per REQ-F-031: every tool result passes through RedactText then
	// StripControl. Aura instance names and credential URIs can carry
	// sensitive values.
	body = commonoutput.StripControl(clievents.RedactText(body))

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: body},
		},
	}, nil
}

// parseSourceOutput unmarshals a --format json output from one source and
// maps each entry to a targetRow.
func parseSourceOutput(source, stdout string) ([]targetRow, error) {
	var raw []map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return nil, err
	}

	rows := make([]targetRow, 0, len(raw))
	for _, item := range raw {
		row := targetRow{Source: source}
		row.Name = stringField(item, "name")
		row.Status = stringField(item, "status")

		switch source {
		case "docker":
			row.Version = stringField(item, "version")
			row.Connection = fmt.Sprintf("bolt://localhost:%s", stringField(item, "bolt_port"))
			if row.Status == "" {
				row.Status = stringField(item, "status")
			}
		case "desktop":
			row.Version = stringField(item, "version")
			row.Connection = stringField(item, "connection_uri")
		case "credential":
			// Credentials are connection profiles, always stored and ready.
			// Connection shows the stored URI. The model passes --credential
			// <name> to connect through a stored profile.
			row.Status = "stored"
			row.Connection = stringField(item, "uri")
		case "aura":
			row.Connection = stringField(item, "id")
			row.Organization = stringField(item, "organization_id")
			if row.Name == "" {
				row.Name = stringField(item, "id")
			}
		}

		rows = append(rows, row)
	}
	return rows, nil
}

// stringField extracts a string value from a map, returning "" when the key
// is absent or the value is not a string.
func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// colDef defines a table column: name (header) and format width. width 0
// means unbounded (%s format, no truncation).
type colDef struct {
	name  string
	width int
}

// targetColumns is the column layout for the discovery table. The header,
// separator, and data-row rendering derive from this slice. When adding a
// column, also update the vals slice in the data-row loop below.
var targetColumns = []colDef{
	{name: "source", width: 10},
	{name: "name", width: 30},
	{name: "status", width: 12},
	{name: "version", width: 10},
	{name: "connection", width: 0},   // unbounded
	{name: "organization", width: 0}, // unbounded last column
}

// unboundedSepWidth is the visual dash count in the separator line for
// unbounded (width==0) columns.
const unboundedSepWidth = 40

// formatTargetsTable renders target rows as a fixed-width table driven by
// targetColumns. An empty slice produces no output.
func formatTargetsTable(rows []targetRow) string {
	if len(rows) == 0 {
		return ""
	}

	// Build format string and separator dash count from the column slice.
	var fmtB strings.Builder
	sepWidth := 0
	for i, c := range targetColumns {
		if c.width > 0 {
			fmt.Fprintf(&fmtB, "%%-%ds", c.width)
			sepWidth += c.width
		} else {
			fmtB.WriteString("%s")
			sepWidth += unboundedSepWidth
		}
		if i < len(targetColumns)-1 {
			fmtB.WriteString("  ")
			sepWidth += 2
		}
	}
	fmtB.WriteString("\n")
	rowFmt := fmtB.String()

	var b strings.Builder

	// Header row
	var hdr []any
	for _, c := range targetColumns {
		hdr = append(hdr, c.name)
	}
	fmt.Fprintf(&b, rowFmt, hdr...)
	b.WriteString(strings.Repeat("-", sepWidth) + "\n")

	// Data rows
	for _, r := range rows {
		vals := []string{r.Source, r.Name, r.Status, r.Version, r.Connection, r.Organization}
		var args []any
		for i, c := range targetColumns {
			if c.width > 0 {
				args = append(args, truncateField(vals[i], c.width))
			} else {
				args = append(args, vals[i])
			}
		}
		fmt.Fprintf(&b, rowFmt, args...)
	}
	return b.String()
}

// truncateField truncates a string to max runes.
func truncateField(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
