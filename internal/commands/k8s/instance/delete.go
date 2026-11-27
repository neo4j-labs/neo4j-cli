// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	"github.com/neo4j/cli/internal/flags"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:         "delete <name>",
		Short:       "Delete a Neo4j resource",
		Annotations: map[string]string{"write": "true"},
		Long: "Delete one Neo4j custom resource from the group's workload namespace, or from `default` when no namespace is set. " +
			"The operator's volume claim retention policy decides whether persistent volume claims remain. " +
			"This command never deletes a volume claim directly. " +
			"Pass --wait to poll until the resource is absent, with a 10 minute timeout. Requires --rw.",
		Example: `# Start deletion and leave volume handling to the resource's retention policy
neo4j-cli k8s instance delete dev --rw

# Wait until the Neo4j resource has gone
neo4j-cli k8s instance delete dev --wait --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			settings, err := instanceSettings(cmd, cfg)
			if err != nil {
				return err
			}
			client := clientFactory(settings)
			if err := k8sflags.PrintWriteTarget(cmd, client, settings.Namespace); err != nil {
				return err
			}
			if err := client.DeleteNeo4j(cmd.Context(), args[0]); err != nil {
				return err
			}
			status := "deletion_started"
			if wait {
				if err := engine.WaitForDelete(cmd.Context(), client, args[0], waitTimeout); err != nil {
					return err
				}
				status = "deleted"
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "Persistent volume claims follow the Neo4j resource's retention policy. This command does not remove them directly.") //nolint:errcheck // Retention guidance is best effort.
			commonoutput.PrintBodyMap(cmd, cfg, deleteResult{
				"name": args[0], "namespace": settings.Namespace, "status": status, "pvc_retention": "follows_spec",
			}, []string{"name", "namespace", "status", "pvc_retention"})
			return nil
		},
	}
	flags.RegisterWait(cmd, &wait, "Wait until the Neo4j resource is absent (10 minute timeout).")
	return cmd
}

type deleteResult map[string]any

func (r deleteResult) AsArray() []map[string]any {
	return []map[string]any{map[string]any(r)}
}
