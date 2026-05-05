// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package render walks a cobra command tree and produces a deterministic,
// agent-skill bundle: a top-level SKILL.md (frontmatter + overview + global
// flag table + subcommand index + gotchas) plus one references/<sub>.md per
// top-level subcommand (recursive flag tables and examples, TOC if >100
// lines).
//
// The frontmatter `version` field is the literal `{{VERSION}}` placeholder;
// the installer substitutes it with the runtime binary version when the
// bundle is written to an agent's skills directory. This lets a single
// generated bundle ship across releases without per-tag regeneration.
//
// Output is deterministic for unchanged input: subcommands and flag rows
// are sorted, hidden commands and the implicit `help` cmd are excluded,
// and inline byte composition avoids map iteration order.
package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// versionPlaceholder is the literal frontmatter value emitted by Bundle. The
// installer substitutes this with the runtime binary version on Install.
const versionPlaceholder = "{{VERSION}}"

// referenceTOCThreshold triggers a Contents TOC at the top of a reference
// file once its rendered body exceeds this number of lines. Matches the
// PRD's >100-line rule.
const referenceTOCThreshold = 100

// Options configures Bundle.
type Options struct {
	// Name is the skill name, embedded into the SKILL.md frontmatter.
	// Must be lowercase letters/digits/hyphens, ≤64 chars. Typically the
	// binary name (e.g. "neo4j-cli", "aura-cli").
	Name string

	// Description is the third-person frontmatter description (≤1024
	// chars). Sourced from each binary's description.txt.
	Description string

	// Additions is the raw markdown that appears under the SKILL.md
	// "Gotchas" heading. Sourced from each binary's additions.md. May be
	// empty.
	Additions string
}

// Bundle walks `root` and returns the in-memory bundle keyed by relative
// path with forward-slash separators (the embed.FS convention). Keys are:
//
//	"SKILL.md"
//	"references/<sub>.md"   one per visible top-level subcommand
//
// The output is byte-deterministic across runs for unchanged input.
func Bundle(root *cobra.Command, opts Options) (map[string][]byte, error) {
	if root == nil {
		return nil, fmt.Errorf("render: nil root command")
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("render: empty Options.Name")
	}
	if opts.Description == "" {
		return nil, fmt.Errorf("render: empty Options.Description")
	}

	subs := visibleSubcommands(root)

	out := make(map[string][]byte, 1+len(subs))
	out["SKILL.md"] = renderSkill(root, subs, opts)
	for _, sub := range subs {
		out["references/"+sub.Name()+".md"] = renderReference(root.Name(), sub)
	}
	return out, nil
}

// visibleSubcommands returns root's direct subcommands (sorted by name)
// excluding hidden cmds and the implicit `help` cmd.
func visibleSubcommands(root *cobra.Command) []*cobra.Command {
	all := root.Commands()
	subs := make([]*cobra.Command, 0, len(all))
	for _, c := range all {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		subs = append(subs, c)
	}
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	return subs
}

// renderSkill builds SKILL.md. Body sections:
//
//   - YAML frontmatter (name, description, version placeholder)
//   - Heading (root.Name) and short/long overview
//   - Global Flags table (root persistent flags)
//   - Subcommands index (links to references/<name>.md)
//   - Gotchas (Options.Additions verbatim)
func renderSkill(root *cobra.Command, subs []*cobra.Command, opts Options) []byte {
	var buf bytes.Buffer

	// Frontmatter — single-line description so naive regex parsers can
	// extract it without a YAML dep. Description must not contain a
	// newline; render.Bundle does not validate (caller's job), but
	// description.txt is single-paragraph by convention.
	buf.WriteString("---\n")
	fmt.Fprintf(&buf, "name: %s\n", opts.Name)
	fmt.Fprintf(&buf, "description: %s\n", strings.TrimSpace(opts.Description))
	fmt.Fprintf(&buf, "version: %s\n", versionPlaceholder)
	buf.WriteString("---\n\n")

	// Heading + overview.
	fmt.Fprintf(&buf, "# %s\n\n", root.Name())
	if s := strings.TrimSpace(root.Short); s != "" {
		buf.WriteString(s)
		buf.WriteString("\n\n")
	}
	if l := strings.TrimSpace(root.Long); l != "" && l != strings.TrimSpace(root.Short) {
		buf.WriteString(l)
		buf.WriteString("\n\n")
	}

	// Global flags from root's persistent flags.
	if hasVisibleFlags(root.PersistentFlags()) {
		buf.WriteString("## Global Flags\n\n")
		writeFlagsTable(&buf, root.PersistentFlags())
		buf.WriteString("\n")
	}

	// Subcommand index — links to references/<sub>.md.
	if len(subs) > 0 {
		buf.WriteString("## Subcommands\n\n")
		buf.WriteString("| Command | Description |\n")
		buf.WriteString("|---------|-------------|\n")
		for _, sub := range subs {
			fmt.Fprintf(&buf, "| [`%s`](references/%s.md) | %s |\n",
				sub.Name(), sub.Name(), escapePipes(strings.TrimSpace(sub.Short)))
		}
		buf.WriteString("\n")
	}

	// Gotchas — verbatim from additions.md, normalised to a single
	// trailing newline.
	buf.WriteString("## Gotchas\n\n")
	body := strings.TrimRight(opts.Additions, "\n")
	if body != "" {
		buf.WriteString(body)
		buf.WriteString("\n")
	}

	return buf.Bytes()
}

// renderReference emits one references/<sub>.md describing `sub` and every
// nested subcommand recursively. Adds a TOC at the top when the rendered
// body exceeds referenceTOCThreshold lines (per Anthropic best practices).
func renderReference(binName string, sub *cobra.Command) []byte {
	var body bytes.Buffer
	var headings []string

	walkRender(&body, &headings, binName, sub, 1)

	// Decide whether to prepend a TOC.
	lineCount := bytes.Count(body.Bytes(), []byte{'\n'})
	if lineCount <= referenceTOCThreshold {
		return body.Bytes()
	}

	var out bytes.Buffer
	// Insert "## Contents" + bulleted anchor links right after the H1.
	// The first heading is the sub's H1 (rendered by walkRender at depth
	// 1). Split the body so the TOC sits between H1 and the rest.
	bs := body.Bytes()
	// Find end of first line (the H1) and the following blank line.
	firstNL := bytes.IndexByte(bs, '\n')
	if firstNL < 0 {
		// Degenerate; just return raw body.
		return bs
	}
	out.Write(bs[:firstNL+1])
	out.WriteString("\n## Contents\n\n")
	// Skip the first heading (it's the H1, not a TOC entry).
	for _, h := range headings[1:] {
		fmt.Fprintf(&out, "- [%s](#%s)\n", h, slugify(h))
	}
	out.WriteString("\n")
	// rest = bs after the H1 line; trim a leading blank line if present
	// since we already wrote one before "## Contents".
	rest := bs[firstNL+1:]
	rest = bytes.TrimLeft(rest, "\n")
	out.Write(rest)
	return out.Bytes()
}

// walkRender renders cmd at heading depth `depth` and recurses into every
// visible subcommand at depth+1. Heading level caps at 6 (markdown limit);
// deeper nesting reuses H6 — not expected in practice.
func walkRender(buf *bytes.Buffer, headings *[]string, binName string, cmd *cobra.Command, depth int) {
	level := depth
	if level > 6 {
		level = 6
	}
	hashes := strings.Repeat("#", level)

	heading := commandPath(binName, cmd)
	*headings = append(*headings, heading)
	fmt.Fprintf(buf, "%s %s\n\n", hashes, heading)

	if s := strings.TrimSpace(cmd.Short); s != "" {
		buf.WriteString(s)
		buf.WriteString("\n\n")
	}
	if l := strings.TrimSpace(cmd.Long); l != "" && l != strings.TrimSpace(cmd.Short) {
		buf.WriteString(l)
		buf.WriteString("\n\n")
	}

	if u := strings.TrimSpace(cmd.UseLine()); u != "" {
		fmt.Fprintf(buf, "Usage: `%s`\n\n", u)
	}

	if hasVisibleFlags(cmd.LocalFlags()) {
		buf.WriteString("Flags:\n\n")
		writeFlagsTable(buf, cmd.LocalFlags())
		buf.WriteString("\n")
	}

	if e := strings.TrimSpace(cmd.Example); e != "" {
		buf.WriteString("Examples:\n\n")
		buf.WriteString("```\n")
		buf.WriteString(e)
		if !strings.HasSuffix(e, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString("```\n\n")
	}

	subs := visibleSubcommands(cmd)
	for _, c := range subs {
		walkRender(buf, headings, binName, c, depth+1)
	}
}

// commandPath returns the heading text for `cmd`: "<binName> <cmd path>".
// E.g. for "instance list" under "aura-cli": "aura-cli instance list".
func commandPath(binName string, cmd *cobra.Command) string {
	// CommandPath returns full path including root, e.g. "aura-cli instance list".
	cp := strings.TrimSpace(cmd.CommandPath())
	if cp != "" {
		return cp
	}
	return binName + " " + cmd.Name()
}

// hasVisibleFlags reports whether `fs` defines any non-hidden flag.
func hasVisibleFlags(fs *pflag.FlagSet) bool {
	found := false
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			found = true
		}
	})
	return found
}

// writeFlagsTable emits a deterministic markdown flag table to `buf`. Rows
// are sorted by long flag name. Pipe characters in usage strings are
// escaped so they don't break the table.
func writeFlagsTable(buf *bytes.Buffer, fs *pflag.FlagSet) {
	type row struct {
		name, shorthand, typ, def, usage string
	}
	var rows []row
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		rows = append(rows, row{
			name:      f.Name,
			shorthand: f.Shorthand,
			typ:       f.Value.Type(),
			def:       f.DefValue,
			usage:     escapePipes(f.Usage),
		})
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	buf.WriteString("| Flag | Type | Default | Description |\n")
	buf.WriteString("|------|------|---------|-------------|\n")
	for _, r := range rows {
		flag := "--" + r.name
		if r.shorthand != "" {
			flag = "-" + r.shorthand + ", " + flag
		}
		def := r.def
		if def == "" {
			def = "-"
		}
		fmt.Fprintf(buf, "| `%s` | %s | %s | %s |\n", flag, r.typ, def, r.usage)
	}
}

// escapePipes escapes `|` in a markdown table cell so it doesn't terminate
// the column.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// slugify produces a GitHub-style markdown anchor slug: lowercase, spaces
// replaced with hyphens, non [a-z0-9-] stripped. Stable for byte-equal
// golden-file tests.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
