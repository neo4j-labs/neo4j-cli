// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agentcontext_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// kebabCase matches a lower-case kebab identifier: one or more lower-case
// alphanumeric segments joined by single hyphens (e.g. `instance`, `bolt-port`).
var kebabCase = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// TestInputIdentifiers_AreKebabCase walks the live neo4j-cli cobra tree from
// app.NewCmd(cfg) and asserts every command name, command alias, and flag long
// name is kebab-case. Single-character flag shorthands are exempt from the
// multi-segment shape (they are matched by pflag's Shorthand, not Name).
//
// The current tree already complies; this gate guards future additions (a
// stray `--bad_flag` or a snake/camel command name fails with the offending
// path / identifier named). Every feature flag is enabled so flag-gated
// subtrees are covered too.
func TestInputIdentifiers_AreKebabCase(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)

	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		for _, alias := range c.Aliases {
			assert.Regexp(t, kebabCase, alias,
				"command %q alias %q must be kebab-case (^[a-z][a-z0-9]*(-[a-z0-9]+)*$)", path, alias)
		}

		checkFlag := func(f *pflag.Flag) {
			assert.Regexp(t, kebabCase, f.Name,
				"command %q flag --%s must be kebab-case (^[a-z][a-z0-9]*(-[a-z0-9]+)*$)", path, f.Name)
		}
		c.LocalFlags().VisitAll(checkFlag)
		c.PersistentFlags().VisitAll(checkFlag)

		for _, sub := range c.Commands() {
			name := strings.Fields(sub.Use)[0]
			// cobra injects `help` / `completion` subtrees on Execute; the
			// completion subtree carries snake-ish generated names we don't own.
			if name == "help" || name == "completion" {
				continue
			}
			// `query :schema` / `query :embed` are intentional REPL-style
			// meta-commands; the leading colon is a deliberate naming
			// convention, the remainder is still kebab-case.
			checkName := strings.TrimPrefix(name, ":")
			assert.Regexp(t, kebabCase, checkName,
				"command %q subcommand %q must be kebab-case (^[a-z][a-z0-9]*(-[a-z0-9]+)*$)", path, name)
			walk(sub, path+" "+name)
		}
	}

	walk(root, root.Name())
}
