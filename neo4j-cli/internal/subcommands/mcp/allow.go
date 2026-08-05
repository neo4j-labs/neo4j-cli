// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"strconv"
	"strings"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

// Policy is the MCP execution policy for one CLI invocation. Every invocation
// resolves to exactly one policy; anything the table below does not cover is
// denied.
type Policy string

const (
	// PolicyAllow may be executed by the read-only tool.
	PolicyAllow Policy = "allow"
	// PolicyWrite may be executed only by the write tool.
	PolicyWrite Policy = "write"
	// PolicyGatedAura additionally requires --allow-aura.
	PolicyGatedAura Policy = "gated:allow-aura"
	// PolicyGatedCredentialWrite additionally requires --allow-credential-write.
	PolicyGatedCredentialWrite Policy = "gated:allow-credential-write"
	// PolicyDeny is never executable over MCP.
	PolicyDeny Policy = "deny"
)

// Flag names for the two opt-in gates.
const (
	AllowAuraFlag            = "allow-aura"
	AllowCredentialWriteFlag = "allow-credential-write"
)

// Gates records which gated policies the operator opted into when starting the
// server. All default false. WriteAllowed is the process gate for the write
// tool (layer 1); the other two control gated policies (layer 2).
type Gates struct {
	AllowAura            bool
	AllowCredentialWrite bool
	WriteAllowed         bool
}

// deniedPaths are subtrees that are never executable over MCP, matched as
// command-path prefixes: `update` swaps the running binary out from under the
// server, `mcp` would spawn a server inside a server, `completion` emits shell
// code for a shell the model does not have, and `history clear` destroys the
// audit trail of what the model did.
var deniedPaths = [][]string{
	{"update"},
	{"mcp"},
	{"completion"},
	{"history", "clear"},
}

// deniedArgs deny a single leaf only when a specific token appears among its
// arguments, for leaves whose danger lives in the argument rather than the
// path. `config set credential-storage insecure` silently rewrites every
// stored secret from the OS keyring into plaintext on disk.
var deniedArgs = []struct {
	path []string
	arg  string
}{
	{path: []string{"config", "set"}, arg: "credential-storage"},
}

// writeArgs escalate a leaf that carries no write annotation but whose
// arguments make it a write. `query` is annotated read: it gates writes on its
// own --rw flag at execution time, so without this rule the read-only tool
// could run `query --rw "MATCH (n) DETACH DELETE n"`. Keeping it here makes the
// table self-sufficient instead of leaning on the tool layer's own --rw
// rejection.
var writeArgs = []struct {
	path []string
	flag string
}{
	{path: []string{"query"}, flag: "rw"},
}

// gatedAuraPaths are Aura subtrees gated in full, reads included: every
// `aura agent` call bills for model tokens, and customer-managed keys can
// render an instance permanently unreadable.
var gatedAuraPaths = [][]string{
	{"aura", "agent"},
	{"aura", "customer-managed-key"},
}

// exposedPaths are the top-level trees the MCP surface exposes at all. A tree
// absent from this list is denied even if its leaves are otherwise harmless,
// so a newly added tree has to be classified deliberately rather than
// inheriting a default of "reachable" (the full-tree gate in allow_test.go
// fails until it is listed).
//
// Listing a whole tree does not expose its writes: writeGatedPaths and the
// `write` policy intercept every write-annotated leaf below, and deniedPaths /
// deniedArgs / gatedAuraPaths are consulted first. So `credential` here means
// the `list` leaves, `config` means `get`/`list`, and `aura` means the
// `list`/`get` leaves.
//
// This table does NOT protect against leaves that read os.Stdin for an
// interactive password prompt (`query` with no argument, `desktop dbms create`,
// …). Those are neutralised by the server's stdio layer, which points stdin at
// os.DevNull so the prompt gets EOF instead of hanging; do not try to fix that
// here.
var exposedPaths = [][]string{
	{"admin"},
	{"agent-context"},
	{"aura"},
	{"config"},
	{"credential"},
	{"dataset"},
	{"desktop"},
	{"docker"},
	{"history"},
	{"query"},
	{"skill"},
}

// writeGatedPaths escalate a write from `write` to a gated policy. Keyed on the
// path rather than on an enumerated leaf list so a new provisioning or
// credential-minting leaf is gated the day it is added: every Aura mutation
// spends real money, and every credential mutation mints or destroys an OS
// keyring entry.
//
// The LONGEST matching prefix wins, not the first. The aura root mounts its own
// `credential` subtree in-process; were it ever shipped under `aura`,
// first-match ordering would put `aura credential add` behind --allow-aura, so
// a consent to spend money would silently also consent to a keyring write.
var writeGatedPaths = []struct {
	path   []string
	policy Policy
}{
	{path: []string{"aura"}, policy: PolicyGatedAura},
	{path: []string{"credential"}, policy: PolicyGatedCredentialWrite},
	{path: []string{"aura", "credential"}, policy: PolicyGatedCredentialWrite},
}

// Classify returns the policy for running cmd with args, and whether that
// policy came from an explicit rule. Callers deciding whether to execute want
// only the policy; the second value exists because Classify denies whatever it
// does not recognise, and that silent default is what must not accumulate as
// the command tree grows (the full-tree gate test asserts it never does).
//
// args matter only to the argument-keyed rules; pass nil to classify a path.
func Classify(cmd *cobra.Command, args []string) (Policy, bool) {
	return classify(commandPath(cmd), flags.IsWriteCommand(cmd), args)
}

// Check returns nil when cmd may be executed over MCP under gates, or a usage
// error naming the flag that would permit it.
//
// PolicyWrite is not refused here: choosing between the read-only and write
// tools is the tool layer's job, and flags.EnforceWriteGate inside Execute()
// stays the authoritative write gate.
func Check(cmd *cobra.Command, args []string, gates Gates) error {
	path := strings.Join(commandPath(cmd), " ")
	policy, _ := Classify(cmd, args)
	switch policy {
	case PolicyDeny:
		return clierr.NewUsageError("`%s` cannot be run over MCP; run it yourself in a terminal", path)
	case PolicyGatedAura:
		if !gates.AllowAura {
			return clierr.NewUsageError("`%s` provisions Aura resources that cost money; restart the server with `neo4j-cli mcp serve --%s` to allow it", path, AllowAuraFlag)
		}
	case PolicyGatedCredentialWrite:
		if !gates.AllowCredentialWrite {
			return clierr.NewUsageError("`%s` changes stored credentials; restart the server with `neo4j-cli mcp serve --%s` to allow it", path, AllowCredentialWriteFlag)
		}
	}
	return nil
}

// RegisterGateFlags adds the two opt-in gate flags to a command. Now only used
// by tests; serve.go adds its own local flags via addServeFlags.
func RegisterGateFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(AllowAuraFlag, false,
		"Let MCP clients provision, modify and delete Aura resources, which costs money")
	cmd.PersistentFlags().Bool(AllowCredentialWriteFlag, false,
		"Let MCP clients add, remove and select stored credentials in the OS keyring")
}

// GatesFromCommand reads the two opt-in gate flags (--allow-aura and
// --allow-credential-write) off cmd, local or inherited. It does NOT populate
// WriteAllowed — that flag (--rw) belongs to the serve command and is set
// separately by task-013's serve handler. A missing flag is an error rather
// than a closed gate, so a caller that forgot RegisterGateFlags fails loudly
// instead of silently refusing every gated call.
func GatesFromCommand(cmd *cobra.Command) (Gates, error) {
	aura, err := gateFlagValue(cmd, AllowAuraFlag)
	if err != nil {
		return Gates{}, err
	}
	credentialWrite, err := gateFlagValue(cmd, AllowCredentialWriteFlag)
	if err != nil {
		return Gates{}, err
	}
	return Gates{AllowAura: aura, AllowCredentialWrite: credentialWrite}, nil
}

// gateFlagValue resolves one gate flag. It goes through cmd.Flag rather than
// cmd.Flags().GetBool because the latter only sees a parent's persistent flags
// once cobra has parsed them, which a caller holding a freshly built tree has
// not yet done (same lookup flags.EnforceWriteGate uses for --rw).
func gateFlagValue(cmd *cobra.Command, name string) (bool, error) {
	flag := cmd.Flag(name)
	if flag == nil {
		return false, clierr.NewFatalError("MCP gate flag --%s is not registered on `%s`", name, cmd.CommandPath())
	}
	return strconv.ParseBool(flag.Value.String())
}

// classify resolves one invocation against the table. The second return value
// is false when no rule matched, i.e. when the returned PolicyDeny is the
// default rather than a decision.
//
// Precedence is deny > gated > write > allow, so a path that appears in a
// static list AND carries the write annotation resolves by the list: `history
// clear` is denied rather than written, and `aura instance create` is gated
// rather than merely written.
func classify(path []string, annotatedWrite bool, args []string) (Policy, bool) {
	for _, denied := range deniedPaths {
		if hasPathPrefix(path, denied) {
			return PolicyDeny, true
		}
	}
	for _, denied := range deniedArgs {
		if hasPathPrefix(path, denied.path) && containsToken(args, denied.arg) {
			return PolicyDeny, true
		}
	}

	if !matchesAny(path, exposedPaths) {
		return PolicyDeny, false
	}

	for _, gated := range gatedAuraPaths {
		if hasPathPrefix(path, gated) {
			return PolicyGatedAura, true
		}
	}

	if annotatedWrite || escalatedByArgs(path, args) {
		policy := PolicyWrite
		matched := 0
		for _, gated := range writeGatedPaths {
			if hasPathPrefix(path, gated.path) && len(gated.path) > matched {
				policy, matched = gated.policy, len(gated.path)
			}
		}
		return policy, true
	}

	return PolicyAllow, true
}

// escalatedByArgs reports whether a writeArgs rule applies to this invocation.
func escalatedByArgs(path []string, args []string) bool {
	for _, rule := range writeArgs {
		if hasPathPrefix(path, rule.path) && containsFlag(args, rule.flag) {
			return true
		}
	}
	return false
}

// commandPath returns cmd's path as tokens with the root binary name dropped,
// e.g. ["aura", "instance", "create"]. The root itself yields an empty slice.
func commandPath(cmd *cobra.Command) []string {
	var path []string
	for c := cmd; c != nil && c.Parent() != nil; c = c.Parent() {
		path = append([]string{c.Name()}, path...)
	}
	return path
}

// hasPathPrefix reports whether prefix matches path token-for-token. An empty
// path (the root command) matches nothing.
func hasPathPrefix(path, prefix []string) bool {
	if len(path) == 0 || len(prefix) > len(path) {
		return false
	}
	for i, token := range prefix {
		if path[i] != token {
			return false
		}
	}
	return true
}

// matchesAny reports whether any of prefixes matches path.
func matchesAny(path []string, prefixes [][]string) bool {
	for _, prefix := range prefixes {
		if hasPathPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// containsToken reports whether token appears among args in any spelling that
// could reach a command: bare, as either half of `x=y`, dash-prefixed, or
// dot-namespaced (`aura.credential-storage`). Matching is deliberately loose so
// an argument-keyed deny cannot be defeated by a spelling the argument parser
// happens to accept later — over-matching only refuses more.
func containsToken(args []string, token string) bool {
	for _, arg := range args {
		for _, candidate := range argSpellings(arg) {
			if strings.EqualFold(candidate, token) {
				return true
			}
		}
	}
	return false
}

// containsFlag reports whether args carry the named flag in any spelling
// (`--rw`, `-rw`, `--rw=false`). A negated value still counts: routing the call
// to the write tool is the conservative answer.
func containsFlag(args []string, name string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		flagName, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.EqualFold(strings.TrimSpace(flagName), name) {
			return true
		}
	}
	return false
}

// argSpellings expands one argument into the tokens a rule may match against.
func argSpellings(arg string) []string {
	arg = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(arg), "-"))
	parts := []string{arg}
	if key, value, found := strings.Cut(arg, "="); found {
		parts = append(parts, key, value)
	}
	initialLen := len(parts)
	for _, part := range parts[:initialLen] {
		if i := strings.LastIndex(part, "."); i >= 0 {
			parts = append(parts, part[i+1:])
		}
	}
	return parts
}
