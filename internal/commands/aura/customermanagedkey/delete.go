// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

import (
	"fmt"
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <id>",
		Short:       "Deletes a customer managed key",
		Annotations: map[string]string{"write": "true"},
		Long: `Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete a customer managed key by ID
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a key and emit JSON for scripting
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete and confirm by piping the response through jq
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json | jq -r '.data.deleted'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cmkID := strings.TrimSpace(args[0])

			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if _, err := auraclient.New(cfg).CustomerManagedKeys().Get(cmd.Context(), auraclient.Scope{ProjectID: projectID}, cmkID); err != nil {
				return err
			}

			if err := confirm.Require(cmd, cmkID); err != nil {
				return err
			}

			if err := auraclient.New(cfg).CustomerManagedKeys().Delete(cmd.Context(), cmkID); err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "customer-managed-key %s deleted\n", cmkID) //nolint:errcheck // narration to stderr; write errors are not actionable
			output.PrintRecord(cmd, cfg, map[string]any{"deleted": true, "id": cmkID},
				[]string{"deleted", "id"})

			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
