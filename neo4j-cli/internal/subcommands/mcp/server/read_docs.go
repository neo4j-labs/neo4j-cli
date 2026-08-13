// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clievents"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/skill"
)

// DefaultReadDocsMaxChars is the default max_chars value for read_docs.
const DefaultReadDocsMaxChars = 6000

// MaxReadDocsMaxChars is the maximum allowed max_chars value.
const MaxReadDocsMaxChars = 20000

// HandleReadDocs handles the neo4j_cli_read_docs tool call. It resolves a
// space-separated CLI path against the embedded skill bundle by matching the
// heading whose text is the full command path, returning content until the next
// heading of level <= the matched level. A tree name alone returns only the
// tree's own prose, not its children.
func HandleReadDocs(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	command, offset, maxChars := parseReadDocsArgs(req)
	if command == "" {
		return &mcpsdk.CallToolResult{
			IsError: true,
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: "Missing required argument: command"},
			},
		}, nil
	}

	tokens := strings.Fields(command)
	// A whitespace-only command yields no tokens; indexing would panic, and
	// this handler is not recover()-wrapped so it would take the server down.
	if len(tokens) == 0 {
		return &mcpsdk.CallToolResult{
			IsError: true,
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: "Missing required argument: command"},
			},
		}, nil
	}
	refPath := "references/" + tokens[0] + ".md"

	data, err := fs.ReadFile(skill.Bundle, refPath)
	if err != nil {
		return &mcpsdk.CallToolResult{
			IsError: true,
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: fmt.Sprintf("No documentation found for %q", command)},
			},
		}, nil
	}

	section := extractSection(string(data), command)
	if section == "" {
		return &mcpsdk.CallToolResult{
			IsError: true,
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: fmt.Sprintf("No documentation section found for %q", command)},
			},
		}, nil
	}

	return paginateSection(section, offset, maxChars), nil
}

// parseReadDocsArgs extracts command, offset, and max_chars from a raw
// CallToolRequest arguments map.
func parseReadDocsArgs(req *mcpsdk.CallToolRequest) (command string, offset, maxChars int) {
	maxChars = DefaultReadDocsMaxChars
	if req == nil || req.Params == nil || req.Params.Arguments == nil {
		return "", offset, maxChars
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return "", offset, maxChars
	}
	command, _ = args["command"].(string)
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		offset = int(v)
	}
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
		if maxChars > MaxReadDocsMaxChars {
			maxChars = MaxReadDocsMaxChars
		}
	}
	return command, offset, maxChars
}

// extractSection finds the heading matching commandPath in the markdown
// content and returns the section from that heading to the appropriate
// stopping point. A tree-only request (H1 match) returns only the tree's
// own prose, not its children.
func extractSection(content, commandPath string) string {
	lines := strings.Split(content, "\n")
	target := "neo4j-cli " + commandPath
	matchLevel := 0
	matchIdx := -1

	for i, line := range lines {
		level := headingLevel(line)
		if level == 0 {
			continue
		}
		text := strings.TrimSpace(line[level:])
		if text == target {
			matchLevel = level
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return ""
	}

	// Find the stop line based on heading hierarchy.
	stopIdx := len(lines)

	if matchLevel == 1 {
		// Tree-only (H1) match: stop at the first H2 that is a child command
		// heading (starts with "neo4j-cli"). This gives Contents TOC + tree
		// prose but not children.
		for i := matchIdx + 1; i < len(lines); i++ {
			level, text := headingText(lines[i])
			if level == 2 && strings.HasPrefix(text, "neo4j-cli") {
				stopIdx = i
				break
			}
		}
	} else {
		// General rule: stop at the next heading of level <= matchLevel that
		// is a command heading (starts with "neo4j-cli"). headingText extracts
		// the text after the # markers, so a code-block line like
		// "# unexpected text" cannot match.
		for i := matchIdx + 1; i < len(lines); i++ {
			level, text := headingText(lines[i])
			if level > 0 && level <= matchLevel && strings.HasPrefix(text, "neo4j-cli") {
				stopIdx = i
				break
			}
		}
	}

	return strings.Join(lines[matchIdx:stopIdx], "\n")
}

// headingText returns the heading level and the text after the # markers.
// A non-heading line returns (0, ""). Unlike headingLevel alone, this
// extracts the text portion for prefix matching against command paths.
func headingText(line string) (int, string) {
	level := headingLevel(line)
	if level == 0 {
		return 0, ""
	}
	return level, strings.TrimSpace(line[level:])
}

// headingLevel returns the markdown heading level of line (number of leading
// '#' characters), or 0 if the line is not a heading.
func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	if len(trimmed) == 0 || trimmed[0] != '#' {
		return 0
	}
	level := 0
	for _, c := range trimmed {
		if c == '#' {
			level++
		} else if c == ' ' {
			break
		} else {
			return 0
		}
	}
	if level < 1 || level > 6 {
		return 0
	}
	return level
}

// paginateSection applies offset and max_chars to a matched section, adding a
// continuation hint and next_offset when truncated. The returned text is
// sanitised through RedactText then StripControl per REQ-F-031 (the tool
// result may end up in a model's context).
func paginateSection(section string, offset, maxChars int) *mcpsdk.CallToolResult {
	runes := []rune(section)

	if offset >= len(runes) {
		return &mcpsdk.CallToolResult{
			IsError: true,
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: "offset exceeds section length"},
			},
		}
	}

	start := offset
	end := start + maxChars
	if end > len(runes) {
		end = len(runes)
	}

	text := string(runes[start:end])
	nextOffset := 0

	if end < len(runes) {
		remaining := len(runes) - end
		text += fmt.Sprintf("\n... truncated: %d chars remain; call again with offset=%d", remaining, end)
		nextOffset = end
	}

	// Per REQ-F-031: every tool result text passes through RedactText then
	// StripControl before being returned. The bundle is build-time generated
	// and contains no runtime secrets, but this path must stay consistent
	// with the rest of the tool surface (result.go:sanitize).
	text = commonoutput.StripControl(clievents.RedactText(text))

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: text},
		},
		StructuredContent: struct {
			NextOffset int `json:"next_offset,omitempty"`
		}{
			NextOffset: nextOffset,
		},
	}
}
