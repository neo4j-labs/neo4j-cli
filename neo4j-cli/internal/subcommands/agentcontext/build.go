// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package agentcontext builds a structured, machine-readable description of
// the neo4j-cli command tree for AI-agent discovery (Layer 2 per
// agent-cli-auditor.md §7.2).
//
// The walker is intentionally a pure function over a cobra tree — no cobra
// side effects, no I/O. It mirrors the flag-iteration pattern in
// common/skill/render/render.go (VisitAll over LocalFlags + InheritedFlags,
// skip Hidden, sort by name) but emits typed struct fields instead of
// markdown rows.
//
// The recursive `commands` tree is reflected at runtime — adding a new
// subcommand, flag, or alias auto-surfaces. The small surface that cannot
// be reflected (exit codes, error categories, async-flag name, schema
// version) is hand-coded below and locked by tests in agentcontext_test.go.
package agentcontext

import (
	"sort"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// schemaVersion is the integer version of the emitted JSON envelope.
// Bump on any breaking shape change: renaming a top-level key, changing a
// field's Go type, dropping a documented exit/error code, or changing the
// recursion structure of `commands`. See AGENTS.md "Agent Context Notes".
const schemaVersion = 1

// asyncFlag is the canonical async-flag name in this repo. v1 emits
// "--await" (honest to the actual flag); renaming to "--wait" is a separate
// audit item.
const asyncFlag = "--await"

// maxDepth bounds recursion defensively. The repo's command tree does not
// exceed depth 4 today; the bound is purely a guardrail.
const maxDepth = 10

// exitCodes documents the integer exit codes the binary actually emits.
// Mirrors the closed set defined in common/clierr/error.go and the
// agent-cli-auditor.md §4.1 table. Keep this in sync with the
// constructors over there. Bump schemaVersion when removing an entry.
var exitCodes = map[string]string{
	"0": "success",
	"1": "general error",
	"2": "usage error: bad flag, missing argument, or malformed invocation",
	"3": "not found: resource doesn't exist",
	"4": "auth error: authentication or authorization failed",
	"5": "conflict: request conflicts with current resource state",
	"6": "validation error: input payload rejected by validation",
	"7": "rate limited: upstream signalled rate limit",
	"8": "upstream error: transient API failure; retry may succeed",
}

// errorCodes mirrors the constructors in common/clierr/error.go. Each
// `error.code` string maps 1:1 to a process exit code in `exitCodes`.
// Wording matches REQ-F-009. Adding a new constructor over there means
// adding an entry here.
var errorCodes = map[string]string{
	"fatal_error":      "unrecoverable internal failure",
	"usage_error":      "invalid flag, missing argument, or other input rejection",
	"not_found":        "resource doesn't exist",
	"auth_error":       "authentication or authorization failed",
	"conflict":         "request conflicts with current resource state",
	"validation_error": "input payload rejected by validation",
	"rate_limited":     "upstream signalled rate limit; retry after the hinted delay",
	"upstream_error":   "transient API failure; retry may succeed",
}

// Context is the top-level JSON envelope. Field order in the struct
// matches the documented envelope shape.
type Context struct {
	SchemaVersion int                `json:"schema_version"`
	CliVersion    string             `json:"cli_version"`
	Binary        string             `json:"binary"`
	Commands      map[string]Command `json:"commands"`
	ExitCodes     map[string]string  `json:"exit_codes"`
	ErrorCodes    map[string]string  `json:"error_codes"`
	OutputFormats []string           `json:"output_formats"`
	AsyncFlag     string             `json:"async_flag"`
}

// Command is one node in the recursive command tree.
type Command struct {
	Use         string             `json:"use"`
	Short       string             `json:"short"`
	Long        string             `json:"long"`
	Example     string             `json:"example"`
	Aliases     []string           `json:"aliases"`
	Hidden      bool               `json:"hidden"`
	Deprecated  string             `json:"deprecated"`
	Flags       []Flag             `json:"flags"`
	Subcommands map[string]Command `json:"subcommands"`
}

// Flag is one row in the flag table for a Command.
type Flag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Inherited   bool   `json:"inherited"`
}

// BuildContext walks the cobra tree rooted at `root` and returns the full
// JSON envelope. `cliVersion` is supplied by the caller (typically
// app.Version) to keep this package free of an import cycle on app.
func BuildContext(root *cobra.Command, cliVersion string) Context {
	return Context{
		SchemaVersion: schemaVersion,
		CliVersion:    cliVersion,
		Binary:        "neo4j-cli",
		Commands:      walkChildren(root, 0),
		ExitCodes:     exitCodes,
		ErrorCodes:    errorCodes,
		OutputFormats: clicfg.ValidFormatValues[:],
		AsyncFlag:     asyncFlag,
	}
}

// walkChildren returns the child-command map for `parent`. Hidden /
// non-available commands (including cobra's auto-generated `help`) are
// skipped via IsAvailableCommand. Keys are the first-Use-token lowercased
// so callers can index by command name without parsing Use themselves.
func walkChildren(parent *cobra.Command, depth int) map[string]Command {
	out := map[string]Command{}
	if depth >= maxDepth {
		return out
	}
	for _, sub := range parent.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		out[firstToken(sub.Use)] = walkCommand(sub, depth+1)
	}
	return out
}

// walkCommand builds a Command node for `cmd`, recursing into its
// subcommands. Aliases is normalised to a non-nil slice so the emitted
// JSON has a stable `"aliases": []` rather than `null`.
func walkCommand(cmd *cobra.Command, depth int) Command {
	aliases := append([]string{}, cmd.Aliases...)
	return Command{
		Use:         cmd.Use,
		Short:       cmd.Short,
		Long:        cmd.Long,
		Example:     cmd.Example,
		Aliases:     aliases,
		Hidden:      false, // hidden commands are filtered upstream; field is kept for forward compatibility
		Deprecated:  cmd.Deprecated,
		Flags:       collectFlags(cmd),
		Subcommands: walkChildren(cmd, depth),
	}
}

// collectFlags returns every visible flag on `cmd` — local first, then
// inherited (parent-persistent) flags the locals didn't redefine.
// Inherited flags carry inherited:true so an agent inspecting a single
// subcommand sees the full effective flag set without walking parents.
//
// Mirrors common/skill/render/render.go:281-293: VisitAll, skip Hidden,
// sort by name. The local-wins dedupe matches cobra's own runtime
// behaviour when a child redeclares a parent's flag.
func collectFlags(cmd *cobra.Command) []Flag {
	seen := map[string]bool{}
	rows := []Flag{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		rows = append(rows, mkFlag(f, false))
		seen[f.Name] = true
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || seen[f.Name] {
			return
		}
		rows = append(rows, mkFlag(f, true))
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// mkFlag converts a pflag.Flag into our serialisable Flag row.
func mkFlag(f *pflag.Flag, inherited bool) Flag {
	return Flag{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Default:     f.DefValue,
		Description: f.Usage,
		Inherited:   inherited,
	}
}

// firstToken returns the lowercased first whitespace-delimited token of a
// cobra `Use` string (e.g. "list [flags]" → "list"). Empty Use returns "".
func firstToken(use string) string {
	use = strings.TrimSpace(use)
	if use == "" {
		return ""
	}
	if i := strings.IndexAny(use, " \t"); i >= 0 {
		use = use[:i]
	}
	return strings.ToLower(use)
}
