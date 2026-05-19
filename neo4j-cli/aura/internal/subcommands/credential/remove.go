// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "remove <name>",
		Short:       "Removes a credential",
		Annotations: map[string]string{"write": "true"},
		Example: `# Remove a stored credential by name
neo4j-cli aura credential remove my-creds --rw

# Remove a staging credential
neo4j-cli aura credential remove staging --rw

# Remove and confirm by listing remaining credentials as JSON
neo4j-cli aura credential remove my-creds --rw && neo4j-cli aura credential list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			credName := strings.TrimSpace(args[0])
			if err := cfg.Credentials.Aura.Remove(credName); err != nil {
				return err
			}
			if cfg.Credentials.StorageMode() == credentials.StorageModeKeyring {
				if err := cfg.Credentials.DeleteKeyringEntries("aura", credName); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", err) //nolint:errcheck // warning write to stderr; original removal already succeeded
				}
			}
			return nil
		},
	}
}
