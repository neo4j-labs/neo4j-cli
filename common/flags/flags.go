// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"fmt"
	"strconv"
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
		return BindFormatFromFlag(cmd, cfg)
	}
}

// RegisterRwFlag adds a persistent --rw flag to cmd.
func RegisterRwFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(
		"rw",
		false,
		"Allow write operations. Required for any command that mutates state (Aura API, local config, credentials, skills, write cypher).",
	)
}

// BindFormatFromFlag validates the --format flag and binds it to cfg.Global.
func BindFormatFromFlag(cmd *cobra.Command, cfg *clicfg.Config) error {
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

// EnforceWriteGate rejects write-annotated commands unless --rw is true.
func EnforceWriteGate(cmd *cobra.Command) error {
	if cmd.Annotations["write"] != "true" {
		return nil
	}

	rwFlag := cmd.Flag("rw")
	if rwFlag != nil {
		rw, err := strconv.ParseBool(rwFlag.Value.String())
		if err != nil {
			return err
		}
		if rw {
			return nil
		}
	}

	return clierr.NewUsageError("this command writes; pass --rw to allow it")
}

// ComposeRootPersistentPreRunE returns a root hook that binds format before enforcing --rw.
func ComposeRootPersistentPreRunE(cfg *clicfg.Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := BindFormatFromFlag(cmd, cfg); err != nil {
			return err
		}
		return EnforceWriteGate(cmd)
	}
}
