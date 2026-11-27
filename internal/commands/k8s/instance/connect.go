// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/clicfg"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/spf13/cobra"
)

func newConnectCmd(cfg *clicfg.Config) *cobra.Command {
	var port int
	var envOutFile string
	cmd := &cobra.Command{
		Use:         "connect <name>",
		Short:       "Forward a standalone Neo4j server's Bolt port to this computer",
		Annotations: map[string]string{"write": "true"},
		Long: "Forward a standalone Neo4j pod's Bolt port to 127.0.0.1 on this computer. " +
			"The command waits up to 60 seconds for an authenticated query to succeed. " +
			"It then prints a dotenv environment on stdout, or writes it with --env-out-file. " +
			"Keep the command running while a query uses the connection, and press Ctrl-C to stop it. " +
			"Use the same --port with `instance credential` to keep a stored URI in step.\n\n" +
			"--env-out-file replaces an existing file without following a symlink and uses mode 0600 on Unix. " +
			"Bolt TLS uses `bolt+ssc`, which encrypts traffic without certificate verification. " +
			"Cluster connections are not supported yet. This command requires --rw.",
		Example: `# Forward a server and write connection details for query --env
neo4j-cli k8s instance connect dev --env-out-file ./dev.env --rw

# Use the same local port as a stored credential
neo4j-cli k8s instance connect dev --port 7688 --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := engine.ValidateResourceName("Neo4j name", args[0]); err != nil {
				return err
			}
			if err := validateLocalPort(port); err != nil {
				return err
			}
			settings, err := instanceSettings(cmd, cfg)
			if err != nil {
				return err
			}
			client := clientFactory(settings)
			if err := k8sflags.PrintWriteTarget(cmd, client, settings.Namespace); err != nil {
				return err
			}
			credential, err := engine.ReadCredential(cmd.Context(), client, args[0], port)
			if err != nil {
				return err
			}
			warnUnverifiedTLS(cmd, credential)
			pod, err := engine.StandalonePod(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			return client.PortForward(cmd.Context(), engine.PortForwardOptions{
				Pod:        pod.Name,
				RemotePort: pod.BoltPort,
				LocalPort:  port,
				OnReady:    connectionReady(cmd, cfg, credential, pod.Name, envOutFile),
			})
		},
	}
	cmd.Flags().IntVar(&port, "port", engine.DefaultBoltPort, "Local Bolt port (use the same port with instance credential)")
	cmd.Flags().StringVar(&envOutFile, "env-out-file", "", "Write the connection environment to this file with owner-only permissions instead of stdout")
	return cmd
}
