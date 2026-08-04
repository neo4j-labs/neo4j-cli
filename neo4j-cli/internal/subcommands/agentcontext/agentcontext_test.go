// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agentcontext_test

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicmd"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/agentcontext"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAppCmd builds the live neo4j-cli cobra tree backed by an in-memory
// filesystem so end-to-end tests can exercise the agent-context command
// through the same entrypoint a real user hits.
func newAppCmd(t *testing.T) *cobra.Command {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	return app.NewCmd(cfg)
}

// newAppCmdEveryFlagEnabled builds the live tree with EVERY registered feature
// flag forced on, so the whole-tree invariant gates below also cover subtrees
// that app.NewCmd mounts only behind a flag (they are absent from newAppCmd's
// default tree, which the shape assertions in this package depend on — hence a
// separate helper rather than a change to newAppCmd).
func newAppCmdEveryFlagEnabled(t *testing.T) *cobra.Command {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	for name := range clicfg.Registry {
		cfg.Flags.SetForTest(name, true)
	}
	return app.NewCmd(cfg)
}

// runAgentContext invokes `agent-context` with the given extra args via the
// live app tree, returning stdout/stderr buffers and the Execute error.
func runAgentContext(t *testing.T, extraArgs ...string) (stdout, stderr *bytes.Buffer, err error) {
	t.Helper()
	cmd := newAppCmd(t)
	args := append([]string{"agent-context"}, extraArgs...)
	cmd.SetArgs(args)
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	err = cmd.Execute()
	return stdout, stderr, err
}

// TestAgentContext_Envelope covers REQ-V-001: the JSON envelope carries
// every documented top-level field with the right cardinality.
func TestAgentContext_Envelope(t *testing.T) {
	stdout, stderr, err := runAgentContext(t, "--format", "json")
	require.NoError(t, err, "agent-context --format json must succeed; stderr=%s", stderr.String())

	var ctx agentcontext.Context
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &ctx),
		"stdout must be valid JSON decodable into Context; got: %s", stdout.String())

	assert.Equal(t, 1, ctx.SchemaVersion, "schema_version must be 1")
	assert.NotEmpty(t, ctx.CliVersion, "cli_version must be non-empty")
	assert.Equal(t, "neo4j-cli", ctx.Binary)
	assert.NotEmpty(t, ctx.Commands, "commands tree must be non-empty (live app has subcommands)")
	assert.Len(t, ctx.ExitCodes, 9, "exit_codes must list entries 0-8 (closed set per agent-cli-auditor §4.1)")
	assert.Equal(t, "success", ctx.ExitCodes["0"])
	for _, code := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		assert.NotEmpty(t, ctx.ExitCodes[code], "exit_codes[%q] must have a description", code)
	}
	assert.Len(t, ctx.ErrorCodes, 8, "error_codes must list all eight clierr categories")
	for _, code := range []string{
		"fatal_error",
		"usage_error",
		"not_found",
		"auth_error",
		"conflict",
		"validation_error",
		"rate_limited",
		"upstream_error",
	} {
		assert.Contains(t, ctx.ErrorCodes, code, "error_codes must include %q", code)
	}
	assert.Equal(t, "--wait", ctx.AsyncFlag)
	assert.Equal(t, clicfg.ValidFormatValues[:], ctx.OutputFormats)
}

// TestAgentContext_OutputFormatsParity covers REQ-V-002: the emitted
// output_formats slice must be the live clicfg.ValidFormatValues — no
// duplicated literal. reflect.DeepEqual via assert.Equal is sufficient.
func TestAgentContext_OutputFormatsParity(t *testing.T) {
	stdout, _, err := runAgentContext(t, "--format", "json")
	require.NoError(t, err)
	var ctx agentcontext.Context
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &ctx))

	assert.Equal(t, clicfg.ValidFormatValues[:], ctx.OutputFormats,
		"output_formats must equal clicfg.ValidFormatValues[:] — agent-context must not hard-code the format list")
}

// TestAgentContext_TreeCoverage covers REQ-V-003: every IsAvailableCommand
// in the live cobra tree must appear in the emitted JSON tree, and vice
// versa. Failure message names missing / extra paths so a regression is
// trivial to localise.
//
// Note: cobra lazily adds its built-in `completion` command on the first
// Execute() call (cobra.Command.InitDefaultCompletionCmd). To compare apples
// to apples, build ONE tree, Execute() to emit the JSON, then walk THAT
// same instance — both halves see the post-init shape.
func TestAgentContext_TreeCoverage(t *testing.T) {
	cmd := newAppCmd(t)
	cmd.SetArgs([]string{"agent-context", "--format", "json"})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	require.NoError(t, cmd.Execute(), "agent-context invocation must succeed; stderr=%s", stderr.String())

	// Walk the LIVE cobra tree gathering every visible command path. After
	// Execute() the auto-injected `completion` subtree is present.
	livePaths := map[string]bool{}
	var walkLive func(c *cobra.Command, prefix string)
	walkLive = func(c *cobra.Command, prefix string) {
		for _, sub := range c.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			name := strings.ToLower(strings.Fields(sub.Use)[0])
			path := name
			if prefix != "" {
				path = prefix + " " + name
			}
			livePaths[path] = true
			walkLive(sub, path)
		}
	}
	walkLive(cmd, "")

	// Walk the emitted JSON tree gathering the same path set.
	var ctx agentcontext.Context
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &ctx))

	jsonPaths := map[string]bool{}
	var walkJSON func(cmds map[string]agentcontext.Command, prefix string)
	walkJSON = func(cmds map[string]agentcontext.Command, prefix string) {
		for key, c := range cmds {
			path := key
			if prefix != "" {
				path = prefix + " " + key
			}
			jsonPaths[path] = true
			walkJSON(c.Subcommands, path)
		}
	}
	walkJSON(ctx.Commands, "")

	var missing, extra []string
	for p := range livePaths {
		if !jsonPaths[p] {
			missing = append(missing, p)
		}
	}
	for p := range jsonPaths {
		if !livePaths[p] {
			extra = append(extra, p)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	assert.Empty(t, missing, "commands present in the live cobra tree but absent from the emitted JSON: %v", missing)
	assert.Empty(t, extra, "commands present in the emitted JSON but absent from the live cobra tree: %v", extra)
}

// TestAgentContext_FormatRoundTrip covers REQ-V-004 + V-006: every
// declared output format produces non-empty stdout without panic or
// error. For json, the output must round-trip back into Context.
func TestAgentContext_FormatRoundTrip(t *testing.T) {
	for _, format := range []string{"json", "toon", "table"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr, err := runAgentContext(t, "--format", format)
			require.NoError(t, err, "agent-context --format %s must succeed; stderr=%s", format, stderr.String())
			assert.NotEmpty(t, strings.TrimSpace(stdout.String()),
				"agent-context --format %s must write non-empty stdout", format)
			if format == "json" {
				var ctx agentcontext.Context
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &ctx),
					"json output must decode back into Context")
				assert.Equal(t, 1, ctx.SchemaVersion)
			}
		})
	}
}

// TestAgentContext_HelpExamples locks REQ-F-016: the leaf's Example field is
// non-empty AND flush-left. A leading two-space indent on the first line
// would be stripped by common/skill/render and produce a ragged bundle
// (see AGENTS.md "Cobra Help / Skill Bundle Rendering Notes").
func TestAgentContext_HelpExamples(t *testing.T) {
	cmd := newAppCmd(t)

	var leaf *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "agent-context" {
			leaf = c
			break
		}
	}
	require.NotNil(t, leaf, "agent-context command must be registered on the live tree")

	assert.NotEmpty(t, leaf.Example, "agent-context.Example must be non-empty")
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	assert.False(t, strings.HasPrefix(firstLine, "  "),
		"agent-context.Example first line must be flush-left (no leading two-space indent); got %q", firstLine)
}

// TestAllLeafCommands_HaveExamples covers REQ-F-001, REQ-F-012, REQ-F-015:
// every visible, runnable cobra command reachable from app.NewCmd(cfg) carries
// a non-empty Example: field whose first line is flush-left AND contains at
// least two invocations (AGENTS.md "Cobra Help / Skill Bundle Rendering
// Notes" — ≥2 invocations per leaf, each preceded by a `# comment` line).
//
// Filtering mirrors common/skill/render.visibleSubcommands — Hidden commands
// and the cobra-injected `help` / `completion` subtrees are skipped, so the
// gate matches what users see via --help and what ships in the SKILL bundle.
// The `query` parent IS runnable (REQ-F-015) and is therefore checked even
// though it owns the `:schema` subcommand; non-runnable parents are skipped
// (only their runnable descendants are checked).
//
// Failure messages name the offending command by its full path (e.g.
// `neo4j-cli aura instance list`) so a missed leaf is trivial to localise.
//
// The tree is built with every feature flag enabled so flag-gated subtrees are
// held to the same bar as the always-mounted ones.
func TestAllLeafCommands_HaveExamples(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		// Skip parent commands whose RunE was auto-installed by the
		// clicmd suggestion walker — they are nominally Runnable only so
		// that cobra invokes the Args validator that emits 'Did you mean'
		// hints for nested typos; their user-visible behaviour is still
		// to print help, so they do not need a hand-authored Example.
		if c.Annotations[clicmd.SuggestionsRunEAnnotation] == "true" {
			// fall through to recurse into children
		} else if c.Runnable() {
			firstLine := strings.SplitN(c.Example, "\n", 2)[0]
			assert.NotEmpty(t, c.Example,
				"command %q must have a non-empty Example: field (REQ-F-001 / REQ-F-015)", path)
			assert.False(t, strings.HasPrefix(firstLine, "  "),
				"command %q Example first line must be flush-left (no leading two-space indent); got %q", path, firstLine)
			assert.GreaterOrEqual(t, countExampleInvocations(c.Example), 2,
				"command %q Example must contain at least two invocations (each preceded by a `# comment` line per AGENTS.md); got Example=%q", path, c.Example)
			if c.Annotations["write"] != "true" {
				assert.True(t, strings.Contains(c.Example, "--format json"),
					"read command %q Example must contain at least one `--format json` invocation (AGENTS.md read-cmd gate); got Example=%q", path, c.Example)
			}
		}
		for _, sub := range c.Commands() {
			if sub.Hidden {
				continue
			}
			name := sub.Name()
			if name == "help" || name == "completion" {
				continue
			}
			walk(sub, path+" "+name)
		}
	}

	// Start the walk at root's children: the root command itself
	// (`neo4j-cli`) is not a runnable leaf and its Example is not required.
	for _, sub := range root.Commands() {
		if sub.Hidden {
			continue
		}
		name := sub.Name()
		if name == "help" || name == "completion" {
			continue
		}
		walk(sub, root.Name()+" "+name)
	}
}

// countExampleInvocations counts the number of `# comment` headers in a
// cobra Example block. AGENTS.md requires each invocation to be preceded by
// a `# comment` line with a blank-line separator, so the comment count
// equals the invocation count.
func countExampleInvocations(example string) int {
	n := 0
	for _, line := range strings.Split(example, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			n++
		}
	}
	return n
}
