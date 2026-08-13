// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// headingLevel and headingText cases.
func TestHeadingLevel(t *testing.T) {
	tests := []struct {
		line string
		want int
	}{
		{"# H1", 1},
		{"## H2", 2},
		{"### H3", 3},
		{"#  spaced", 1},
		{"  # indented", 1},
		{"not a heading", 0},
		{"", 0},
		{"#no-space", 0},
		{"####### too many", 0},
	}
	for _, tt := range tests {
		got := headingLevel(tt.line)
		assert.Equal(t, tt.want, got, "headingLevel(%q)", tt.line)
	}
}

func TestHeadingText(t *testing.T) {
	level, text := headingText("## neo4j-cli docker load")
	assert.Equal(t, 2, level)
	assert.Equal(t, "neo4j-cli docker load", text)

	level, text = headingText("not a heading")
	assert.Equal(t, 0, level)
	assert.Equal(t, "", text)

	level, text = headingText("")
	assert.Equal(t, 0, level)
	assert.Equal(t, "", text)
}

// H3 heading slicing — exercises the stop-at-next-sibling logic for deeper
// nesting levels (PDRD expects children of matched headings to be excluded).
func TestExtractSection_H3Leaf(t *testing.T) {
	content := `# neo4j-cli test

Test tree

## neo4j-cli test parent

Parent command

### neo4j-cli test parent child

Child command

Usage: child

### neo4j-cli test parent sibling

Another child

## neo4j-cli test other

Other parent
`
	got := extractSection(content, "test parent child")
	assert.Contains(t, got, "neo4j-cli test parent child")
	assert.Contains(t, got, "Usage: child")
	assert.NotContains(t, got, "neo4j-cli test parent sibling", "must stop at next H3 sibling")
	assert.NotContains(t, got, "neo4j-cli test other", "must stop at ancestor H2")
}

// extractSection cases cover tree-level (H1) and command-level (H2) matches.
func TestExtractSection_TreeOnly(t *testing.T) {
	content := `# neo4j-cli docker

## Contents

- [neo4j-cli docker create](#neo4j-cli-docker-create)

Manage Docker containers

Usage: neo4j-cli docker

Flags:

| Flag | Description |
|------|-------------|
| --debug | Enable debug |

## neo4j-cli docker create

Create a container

## neo4j-cli docker list

List containers
`
	got := extractSection(content, "docker")
	assert.Contains(t, got, "neo4j-cli docker")
	// TOC entry mentions the child name, but the child section heading must not
	// be present — tree-only returns Contents TOC + prose, not child sections.
	assert.Contains(t, got, "Contents")
	assert.Contains(t, got, "Manage Docker containers")
	assert.NotContains(t, got, "## neo4j-cli docker create", "child section heading must not be present")
	assert.NotContains(t, got, "## neo4j-cli docker list", "child section heading must not be present")
}

func TestExtractSection_LeafCommand(t *testing.T) {
	content := `# neo4j-cli docker

## neo4j-cli docker create

Create a container

Usage: neo4j-cli docker create

Flags:

| Flag | Description |
|------|-------------|
| --name | Container name |

Examples:

neo4j-cli docker create --name dev

## neo4j-cli docker list

List containers
`
	got := extractSection(content, "docker create")
	assert.Contains(t, got, "neo4j-cli docker create")
	assert.Contains(t, got, "--name")
	assert.Contains(t, got, "Examples:")
	assert.NotContains(t, got, "neo4j-cli docker list")
}

func TestExtractSection_NotFound(t *testing.T) {
	got := extractSection("# neo4j-cli docker\n\ncontent", "docker nonexistent")
	assert.Equal(t, "", got)
}

func TestExtractSection_Empty(t *testing.T) {
	got := extractSection("", "docker")
	assert.Equal(t, "", got)
}

// paginateSection tests cover offset, max_chars, and continuation.
func TestPaginateSection_NoTruncation(t *testing.T) {
	section := "short text"
	result := paginateSection(section, 0, 1000)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Equal(t, section, tc.Text)
}

func TestPaginateSection_TruncateAtMaxChars(t *testing.T) {
	section := strings.Repeat("a", 200)
	result := paginateSection(section, 0, 50)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.LessOrEqual(t, len([]rune(tc.Text)), 200)
	assert.Contains(t, tc.Text, "... truncated:")
	assert.Contains(t, tc.Text, "call again with offset=50")
	require.NotNil(t, result.StructuredContent)
}

func TestPaginateSection_Offset(t *testing.T) {
	section := "abcdefghij"
	result := paginateSection(section, 3, 100)
	require.False(t, result.IsError)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Equal(t, "defghij", tc.Text)
}

func TestPaginateSection_OffsetBeyondEnd(t *testing.T) {
	section := "abc"
	result := paginateSection(section, 10, 5)
	require.True(t, result.IsError)
}

// Handler smoke tests — use the real embedded bundle so we know the full path
// from argument to heading match works end to end.
func TestHandleReadDocs_RealBundle_DockerLoad(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load"}`),
		},
	}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)

	assert.Contains(t, tc.Text, "neo4j-cli docker load")
	assert.Contains(t, tc.Text, "Flags:")
	assert.Contains(t, tc.Text, "Examples:")
}

func TestHandleReadDocs_RealBundle_Aura(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"aura"}`),
		},
	}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)

	assert.Contains(t, tc.Text, "neo4j-cli aura")
	assert.Contains(t, tc.Text, "Contents")
	// The Contents TOC mentions child names, but child section headings must not
	// be present — tree-only returns Contents TOC + prose, not child sections.
	assert.NotContains(t, tc.Text, "## neo4j-cli aura instance", "child section heading must not be present")
	assert.NotContains(t, tc.Text, "## neo4j-cli aura project", "child section heading must not be present")
}

func TestHandleReadDocs_UnknownTree(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"nonexistent"}`),
		},
	}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
}

func TestHandleReadDocs_MissingCommand(t *testing.T) {
	req := &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{}}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
}

func TestHandleReadDocs_Truncation(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","max_chars":10}`),
		},
	}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "... truncated:")
	assert.Contains(t, tc.Text, "call again with offset=10")
	require.NotNil(t, result.StructuredContent)
}

func TestHandleReadDocs_Offset(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","offset":5,"max_chars":100}`),
		},
	}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.NotEmpty(t, tc.Text)
}

// Ensure the real handler works on the real bundle (calls HandleReadDocs which
// accesses skill.Bundle internally).
func TestHandleReadDocs_UsesEmbeddedBundle(t *testing.T) {
	// Just verify the bundle path is wired right — a non-bundle error would
	// return an IsError result, not a panic.
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"query"}`),
		},
	}
	result, err := HandleReadDocs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "query tree must resolve from the embedded bundle")
}

// TestHandleReadDocs_WhitespaceOnlyCommand guards the panic found by the
// security review: strings.Fields on a whitespace-only command returns no
// tokens, and this handler is not recover()-wrapped, so indexing tokens[0]
// would crash the serve process.
func TestHandleReadDocs_WhitespaceOnlyCommand(t *testing.T) {
	for _, cmd := range []string{"   ", "\t", " \t "} {
		raw, err := json.Marshal(map[string]any{"command": cmd})
		require.NoError(t, err)
		res, err := HandleReadDocs(context.Background(),
			&mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{Arguments: raw}})
		require.NoError(t, err, "must not error at the protocol level for %q", cmd)
		require.NotNil(t, res)
		assert.True(t, res.IsError, "whitespace-only command %q must be a tool error", cmd)
	}
}
