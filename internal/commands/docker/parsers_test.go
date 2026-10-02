// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParsePsOutput exercises the JSON-per-line decode of
// `docker ps --format '{{json .}}'`. Fixtures are hand-crafted from Docker's
// documented field shape (Title-Case keys, comma-separated label string).
func TestParsePsOutput(t *testing.T) {
	managedLine := `{"ID":"abc123","Names":"dev","Status":"Up 2 minutes","State":"running","Image":"neo4j:latest-enterprise","Labels":"org.neo4j.cli.managed=true,org.neo4j.cli.edition=enterprise,org.neo4j.cli.version=latest,org.neo4j.cli.bolt-port=7687,org.neo4j.cli.http-port=7474,org.neo4j.cli.ephemeral=false"}`
	unmanagedLine := `{"ID":"def456","Names":"other","Status":"Exited (0) 5 minutes ago","State":"exited","Image":"redis:7","Labels":"app=redis"}`

	t.Run("empty stdout returns empty slice and nil error", func(t *testing.T) {
		got, err := parsePsOutput("")
		require.NoError(t, err)
		assert.Equal(t, []PsEntry{}, got)
	})

	t.Run("one managed container line", func(t *testing.T) {
		got, err := parsePsOutput(managedLine + "\n")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "abc123", got[0].ID)
		assert.Equal(t, "dev", got[0].Names)
		assert.Equal(t, "Up 2 minutes", got[0].Status)
		assert.Equal(t, "running", got[0].State)
		assert.Equal(t, "neo4j:latest-enterprise", got[0].Image)
		// Labels is the raw comma-separated string Docker emits in this format.
		assert.Contains(t, got[0].Labels, "org.neo4j.cli.managed=true")
		assert.Contains(t, got[0].Labels, "org.neo4j.cli.edition=enterprise")
	})

	t.Run("two lines preserves order (managed + unmanaged)", func(t *testing.T) {
		got, err := parsePsOutput(managedLine + "\n" + unmanagedLine + "\n")
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "dev", got[0].Names)
		assert.Equal(t, "other", got[1].Names)
	})

	t.Run("trailing newline tolerated", func(t *testing.T) {
		got, err := parsePsOutput(managedLine + "\n\n")
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("malformed JSON on second line surfaces line number and no partial result", func(t *testing.T) {
		got, err := parsePsOutput(managedLine + "\nnot-json-here\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 2")
		assert.Nil(t, got, "partial result must not be returned on parse failure")
	})

	t.Run("malformed JSON on first line surfaces line number 1", func(t *testing.T) {
		_, err := parsePsOutput("not-json\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 1")
	})

	t.Run("oversized label payload beyond default 64KB is handled", func(t *testing.T) {
		// Build a fixture with a label payload that exceeds bufio.Scanner's
		// default 64KB token cap. We rely on the parser's explicit 4MB buffer.
		big := strings.Repeat("a", 200*1024)
		line := `{"ID":"x","Names":"big","Status":"running","State":"running","Image":"neo4j","Labels":"org.neo4j.cli.managed=true,blob=` + big + `"}`
		got, err := parsePsOutput(line + "\n")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "big", got[0].Names)
	})
}

// TestParseInspectOutput exercises the JSON-array decode of
// `docker inspect <name>`. Hand-crafted fixtures mirror Docker's documented
// engine-API response shape.
func TestParseInspectOutput(t *testing.T) {
	fullFixture := `[
		{
			"Name": "/dev",
			"State": {
				"Status": "running",
				"Running": true
			},
			"Config": {
				"Image": "neo4j:latest-enterprise",
				"Labels": {
					"org.neo4j.cli.managed": "true",
					"org.neo4j.cli.edition": "enterprise",
					"org.neo4j.cli.version": "latest",
					"org.neo4j.cli.bolt-port": "7687",
					"org.neo4j.cli.http-port": "7474",
					"org.neo4j.cli.ephemeral": "true"
				}
			}
		}
	]`

	t.Run("single-element array with full label set populates every field", func(t *testing.T) {
		got, err := parseInspectOutput("dev", fullFixture)
		require.NoError(t, err)
		assert.Equal(t, "dev", got.Name, "leading / must be stripped")
		assert.Equal(t, "running", got.Status)
		assert.True(t, got.Running)
		assert.Equal(t, "neo4j:latest-enterprise", got.Image)
		assert.Equal(t, "enterprise", got.Edition)
		assert.Equal(t, "latest", got.Version)
		assert.Equal(t, "7687", got.BoltPort)
		assert.Equal(t, "7474", got.HTTPPort)
		assert.True(t, got.Ephemeral)
		assert.True(t, got.Managed)
		// URI is the leaf's job, not the parser's (REQ-F-031).
		assert.Empty(t, got.URI)
	})

	t.Run("Name with leading slash is stripped", func(t *testing.T) {
		fix := `[{"Name":"/myctr","State":{"Status":"running","Running":true},"Config":{"Image":"x","Labels":{}}}]`
		got, err := parseInspectOutput("myctr", fix)
		require.NoError(t, err)
		assert.Equal(t, "myctr", got.Name)
	})

	t.Run("State.Running false flows through", func(t *testing.T) {
		fix := `[{"Name":"/stopped","State":{"Status":"exited","Running":false},"Config":{"Image":"x","Labels":{"org.neo4j.cli.managed":"true"}}}]`
		got, err := parseInspectOutput("stopped", fix)
		require.NoError(t, err)
		assert.False(t, got.Running)
		assert.Equal(t, "exited", got.Status)
	})

	t.Run("missing labels block yields empty strings and Managed=false (NOT an error)", func(t *testing.T) {
		fix := `[{"Name":"/anon","State":{"Status":"running","Running":true},"Config":{"Image":"redis:7","Labels":null}}]`
		got, err := parseInspectOutput("anon", fix)
		require.NoError(t, err)
		assert.Equal(t, "anon", got.Name)
		assert.False(t, got.Managed)
		assert.False(t, got.Ephemeral)
		assert.Empty(t, got.Edition)
		assert.Empty(t, got.Version)
		assert.Empty(t, got.BoltPort)
		assert.Empty(t, got.HTTPPort)
	})

	t.Run("ephemeral label false means Ephemeral=false", func(t *testing.T) {
		fix := `[{"Name":"/a","State":{"Running":true},"Config":{"Image":"x","Labels":{"org.neo4j.cli.ephemeral":"false"}}}]`
		got, err := parseInspectOutput("a", fix)
		require.NoError(t, err)
		assert.False(t, got.Ephemeral)
	})

	t.Run("ephemeral label missing means Ephemeral=false", func(t *testing.T) {
		fix := `[{"Name":"/a","State":{"Running":true},"Config":{"Image":"x","Labels":{}}}]`
		got, err := parseInspectOutput("a", fix)
		require.NoError(t, err)
		assert.False(t, got.Ephemeral)
	})

	t.Run("empty array returns error mentioning expected count", func(t *testing.T) {
		got, err := parseInspectOutput("dev", `[]`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected 1 container, got 0")
		assert.Equal(t, Container{}, got)
	})

	t.Run("two-element array returns error mentioning got 2", func(t *testing.T) {
		fix := `[{"Name":"/a","Config":{"Labels":{}}},{"Name":"/b","Config":{"Labels":{}}}]`
		_, err := parseInspectOutput("dev", fix)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected 1 container, got 2")
	})

	t.Run("malformed JSON returns error naming the container", func(t *testing.T) {
		_, err := parseInspectOutput("dev", `"not json"`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"dev"`)
	})
}
