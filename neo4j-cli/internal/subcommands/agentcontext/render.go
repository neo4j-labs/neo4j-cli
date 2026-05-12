// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agentcontext

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	toon "github.com/toon-format/toon-go"
)

// renderToon emits the envelope as TOON via the existing marshal-via-JSON
// pattern in common/output (json.Marshal -> any -> toon.Marshal). The double
// hop is needed because toon.Marshal walks a plain `any` shape, and going via
// JSON honours any custom MarshalJSON impls along the way.
func renderToon(cmd *cobra.Command, ctx Context) error {
	b, err := json.Marshal(ctx)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	out, err := toon.Marshal(v, toon.WithLengthMarkers(true))
	if err != nil {
		return err
	}
	cmd.Println(string(out))
	return nil
}

// renderTable emits a degraded flat command-list with columns
// path | aliases | short, sorted by command path, plus a footer of
// cli_version / schema_version / async_flag. The nested commands tree is
// flattened so a human scanning the table sees every visible subcommand at
// once, not just the top-level entries.
func renderTable(cmd *cobra.Command, ctx Context) error {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"path", "aliases", "short"})
	rows := flattenCommands(ctx.Binary, ctx.Commands)
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	for _, r := range rows {
		t.AppendRow(table.Row{r[0], r[1], r[2]})
	}
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
	cmd.Println(fmt.Sprintf("cli_version: %s", ctx.CliVersion))
	cmd.Println(fmt.Sprintf("schema_version: %d", ctx.SchemaVersion))
	cmd.Println(fmt.Sprintf("async_flag: %s", ctx.AsyncFlag))
	return nil
}

// flattenCommands walks the recursive Commands map and returns one row per
// visible command as [path, aliases, short]. `prefix` is the parent path
// already built (e.g. "neo4j-cli aura"). Map keys are first-Use tokens so
// the path is reconstructed by joining keys with spaces.
func flattenCommands(prefix string, cmds map[string]Command) [][3]string {
	rows := [][3]string{}
	keys := make([]string, 0, len(cmds))
	for k := range cmds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := cmds[k]
		path := prefix + " " + k
		rows = append(rows, [3]string{path, strings.Join(c.Aliases, ","), c.Short})
		rows = append(rows, flattenCommands(path, c.Subcommands)...)
	}
	return rows
}
