// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Removes a dbms credential",
		Long:  "Remove a stored Bolt connection profile by name. Linked embed-credential references on other profiles are not modified.",
		Example: `# Remove a dbms credential by name
neo4j-cli credential dbms remove local --rw

# Remove a staging dbms credential
neo4j-cli credential dbms remove staging --rw

# Remove a prod dbms credential
neo4j-cli credential dbms remove prod --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := cfg.Credentials.Dbms.Remove(name); err != nil {
				return err
			}
			if cfg.Credentials.StorageMode() == credentials.StorageModeKeyring {
				if err := cfg.Credentials.DeleteKeyringEntries("dbms", name); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", err) //nolint:errcheck // warning write to stderr; original removal already succeeded
				}
			}
			return nil
		},
	}
}
