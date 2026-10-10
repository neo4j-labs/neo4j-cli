// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd

import (
	"encoding/json"
	"fmt"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/neo4j/cli/internal/desktopclient"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newDeleteCmd generates the `<resource> delete <resource>` leaf. The
// positional accepts a UUID or an exact name (resolved via the spec's Resolve
// hook); the resolved id rides in the DELETE querystring. The destructive
// action is gated by the shared confirm helper on the RAW positional BEFORE any
// catalog lookup, so non-TTY callers without `--yes --force` fail fast without
// touching Desktop and the TTY prompt echoes what the user typed.
func newDeleteCmd[T any, S any](cfg *clicfg.Config, spec Spec[T, S]) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <" + spec.Noun + ">",
		Short:       "Delete a " + spec.Noun + " from the local Neo4j Desktop 2 install",
		Long:        spec.DeleteLong,
		Example:     spec.DeleteExample,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			if err := confirm.Require(cmd, args[0]); err != nil {
				return err
			}

			client, err := desktopclient.Connect(ctx, fs, port)
			if err != nil {
				return err
			}

			id, err := spec.Resolve(ctx, client, args[0])
			if err != nil {
				return err
			}

			if err := spec.Delete(ctx, client, id); err != nil {
				return err
			}

			// DELETE responds with the full post-delete state — the removed
			// entry is by definition absent from it — so emit a slim
			// confirmation envelope keyed on the id (mirrors the slim
			// `{"id": ...}` result dbms start/delete use).
			switch commonoutput.ResolveOutput(cmd, cfg) {
			case "json":
				payload := struct {
					ID      string `json:"id"`
					Deleted bool   `json:"deleted"`
				}{ID: id, Deleted: true}
				buf, jerr := json.MarshalIndent(payload, "", "\t")
				if jerr != nil {
					return jerr
				}
				cmd.Println(string(buf))
			default:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s %s.\n", spec.Noun, id)
			}
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
