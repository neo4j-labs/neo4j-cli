// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	commonflags "github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateGolden regenerates testdata/policy.golden. Mirrors the -update flag in
// common/skill/render's golden tests.
var updateGolden = flag.Bool("update", false, "regenerate the MCP policy golden file")

// TestClassify_LiveTreePaths pins one representative path per policy against
// the real command tree, so a leaf that is renamed or re-annotated upstream
// shows up here rather than silently changing policy.
func TestClassify_LiveTreePaths(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	for _, tc := range []struct {
		path string
		args []string
		want mcp.Policy
	}{
		{path: "docker list", want: mcp.PolicyAllow},
		{path: "query :schema", want: mcp.PolicyAllow},
		{path: "query", args: []string{"MATCH (n) RETURN n"}, want: mcp.PolicyAllow},
		{path: "credential dbms list", want: mcp.PolicyAllow},
		{path: "aura instance list", want: mcp.PolicyAllow},
		{path: "config get", want: mcp.PolicyAllow},
		{path: "history list", want: mcp.PolicyAllow},
		{path: "agent-context", want: mcp.PolicyAllow},

		{path: "docker create", want: mcp.PolicyWrite},
		{path: "admin database create", want: mcp.PolicyWrite},
		{path: "skill install", want: mcp.PolicyWrite},
		{path: "config set", args: []string{"format", "json"}, want: mcp.PolicyWrite},
		// query is annotated read and gates writes on its own --rw at
		// execution time, so the flag has to escalate it here.
		{path: "query", args: []string{"--rw", "MATCH (n) DETACH DELETE n"}, want: mcp.PolicyWrite},
		{path: "query", args: []string{"--rw=true", "CREATE (n)"}, want: mcp.PolicyWrite},

		{path: "aura instance create", want: mcp.PolicyGatedAura},
		{path: "aura instance delete", want: mcp.PolicyGatedAura},
		{path: "aura instance overwrite", want: mcp.PolicyGatedAura},
		{path: "aura instance update", want: mcp.PolicyGatedAura},
		{path: "aura graph-analytics session create", want: mcp.PolicyGatedAura},
		{path: "aura agent invoke", want: mcp.PolicyGatedAura},
		{path: "aura agent list", want: mcp.PolicyGatedAura},
		{path: "aura customer-managed-key list", want: mcp.PolicyGatedAura},

		{path: "credential dbms add", want: mcp.PolicyGatedCredentialWrite},
		{path: "credential dbms use", want: mcp.PolicyGatedCredentialWrite},
		{path: "credential dbms set-embed", want: mcp.PolicyGatedCredentialWrite},
		{path: "credential aura-client remove", want: mcp.PolicyGatedCredentialWrite},

		{path: "update", want: mcp.PolicyDeny},
		{path: "update check", want: mcp.PolicyDeny},
		{path: "mcp tool", want: mcp.PolicyDeny},
		{path: "history clear", want: mcp.PolicyDeny},
		// The keyring→plaintext downgrade must stay denied in every
		// spelling an argument parser could plausibly accept, not just the
		// one `config set` accepts today.
		{path: "config set", args: []string{"credential-storage", "insecure"}, want: mcp.PolicyDeny},
		{path: "config set", args: []string{"credential-storage=insecure"}, want: mcp.PolicyDeny},
		{path: "config set", args: []string{"aura.credential-storage", "insecure"}, want: mcp.PolicyDeny},
		{path: "config set", args: []string{"--key=credential-storage", "--value=insecure"}, want: mcp.PolicyDeny},
	} {
		name := tc.path
		if len(tc.args) > 0 {
			name += " " + strings.Join(tc.args, " ")
		}
		t.Run(name, func(t *testing.T) {
			cmd := findCommand(t, root, tc.path)
			policy, explicit := mcp.Classify(cmd, tc.args)
			assert.Equal(t, tc.want, policy)
			assert.True(t, explicit, "must be an explicit rule, not the default deny")
		})
	}
}

// TestClassify_Precedence covers the paths that match a static list AND carry
// Annotations["write"]="true". The static list must win, deterministically:
// otherwise `history clear` would be reachable through the write tool and
// `aura instance create` would skip its gate.
func TestClassify_Precedence(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	for _, tc := range []struct {
		path string
		want mcp.Policy
	}{
		{path: "history clear", want: mcp.PolicyDeny},
		{path: "aura instance create", want: mcp.PolicyGatedAura},
		{path: "aura agent create", want: mcp.PolicyGatedAura},
		{path: "credential embed add", want: mcp.PolicyGatedCredentialWrite},
	} {
		t.Run(tc.path, func(t *testing.T) {
			cmd := findCommand(t, root, tc.path)
			require.True(t, commonflags.IsWriteCommand(cmd),
				"precedence case is only meaningful while %q is write-annotated", tc.path)
			policy, _ := mcp.Classify(cmd, nil)
			assert.Equal(t, tc.want, policy)
		})
	}
}

// TestClassify_MostSpecificWriteGateWins pins the within-list precedence: a
// credential write nested under `aura` must land on the credential gate, so
// consenting to Aura spend never also consents to a keyring write. The aura
// root mounts its own `credential` subtree in-process, so this is one refactor
// away from being a live path.
func TestClassify_MostSpecificWriteGateWins(t *testing.T) {
	cmd := syntheticCmd(true, "aura", "credential", "add")
	policy, explicit := mcp.Classify(cmd, nil)
	assert.Equal(t, mcp.PolicyGatedCredentialWrite, policy)
	assert.True(t, explicit)
}

// TestClassify_CompletionIsDenied covers the one denied path the live tree
// cannot supply: cobra injects `completion` at Execute() time.
func TestClassify_CompletionIsDenied(t *testing.T) {
	cmd := syntheticCmd(false, "completion", "zsh")
	policy, explicit := mcp.Classify(cmd, nil)
	assert.Equal(t, mcp.PolicyDeny, policy)
	assert.True(t, explicit)
}

// TestClassify_UnclassifiedDefaultsToDeny is the safety property the whole file
// exists for: a tree nobody classified is refused, and the write annotation
// does not launder it into the write policy.
func TestClassify_UnclassifiedDefaultsToDeny(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write bool
	}{
		{name: "read", write: false},
		{name: "write-annotated", write: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := syntheticCmd(tc.write, "frobnicate", "widget")
			policy, explicit := mcp.Classify(cmd, nil)
			assert.Equal(t, mcp.PolicyDeny, policy)
			assert.False(t, explicit,
				"an unlisted tree must report as unclassified so the gate test fails")
		})
	}
}

// TestClassify_RootIsDenied guards the empty-path edge: the bare binary starts
// no command and must not match a prefix rule.
func TestClassify_RootIsDenied(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	policy, explicit := mcp.Classify(root, nil)
	assert.Equal(t, mcp.PolicyDeny, policy)
	assert.False(t, explicit)
}

// TestPolicyTable_ClassifiesEveryCommandPath fails on any command the table
// does not classify explicitly, so a new top-level tree cannot quietly land on
// the default deny — and, because the write policy is derived from the
// annotation, cannot launder itself into the write tool either.
//
// It says nothing about a new leaf under an already-exposed tree; that is what
// TestPolicyTable_Golden below is for.
func TestPolicyTable_ClassifiesEveryCommandPath(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	var unclassified []string
	for _, cmd := range executableCommands(root) {
		if _, explicit := mcp.Classify(cmd, nil); !explicit {
			unclassified = append(unclassified, strings.Join(commandPathOf(cmd), " "))
		}
	}

	assert.Empty(t, unclassified,
		"every command path must be classified in mcp/allow.go; add these to a policy list (deny, gated, exposed) rather than leaving them on the default deny: %v",
		unclassified)
}

// TestPolicyTable_Golden is the forcing function for leaves added under a tree
// that is ALREADY exposed: those classify silently (allow for a read, write for
// an annotated write), so only a committed snapshot makes a reviewer look. A
// diff here means "decide what an MCP client may do with this command", and is
// resolved by regenerating with `go test ./neo4j-cli/internal/subcommands/mcp -update`.
func TestPolicyTable_Golden(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	var lines []string
	for _, cmd := range executableCommands(root) {
		policy, _ := mcp.Classify(cmd, nil)
		lines = append(lines, fmt.Sprintf("%s\t%s", strings.Join(commandPathOf(cmd), " "), policy))
	}
	sort.Strings(lines)
	got := []byte(strings.Join(lines, "\n") + "\n")

	path := filepath.Join("testdata", "policy.golden")
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, got, 0644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read %s (regenerate with `go test ./neo4j-cli/internal/subcommands/mcp -update`)", path)
	if !assert.Equal(t, string(want), string(got), "the MCP policy of at least one command changed") {
		t.Log("hint: if the new classification is intended, regenerate with `go test ./neo4j-cli/internal/subcommands/mcp -update`")
	}
}

func TestCheck_Gates(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	for _, tc := range []struct {
		name        string
		path        string
		args        []string
		gates       mcp.Gates
		wantErr     bool
		wantContain string
	}{
		{name: "allow needs no gate", path: "docker list"},
		{name: "write needs no gate here", path: "docker create"},
		{
			name:        "aura provisioning refused by default",
			path:        "aura instance create",
			wantErr:     true,
			wantContain: "--allow-aura",
		},
		{
			name:  "aura provisioning permitted when opted in",
			path:  "aura instance create",
			gates: mcp.Gates{AllowAura: true},
		},
		{
			name:        "credential write refused by default",
			path:        "credential dbms add",
			wantErr:     true,
			wantContain: "--allow-credential-write",
		},
		{
			name:  "credential write permitted when opted in",
			path:  "credential dbms add",
			gates: mcp.Gates{AllowCredentialWrite: true},
		},
		{
			name:        "the aura gate does not open the credential gate",
			path:        "credential dbms add",
			gates:       mcp.Gates{AllowAura: true},
			wantErr:     true,
			wantContain: "--allow-credential-write",
		},
		{
			name:        "denied path stays denied with both gates open",
			path:        "history clear",
			gates:       mcp.Gates{AllowAura: true, AllowCredentialWrite: true},
			wantErr:     true,
			wantContain: "cannot be run over MCP",
		},
		{
			name:        "credential-storage downgrade stays denied",
			path:        "config set",
			args:        []string{"credential-storage", "insecure"},
			gates:       mcp.Gates{AllowAura: true, AllowCredentialWrite: true},
			wantErr:     true,
			wantContain: "cannot be run over MCP",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := mcp.Check(findCommand(t, root, tc.path), tc.args, tc.gates)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantContain)
		})
	}
}

// TestGateFlags_RegisteredOnServe checks both gate flags exist on the serve
// command, default to false, and are readable. The flags were moved from the
// mcp parent to serve in task-013.
func TestGateFlags_RegisteredOnServe(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	serve := findSubcommand(findSubcommand(root, "mcp"), "serve")
	require.NotNil(t, serve)

	for _, name := range []string{"allow-aura", "allow-credential-write"} {
		flag := serve.Flags().Lookup(name)
		require.NotNil(t, flag, "--%s must be registered on serve", name)
		assert.Equal(t, "false", flag.DefValue, "--%s must be opt-in", name)
	}

	gates, err := mcp.GatesFromCommand(serve)
	require.NoError(t, err)
	assert.Equal(t, mcp.Gates{}, gates, "both gates closed before any flag is set")

	require.NoError(t, serve.Flags().Set("allow-aura", "true"))
	gates, err = mcp.GatesFromCommand(serve)
	require.NoError(t, err)
	assert.Equal(t, mcp.Gates{AllowAura: true}, gates)
}

// TestGatesFromCommand_MissingFlagErrors pins the fail-loudly choice: a caller
// that never registered the flags gets an error, not silently closed gates.
func TestGatesFromCommand_MissingFlagErrors(t *testing.T) {
	_, err := mcp.GatesFromCommand(&cobra.Command{Use: "bare"})
	assert.Error(t, err)
}
