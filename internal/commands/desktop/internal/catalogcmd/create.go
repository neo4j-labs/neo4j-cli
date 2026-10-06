// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/desktopclient"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newCreateCmd generates the `<resource> create <name>` leaf. Desktop's POST
// responds with the full post-create catalog state rather than the single
// entry, so the created entry is resolved out of that state by name via the
// spec's FindByName — absent or ambiguous is a fatal error, never a silently
// printed `null` or a guessed entry. A color-bearing spec validates --color
// before the client is built so a bad palette index fails fast without
// touching Desktop.
func newCreateCmd[T any, S any](cfg *clicfg.Config, spec Spec[T, S]) *cobra.Command {
	var color string

	cmd := &cobra.Command{
		Use:         "create <name>",
		Short:       "Create a " + spec.Noun + " in the local Neo4j Desktop 2 install",
		Long:        spec.CreateLong,
		Example:     spec.CreateExample,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			if spec.Color != nil {
				if err := validatePaletteColor(color); err != nil {
					return err
				}
			}

			ctx := cmd.Context()
			fs := cfg.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			client, err := desktopclient.Connect(ctx, fs, port)
			if err != nil {
				return err
			}

			state, err := spec.Create(ctx, client, name, color)
			if err != nil {
				return err
			}

			created, err := spec.FindByName(state, name)
			if err != nil {
				return err
			}

			commonoutput.PrintBodyMap(cmd, cfg, spec.itemResult(created), spec.Fields)
			return nil
		},
	}

	if spec.Color != nil {
		cmd.Flags().StringVar(&color, "color", "", spec.Color.CreateUsage)
	}

	return cmd
}
