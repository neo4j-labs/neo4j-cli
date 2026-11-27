// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	"github.com/neo4j/cli/internal/flags"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var opts engine.Neo4jManifestOptions
	var fromFile, operatorNamespace string
	var wait bool

	cmd := &cobra.Command{
		Use:         "create <name>",
		Short:       "Create a Neo4j standalone server or cluster",
		Annotations: map[string]string{"write": "true"},
		Long: "Create one Neo4j custom resource in the group's workload namespace, or in `default` when no namespace is set. " +
			"The command checks the operator's watch scope before it creates anything. " +
			"Enterprise edition requires an explicit --accept-license yes or eval. " +
			"Cluster mode requires --insecure-admin-connection unless --from-file supplies Bolt TLS. " +
			"The insecure option sends the operator's admin password over an unencrypted pod-network connection. " +
			"--memory and --cpu set both container requests and limits. --heap sets the initial and maximum heap together. " +
			"The command reserves 1Gi for the JVM and operating system when it checks heap and page cache against the memory limit. " +
			"--from-file accepts one complete Neo4j YAML object and preserves fields that have no dedicated flag, including TLS. " +
			"The positional name and group namespace replace the file's metadata values. " +
			"Other specification flags cannot be combined with --from-file. " +
			"Pass --wait to poll until Ready=True for the current generation, with a 10 minute timeout. Requires --rw.",
		Example: `# Create a standalone server with an evaluation licence and wait for readiness
neo4j-cli k8s instance create dev --accept-license eval --wait --rw

# Create a three-primary cluster with an unencrypted operator admin connection
neo4j-cli k8s instance create dev-cluster --mode cluster --primaries 3 --accept-license eval --insecure-admin-connection --rw

# Create from a complete manifest that contains a TLS trust block
neo4j-cli k8s instance create dev-tls --from-file ./neo4j.yaml --wait --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			settings, err := instanceSettings(cmd, cfg)
			if err != nil {
				return err
			}
			opts.Name = args[0]
			opts.Namespace = settings.Namespace
			opts.PrimariesSet = cmd.Flags().Changed("primaries")

			var manifest []byte
			if fromFile != "" {
				if err := rejectFlagsWithFile(cmd); err != nil {
					return err
				}
				data, err := afero.ReadFile(cfg.Fs(), fromFile)
				if err != nil {
					return clierr.NewUsageError("read --from-file %q: %s", fromFile, err)
				}
				manifest, err = engine.PrepareNeo4jManifestFromFile(data, opts.Name, opts.Namespace)
				if err != nil {
					return err
				}
			} else {
				manifest, err = engine.BuildNeo4jManifest(opts)
				if err != nil {
					return err
				}
			}
			summary, err := engine.InspectNeo4jManifest(manifest)
			if err != nil {
				return err
			}
			if summary.Mode == "Cluster" && summary.Primaries == 1 {
				fmt.Fprintln(cmd.ErrOrStderr(), "Warning: a cluster created with one primary cannot grow its primary pool later.") //nolint:errcheck // Warning output is best effort.
			}
			if summary.Mode == "Cluster" && summary.InsecureAdminConnection {
				fmt.Fprintln(cmd.ErrOrStderr(), "Warning: the operator will send its admin password over an unencrypted pod-network connection.") //nolint:errcheck // Warning output is best effort.
			}

			client := clientFactory(settings)
			if err := k8sflags.PrintWriteTarget(cmd, client, settings.Namespace); err != nil {
				return err
			}
			operatorSettings := settings
			operatorSettings.Namespace = operatorNamespace
			if err := ensureNamespaceWatched(cmd.Context(), clientFactory(operatorSettings), settings.Namespace, operatorNamespace); err != nil {
				return err
			}
			instance, err := client.CreateNeo4j(cmd.Context(), manifest)
			if err != nil {
				return err
			}
			if wait {
				instance, err = engine.WaitForReady(cmd.Context(), client, opts.Name, waitTimeout)
				if err != nil {
					return err
				}
			}
			printInstance(cmd, cfg, instance)
			return nil
		},
	}

	cmd.Flags().StringVar(&operatorNamespace, "operator-namespace", engine.DefaultOperatorNamespace, "Namespace that contains the Neo4j operator")
	cmd.Flags().StringVar(&opts.Mode, "mode", "standalone", "Topology mode: standalone or cluster")
	cmd.Flags().IntVar(&opts.Primaries, "primaries", 3, "Number of primary members in cluster mode (odd, 1 to 15)")
	cmd.Flags().StringVar(&opts.Version, "version", engine.DefaultNeo4jVersion, "Neo4j image version")
	cmd.Flags().StringVar(&opts.StorageSize, "storage-size", engine.DefaultStorageSize, "Data volume size")
	cmd.Flags().StringVar(&opts.StorageClass, "storage-class", "", "StorageClass for the data volume (default: the cluster default)")
	cmd.Flags().StringVar(&opts.Edition, "edition", "enterprise", "Neo4j edition: enterprise or community")
	cmd.Flags().StringVar(&opts.AcceptLicense, "accept-license", "", "Enterprise licence acceptance: yes or eval")
	cmd.Flags().BoolVar(&opts.InsecureAdminConnection, "insecure-admin-connection", false, "Let the operator administer a cluster over unencrypted Bolt")
	cmd.Flags().StringVar(&opts.ServiceType, "service-type", "ClusterIP", "Client Service type: ClusterIP, LoadBalancer or NodePort")
	cmd.Flags().StringArrayVar(&opts.LoadBalancerSourceRanges, "load-balancer-source-range", nil, "CIDR allowed to reach a LoadBalancer Service (repeatable, required for LoadBalancer)")
	cmd.Flags().StringArrayVar(&opts.Plugins, "plugin", nil, "Plugin to install in standalone mode: apoc, gds or bloom (repeatable)")
	cmd.Flags().StringVar(&opts.Memory, "memory", "", "Container memory request and limit, for example 8Gi")
	cmd.Flags().StringVar(&opts.CPU, "cpu", "", "Container CPU request and limit, for example 2")
	cmd.Flags().StringVar(&opts.Heap, "heap", "", "Initial and maximum Neo4j heap size, for example 2g")
	cmd.Flags().StringVar(&opts.PageCache, "pagecache", "", "Neo4j page cache size, for example 1g")
	cmd.Flags().StringArrayVar(&opts.Config, "config", nil, "Neo4j configuration as key=value (repeatable)")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Create from one complete Neo4j YAML object instead of the specification flags")
	flags.RegisterWait(cmd, &wait, "Wait until the operator reports the current generation as ready (10 minute timeout).")
	return cmd
}

func rejectFlagsWithFile(cmd *cobra.Command) error {
	for _, name := range []string{
		"mode", "primaries", "version", "storage-size", "storage-class", "edition", "accept-license",
		"insecure-admin-connection", "service-type", "load-balancer-source-range", "plugin", "memory", "cpu",
		"heap", "pagecache", "config",
	} {
		if cmd.Flags().Changed(name) {
			return clierr.NewUsageError("--from-file cannot be combined with --%s. Put that field in the YAML file", name)
		}
	}
	return nil
}
