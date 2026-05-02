// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

// RegisterOutputFlag adds a persistent --output flag to cmd and installs a
// PersistentPreRunE hook that validates the value and binds it to cfg.Global.
func RegisterOutputFlag(cmd *cobra.Command, cfg *clicfg.Config) {
	cmd.PersistentFlags().String(
		"output",
		"",
		fmt.Sprintf("Format to print console output in, from a choice of [%s]", strings.Join(clicfg.ValidOutputValues[:], ", ")),
	)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		outputFlag := cmd.Flags().Lookup("output")
		if outputFlag != nil && outputFlag.Value.String() != "" {
			outputValue := outputFlag.Value.String()
			valid := false
			for _, v := range clicfg.ValidOutputValues {
				if v == outputValue {
					valid = true
					break
				}
			}
			if !valid {
				return clierr.NewUsageError("invalid output value specified: %s", outputValue)
			}
		}

		cfg.Global.BindOutput(cmd.Flags().Lookup("output"))

		return nil
	}
}
