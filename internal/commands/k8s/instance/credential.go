// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/spf13/cobra"
)

func newCredentialCmd(cfg *clicfg.Config) *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:         "credential <name>",
		Short:       "Store a connection credential for a standalone Neo4j server",
		Annotations: map[string]string{"write": "true"},
		Long: "Read NEO4J_AUTH from Secret `<name>-auth` and save a DBMS credential for a standalone server. " +
			"The credential is named `k8s-<namespace>/<name>@<context>`, so the same instance name on two clusters keeps two credentials. " +
			"Another run replaces the connection and password without changing the default credential. " +
			"The URI points to 127.0.0.1 on --port. Use the same port with `instance connect`, and keep connect running during a query. " +
			"Bolt TLS uses `bolt+ssc`, which encrypts traffic without certificate verification. This command requires --rw.",
		Example: `# Store connection details for a standalone server
neo4j-cli k8s instance credential dev --rw

# Store a connection that uses a different local port
neo4j-cli k8s instance credential dev --port 7688 --format json --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := engine.ValidateResourceName("Neo4j name", args[0]); err != nil {
				return err
			}
			if err := validateLocalPort(port); err != nil {
				return err
			}
			if cfg.DbmsCredentials() == nil {
				return clierr.NewUsageError("credential storage is not available")
			}
			settings, err := instanceSettings(cmd, cfg)
			if err != nil {
				return err
			}
			client := clientFactory(settings)
			kubeContext, err := k8sflags.PrintWriteTarget(cmd, client, settings.Namespace)
			if err != nil {
				return err
			}
			credential, err := engine.ReadCredential(cmd.Context(), client, kubeContext, args[0], port)
			if err != nil {
				return err
			}
			warnUnverifiedTLS(cmd, credential)
			if err := storeCredential(cfg, credential); err != nil {
				return err
			}
			printCredential(cmd, cfg, credential)
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", engine.DefaultBoltPort, "Local Bolt port for the stored URI (use the same port with instance connect)")
	return cmd
}
