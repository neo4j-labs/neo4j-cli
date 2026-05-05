// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

// RegisterOutputFlag adds a persistent --format/-f flag to cmd and installs a
// PersistentPreRunE hook that validates the value and binds it to cfg.Global.
func RegisterOutputFlag(cmd *cobra.Command, cfg *clicfg.Config) {
	cmd.PersistentFlags().StringP(
		"format",
		"f",
		"",
		fmt.Sprintf("Format to print console output in, from a choice of [%s]", strings.Join(clicfg.ValidFormatValues[:], ", ")),
	)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		formatFlag := cmd.Flags().Lookup("format")
		if formatFlag != nil && formatFlag.Value.String() != "" {
			formatValue := formatFlag.Value.String()
			valid := false
			for _, v := range clicfg.ValidFormatValues {
				if v == formatValue {
					valid = true
					break
				}
			}
			if !valid {
				return clierr.NewUsageError("invalid format value specified: %s", formatValue)
			}
		}

		cfg.Global.BindFormat(cmd.Flags().Lookup("format"))

		return nil
	}
}
