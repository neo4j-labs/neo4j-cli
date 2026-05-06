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

// RegisterAuraCredentialFlag adds a persistent --credential/-c flag to cmd and
// wraps any existing PersistentPreRunE with a hook that resolves the named
// credential from the store and stores it via cfg.Aura.SetActiveCredential.
//
// Hook execution order:
//  1. Run the pre-existing PersistentPreRunE (if any); abort on error.
//  2. If --credential was set, look up the credential by name.
//     On failure, return a usage error hinting the correct credential list command.
//     On success, call cfg.Aura.SetActiveCredential.
//  3. If --credential was not set, cfg is left unchanged (GetDefault fallback applies).
func RegisterAuraCredentialFlag(cmd *cobra.Command, cfg *clicfg.Config) {
	cmd.PersistentFlags().StringP(
		"credential",
		"c",
		"",
		"Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list')",
	)

	prior := cmd.PersistentPreRunE

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if prior != nil {
			if err := prior(cmd, args); err != nil {
				return err
			}
		}

		credFlag := cmd.Flag("credential")
		if credFlag == nil || !credFlag.Changed {
			return nil
		}

		name := credFlag.Value.String()
		cred, err := cfg.Credentials.Aura.Get(name)
		if err != nil {
			rootUse := cmd.Root().Use
			var hint string
			if rootUse == "neo4j-cli" {
				hint = "neo4j-cli aura credential list"
			} else {
				hint = "aura-cli credential list"
			}
			return clierr.NewUsageError("credential %q not found, run `%s` to see available credentials", name, hint)
		}

		cfg.Aura.SetActiveCredential(cred)
		return nil
	}
}
