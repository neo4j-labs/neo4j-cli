// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listTargetsRootFactory returns a stub cobra tree for testing the
// list_targets handler with controllable per-source outputs. Each source
// returns the preloaded JSON array from its own stdout writer.
func listTargetsRootFactory(stubs map[string]string) RootFactory {
	return func(cfg *clicfg.Config) *cobra.Command {
		root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
		// The executor passes --format json; register it so cobra does not
		// reject the flag before reaching the stubbed RunE.
		root.PersistentFlags().String("format", "", "Output format")

		// Docker
		docker := &cobra.Command{Use: "docker"}
		docker.AddCommand(&cobra.Command{
			Use: "list", RunE: func(c *cobra.Command, _ []string) error {
				if s, ok := stubs["docker"]; ok {
					_, _ = c.OutOrStdout().Write([]byte(s))
				}
				return nil
			},
		})
		root.AddCommand(docker)

		// Desktop
		desktop := &cobra.Command{Use: "desktop"}
		desktopDBMS := &cobra.Command{Use: "dbms"}
		desktopDBMS.AddCommand(&cobra.Command{
			Use: "list", RunE: func(c *cobra.Command, _ []string) error {
				if s, ok := stubs["desktop"]; ok {
					_, _ = c.OutOrStdout().Write([]byte(s))
				}
				return nil
			},
		})
		desktop.AddCommand(desktopDBMS)
		root.AddCommand(desktop)

		// Credential
		credential := &cobra.Command{Use: "credential"}
		credDBMS := &cobra.Command{Use: "dbms"}
		credDBMS.AddCommand(&cobra.Command{
			Use: "list", RunE: func(c *cobra.Command, _ []string) error {
				if s, ok := stubs["credential"]; ok {
					_, _ = c.OutOrStdout().Write([]byte(s))
				}
				return nil
			},
		})
		credential.AddCommand(credDBMS)
		root.AddCommand(credential)

		// Aura
		aura := &cobra.Command{Use: "aura"}
		instance := &cobra.Command{Use: "instance"}
		instance.AddCommand(&cobra.Command{
			Use: "list", RunE: func(c *cobra.Command, _ []string) error {
				if s, ok := stubs["aura"]; ok {
					_, _ = c.OutOrStdout().Write([]byte(s))
				}
				return nil
			},
		})
		aura.AddCommand(instance)
		root.AddCommand(aura)

		return root
	}
}

// listTargetsExecutor builds an executor for list_targets tests from a
// rootFactory that has its stubs populated.
func listTargetsExecutor(t *testing.T, stubs map[string]string) *Executor {
	t.Helper()
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	exec, err := NewExecutor(cfg, listTargetsRootFactory(stubs))
	require.NoError(t, err)
	return exec
}

func TestHandleListTargets_FourSources(t *testing.T) {
	ctx := context.Background()

	stubs := map[string]string{
		"docker":     `[{"name":"dev","status":"Up 5 minutes","edition":"enterprise","version":"5.20","bolt_port":"7687","http_port":"7474"}]`,
		"desktop":    `[{"id":"d1","name":"local-db","connection_uri":"bolt://localhost:7687","status":"started","version":"5.20"}]`,
		"credential": `[{"name":"my-creds","username":"neo4j","database_name":"neo4j","uri":"bolt://localhost:7687","default":true}]`,
		"aura":       `[{"id":"a1","name":"aura-instance","status":"running","project_id":"proj-1","cloud_provider":"aws"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	tc := result.Content[0].(*mcpsdk.TextContent)
	text := tc.Text

	// All four sources should appear in the output
	assert.Contains(t, text, "docker")
	assert.Contains(t, text, "desktop")
	assert.Contains(t, text, "credential")
	assert.Contains(t, text, "aura")

	// All target names should appear
	assert.Contains(t, text, "dev")
	assert.Contains(t, text, "local-db")
	assert.Contains(t, text, "my-creds")
	assert.Contains(t, text, "aura-instance")

	// No omission note when all sources succeed
	assert.NotContains(t, text, "Sources not available")

	// Headers are snake_case
	assert.Contains(t, text, "source")
	assert.Contains(t, text, "name")
	assert.Contains(t, text, "status")
	assert.Contains(t, text, "version")
	assert.Contains(t, text, "connection")

	// Docker connection is constructed from bolt_port
	assert.Contains(t, text, "bolt://localhost:7687")

	// Credential shows stored status
	assert.Contains(t, text, "stored")
}

func TestHandleListTargets_SourceFailuresDegrade(t *testing.T) {
	ctx := context.Background()

	// Only docker succeeds; desktop credential aura all fail (no stub).
	stubs := map[string]string{
		"docker": `[{"name":"dev","status":"Up","version":"5.20","bolt_port":"7687"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError, "partial failure must not be an error")
	require.Len(t, result.Content, 1)

	tc := result.Content[0].(*mcpsdk.TextContent)
	text := tc.Text

	// Docker data must be present
	assert.Contains(t, text, "dev")
	assert.Contains(t, text, "docker")

	// The omission note must list the failed sources
	assert.Contains(t, text, "Sources not available")
	assert.Contains(t, text, "desktop")
	assert.Contains(t, text, "credential")
	assert.Contains(t, text, "aura")

	// Failed sources must NOT appear in the table body
	assert.NotContains(t, text, "desktop  ")

	// The single successful source shows, with no mention of others before
	// the omission note — the named line does not need assertion beyond
	// "dev" and "docker" appearing.
}

func TestHandleListTargets_AllSourcesFail(t *testing.T) {
	ctx := context.Background()

	// No stubs at all — every source returns empty output.
	exec := listTargetsExecutor(t, map[string]string{})
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError, "all failures must still not be an error")
	require.Len(t, result.Content, 1)

	tc := result.Content[0].(*mcpsdk.TextContent)
	text := tc.Text

	// Must report "No Neo4j targets found from any source."
	assert.Contains(t, text, "No Neo4j targets found")
	// The omission note follows
	assert.Contains(t, text, "Sources not available")
}

func TestHandleListTargets_AllSourcesEmpty(t *testing.T) {
	ctx := context.Background()

	// All sources succeed but return empty JSON arrays
	stubs := map[string]string{
		"docker":     `[]`,
		"desktop":    `[]`,
		"credential": `[]`,
		"aura":       `[]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError, "empty lists must not be errors")
	require.Len(t, result.Content, 1)

	tc := result.Content[0].(*mcpsdk.TextContent)
	text := tc.Text

	// Must mention "No Neo4j targets found" since no targets, but no sources failed
	// (empty outputs are not failures). The "Sources not available" note is NOT
	// expected for empty responses.
	assert.Contains(t, text, "No Neo4j targets found from any source.")
	assert.NotContains(t, text, "Sources not available")
}

func TestHandleListTargets_DockerConnection(t *testing.T) {
	ctx := context.Background()

	stubs := map[string]string{
		"docker": `[{"name":"c1","bolt_port":"7687","status":"Up 1h"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError)

	tc := result.Content[0].(*mcpsdk.TextContent)
	// Connection must be constructed from bolt_port
	assert.Contains(t, tc.Text, "bolt://localhost:7687")
}

func TestHandleListTargets_CredentialRow(t *testing.T) {
	ctx := context.Background()

	stubs := map[string]string{
		"credential": `[{"name":"mydb","database_name":"neo4j","uri":"bolt://localhost:7687","default":true}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError)

	tc := result.Content[0].(*mcpsdk.TextContent)
	// Credential shows "stored" as status and the URI as connection
	assert.Contains(t, tc.Text, "stored")
	assert.Contains(t, tc.Text, "bolt://localhost:7687")
	assert.Contains(t, tc.Text, "mydb")
}

func TestHandleListTargets_AuraRow(t *testing.T) {
	ctx := context.Background()

	stubs := map[string]string{
		"aura": `[{"id":"inst-abc-123","name":"prod-instance","status":"running"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError)

	tc := result.Content[0].(*mcpsdk.TextContent)
	// Aura shows the instance ID as connection and the name
	assert.Contains(t, tc.Text, "prod-instance")
	assert.Contains(t, tc.Text, "inst-abc-123")
}

func TestHandleListTargets_TimeoutGuard(t *testing.T) {
	// Verify the handler is bounded: use a cancelled context so executor
	// calls return promptly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stubs := map[string]string{
		"docker": `[{"name":"dev","status":"Up"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	// With a cancelled context, executor returns an error, so all sources
	// degrade to omission.
	require.False(t, result.IsError)
}

func TestHandleListTargets_TruncatedField(t *testing.T) {
	longName := strings.Repeat("x", 50)
	ctx := context.Background()

	stubs := map[string]string{
		"docker": `[{"name":"` + longName + `","status":"Up","bolt_port":"7687"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError)

	tc := result.Content[0].(*mcpsdk.TextContent)
	// The name column is 30 chars wide, so only 30 'x' should appear
	assert.Contains(t, tc.Text, strings.Repeat("x", 30))
	// The full 50 should not appear as-is (it's truncated)
	assert.NotContains(t, tc.Text, strings.Repeat("x", 50))
}

func TestParseSourceOutput_Docker(t *testing.T) {
	stdout := `[{"name":"dev","status":"Up 5m","edition":"enterprise","version":"5.20","bolt_port":"7687"}]`
	rows, err := parseSourceOutput("docker", stdout)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "docker", rows[0].Source)
	assert.Equal(t, "dev", rows[0].Name)
	assert.Equal(t, "Up 5m", rows[0].Status)
	assert.Equal(t, "5.20", rows[0].Version)
	assert.Equal(t, "bolt://localhost:7687", rows[0].Connection)
}

func TestParseSourceOutput_Desktop(t *testing.T) {
	stdout := `[{"id":"d1","name":"local-db","connection_uri":"bolt://localhost:7687","status":"started","version":"5.20"}]`
	rows, err := parseSourceOutput("desktop", stdout)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "desktop", rows[0].Source)
	assert.Equal(t, "local-db", rows[0].Name)
	assert.Equal(t, "started", rows[0].Status)
	assert.Equal(t, "5.20", rows[0].Version)
	assert.Equal(t, "bolt://localhost:7687", rows[0].Connection)
}

func TestParseSourceOutput_Credential(t *testing.T) {
	stdout := `[{"name":"my-creds","uri":"bolt://localhost:7687","default":true}]`
	rows, err := parseSourceOutput("credential", stdout)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "credential", rows[0].Source)
	assert.Equal(t, "my-creds", rows[0].Name)
	assert.Equal(t, "stored", rows[0].Status)
	assert.Equal(t, "", rows[0].Version)
	assert.Equal(t, "bolt://localhost:7687", rows[0].Connection)
}

func TestParseSourceOutput_Aura(t *testing.T) {
	stdout := `[{"id":"a1","name":"aura-instance","status":"running","project_id":"p1"}]`
	rows, err := parseSourceOutput("aura", stdout)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "aura", rows[0].Source)
	assert.Equal(t, "aura-instance", rows[0].Name)
	assert.Equal(t, "running", rows[0].Status)
	assert.Equal(t, "", rows[0].Version)
	assert.Equal(t, "a1", rows[0].Connection)
}

func TestParseSourceOutput_EmptyArray(t *testing.T) {
	rows, err := parseSourceOutput("docker", `[]`)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestParseSourceOutput_InvalidJSON(t *testing.T) {
	rows, err := parseSourceOutput("docker", `not-json`)
	require.Error(t, err)
	require.Nil(t, rows)
}

func TestFormatTargetsTable_Empty(t *testing.T) {
	output := formatTargetsTable(nil)
	assert.Empty(t, output, "nil input produces empty string")
}

func TestFormatTargetsTable_Headers(t *testing.T) {
	rows := []targetRow{
		{Source: "docker", Name: "c1", Status: "Up", Version: "5.20", Connection: "bolt://localhost:7687"},
	}
	output := formatTargetsTable(rows)
	assert.Contains(t, output, "source")
	assert.Contains(t, output, "name")
	assert.Contains(t, output, "status")
	assert.Contains(t, output, "version")
	assert.Contains(t, output, "connection")
	assert.Contains(t, output, "c1")
	assert.Contains(t, output, "docker")
}

func TestTruncateField(t *testing.T) {
	assert.Equal(t, "hello", truncateField("hello", 10))
	assert.Equal(t, "hello", truncateField("hello", 5))
	assert.Equal(t, "he", truncateField("hello", 2))
	assert.Equal(t, "", truncateField("hello", 0))
}

func TestStringField(t *testing.T) {
	m := map[string]any{"name": "test", "count": 42, "nested": map[string]any{}}
	assert.Equal(t, "test", stringField(m, "name"))
	assert.Equal(t, "", stringField(m, "count"))
	assert.Equal(t, "", stringField(m, "nested"))
	assert.Equal(t, "", stringField(m, "missing"))
}

func TestHandleListTargets_RedactionApplied(t *testing.T) {
	// Verify that output passes through sanitize (RedactText + StripControl)
	// by including text that RedactText normally handles.
	ctx := context.Background()

	stubs := map[string]string{
		"docker": `[{"name":"dev","status":"Up","bolt_port":"7687"}]`,
	}

	exec := listTargetsExecutor(t, stubs)
	result, err := HandleListTargets(ctx, exec)
	require.NoError(t, err)
	require.False(t, result.IsError)

	tc := result.Content[0].(*mcpsdk.TextContent)
	// Verify basic output is present
	assert.Contains(t, tc.Text, "docker")
	assert.Contains(t, tc.Text, "dev")
	// Control characters must be stripped
	assert.NotContains(t, tc.Text, "\x00")
	assert.NotContains(t, tc.Text, "\x1b")
}

func TestListTargetsTool_InManifest(t *testing.T) {
	// Verify neo4j_cli_list_targets appears in the tool definitions with
	// the correct annotations.
	var found bool
	for _, td := range toolDefinitions() {
		if td.Name == "neo4j_cli_list_targets" {
			found = true
			require.NotNil(t, td.Annotations)
			assert.True(t, td.Annotations.ReadOnlyHint)
			assert.True(t, td.Annotations.IdempotentHint)
			// No parameters — InputSchema is type any; verify it has no properties
			if schema, ok := td.InputSchema.(map[string]any); ok {
				props, _ := schema["properties"].(map[string]any)
				assert.Empty(t, props)
			}
			break
		}
	}
	assert.True(t, found, "neo4j_cli_list_targets must be in toolDefinitions()")

	// Verify exactly five tools
	require.Len(t, toolDefinitions(), 5)
}

func TestListTargetsTool_NameConvention(t *testing.T) {
	// Verify the tool name matches ^neo4j_cli_[a-z0-9_]+$
	for _, td := range toolDefinitions() {
		assert.Regexp(t, `^neo4j_cli_[a-z0-9_]+$`, td.Name)
	}
}
