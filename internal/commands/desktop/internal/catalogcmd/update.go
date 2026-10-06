// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/desktopclient"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newUpdateCmd generates the `<resource> update <resource> --name <new>` leaf.
// The positional accepts a UUID or an exact name (resolved via the spec's
// Resolve hook; UUIDs pass through without a catalog fetch). `--name` is
// required because Desktop's update schemas demand a name on every PATCH; the
// id rides in the PATCH body, not the path. A color-bearing spec validates
// --color before the client is built so a bad palette index fails fast without
// touching Desktop.
func newUpdateCmd[T any, S any](cfg *clicfg.Config, spec Spec[T, S]) *cobra.Command {
	var (
		name  string
		color string
	)

	cmd := &cobra.Command{
		Use:         "update <" + spec.Noun + ">",
		Short:       spec.UpdateVerb + " a " + spec.Noun + " in the local Neo4j Desktop 2 install",
		Long:        spec.UpdateLong,
		Example:     spec.UpdateExample,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

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

			id, err := spec.Resolve(ctx, client, args[0])
			if err != nil {
				return err
			}

			state, err := spec.Update(ctx, client, id, name, color)
			if err != nil {
				return err
			}

			updated, err := spec.FindByID(state, id)
			if err != nil {
				return err
			}

			commonoutput.PrintBodyMap(cmd, cfg, spec.itemResult(updated), spec.Fields)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "(required) New name for the "+spec.Noun)
	cmd.MarkFlagRequired("name") //nolint:errcheck // MarkFlagRequired only errors if the flag does not exist, which is a programming error caught at startup

	if spec.Color != nil {
		cmd.Flags().StringVar(&color, "color", "", spec.Color.UpdateUsage)
	}

	return cmd
}
