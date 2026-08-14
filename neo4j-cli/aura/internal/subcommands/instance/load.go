// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/docker"
	"github.com/spf13/cobra"
)

// gdsPluginSlug is the NEO4J_PLUGINS slug for the Graph Data Science plugin. A
// dataset requiring it cannot be loaded into Aura (GDS is not installable
// there), so its presence in the resolved manifest is a hard error.
const gdsPluginSlug = "graph-data-science"

// resolveDatasetFn / downloadDatasetFn are the injectable seams the load leaf
// uses to talk to the internal/dataset support layer (manifest resolution +
// secure LFS download). Production wires dataset.Resolve / dataset.Download;
// load_test.go swaps deterministic fakes so the leaf's orchestration can be
// exercised without touching the network. They mirror the seam idiom used by
// the docker load leaf (dataset internals are unexported cross-package).
var (
	resolveDatasetFn  = dataset.Resolve
	downloadDatasetFn = dataset.Download
)

// dockerAvailableFn reports whether a local Docker daemon is reachable. Staging
// the dump requires it, so the leaf probes BEFORE creating the Aura instance to
// avoid orphaning a billable instance. Production lists containers (which fails
// when the docker binary is missing or the daemon is down); tests swap a stub.
var dockerAvailableFn = func(ctx context.Context) error {
	_, err := docker.NewDeployClient().PsAll(ctx, nil)
	return err
}

// stageViaDockerFn loads the dump into a fresh ephemeral local Neo4j container,
// pushes that database into the Aura target via neo4j-admin over Bolt, and tears
// the ephemeral container down afterwards. Production wires the real docker
// client + LoadDumpIntoNewContainer + PushToAura; load_test.go swaps a recorder
// so the leaf's orchestration can be exercised without a docker daemon or Bolt.
var stageViaDockerFn = func(ctx context.Context, cfg *clicfg.Config, load datasetStageLoad, target deployTarget, warnOut io.Writer) error {
	return stageViaDocker(ctx, cfg, load, target, warnOut)
}

// datasetStageLoad bundles the inputs the ephemeral docker staging step needs.
type datasetStageLoad struct {
	Database string
	Version  string
	Plugins  []string
	DumpPath string
}

func NewLoadCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		version              string
		region               string
		memory               flags.Memory
		name                 string
		_type                flags.InstanceType
		cloudProvider        flags.CloudProvider
		customerManagedKeyId string
		vectorOptimized      bool
		graphAnalyticsPlugin bool
		credentialName       string
		noCredentialStorage  bool
		noCredentialPrint    bool

		database string
		maxSize  int64
	)

	const (
		versionFlag             = "version"
		regionFlag              = "region"
		memoryFlag              = "memory"
		nameFlag                = "name"
		typeFlag                = "type"
		cloudProviderFlag       = "cloud-provider"
		vectorOptimizedFlag     = "vector-optimized"
		credentialNameFlag      = "credential-name"
		noCredentialStorageFlag = "no-credential-storage"
		noCredentialPrintFlag   = "no-credential-print"
		databaseFlag            = "database"
		maxSizeFlag             = "max-size"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "load <owner/repo>",
		Short:       "Creates a new Aura instance pre-loaded with an example dataset",
		Args:        cobra.ExactArgs(1),
		Example: `# Load the movies dataset into a new free Aura instance
neo4j-cli aura instance load neo4j-graph-examples/movies --rw --name movies-demo --type free --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Load the recommendations dataset into a new professional instance on AWS and emit JSON
neo4j-cli aura instance load neo4j-graph-examples/recommendations --rw --name recs --type professional --cloud-provider aws --region us-east-1 --memory 2GB --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		Long: `This subcommand creates a new Aura instance and loads an example Neo4j dataset into it.

A dataset is a '.dump' published by a GitHub repo carrying a 'relate.project-install.json' manifest (e.g. 'neo4j-graph-examples/movies'). The manifest is resolved for the requested --version, the matching dump is downloaded from the Git-LFS media host, and the data is loaded into the --database (default "neo4j") of a new Aura instance provisioned with the same flags as 'instance create'.

Aura has no dump-upload API, so the dump is staged through an ephemeral local Neo4j Docker container and then uploaded into the new instance over Bolt. A local Docker daemon is therefore REQUIRED; the command errors early (before creating any Aura instance) if Docker is unavailable.

Datasets requiring the graph-data-science plugin cannot be loaded into Aura (GDS is not installable there); such a dataset is rejected before any work is done. The apoc plugin is allowed.

If the data load fails after the instance was created, the instance is left in place (it is not deleted), load_status=failed is reported, and the instance id is printed so you can retry or delete it manually.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if strings.EqualFold(database, "system") {
				return clierr.NewUsageError(`invalid argument "system" for "--%s" flag: the system database cannot be loaded into`, databaseFlag)
			}
			return validateInstanceFlags(cmd, cfg, instanceFlags{
				instanceType:        _type,
				memory:              memory,
				region:              region,
				cloudProvider:       cloudProvider,
				version:             version,
				credentialName:      credentialName,
				credentialNameSet:   cmd.Flags().Changed(credentialNameFlag),
				noCredentialStorage: noCredentialStorage,
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			errOut := cmd.ErrOrStderr()
			ownerRepo := args[0]

			resolvedOrgID, resolvedProjectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			// Resolve the manifest first so the GDS hard-error and the
			// downstream plugin set are known BEFORE any instance is created.
			spec, err := resolveDatasetFn(ctx, ownerRepo, version)
			if err != nil {
				return clierr.NewUsageError("resolve dataset %q: %s", ownerRepo, err.Error())
			}

			// (1) GDS hard-error: GDS is not installable on Aura, so refuse
			// before doing any work (before the docker probe and instance
			// creation).
			if containsPlugin(spec.Plugins, gdsPluginSlug) {
				return clierr.NewUsageError(
					"dataset %q requires the %q plugin, which cannot be installed on Aura. Load it into a local target instead (e.g. `neo4j-cli docker load %s --name %s`).",
					ownerRepo, gdsPluginSlug, ownerRepo, spec.Repo,
				)
			}

			// (2) Docker daemon availability: staging requires a local Docker
			// daemon, so probe BEFORE creating the instance to avoid orphaning a
			// billable instance when Docker is unavailable.
			if err := dockerAvailableFn(ctx); err != nil {
				return clierr.NewUsageError(
					"aura instance load stages the dump through an ephemeral local Neo4j Docker container, but Docker is not available: %s",
					err.Error(),
				)
			}

			name, err = resolveInstanceName(cfg, name, resolvedOrgID, resolvedProjectID)
			if err != nil {
				return err
			}

			body := buildCreateInstanceBody(version, region, name, _type, cloudProvider, customerManagedKeyId, memory, vectorOptimized, graphAnalyticsPlugin, resolvedProjectID)

			fmt.Fprintln(errOut, "Creating instance...") //nolint:errcheck // narration to stderr; write errors are not actionable

			instance, err := createAndStoreInstance(cfg, body, resolvedOrgID, resolvedProjectID, credentialOptions{
				instanceType:        string(_type),
				credentialName:      credentialName,
				noCredentialStorage: noCredentialStorage,
				noCredentialPrint:   noCredentialPrint,
				warnOut:             errOut,
			})
			if err != nil {
				return err
			}
			if instance == nil {
				return clierr.NewFatalError("instance creation did not return a usable response")
			}

			instanceID, _ := instance["id"].(string)
			target := deployTarget{
				URI:      stringField(instance, "connection_url"),
				Username: stringField(instance, "username"),
				Password: stringField(instance, "password"),
			}

			fmt.Fprintln(errOut, "Waiting for instance to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
			if _, err := api.PollInstance(cfg, resolvedOrgID, resolvedProjectID, instanceID, api.InstanceStatusCreating); err != nil {
				return err
			}

			fmt.Fprintf(errOut, "Downloading dataset %q...\n", ownerRepo) //nolint:errcheck // narration to stderr; write errors are not actionable
			dumpPath, cleanup, err := downloadDatasetFn(ctx, spec, maxSize)
			if err != nil {
				instance["load_status"] = "failed"
				renderInstanceResult(cmd, cfg, instance, noCredentialPrint, noCredentialStorage, "load_status")
				fmt.Fprintf(errOut, "Error: dataset download failed; the instance %q was created and left in place — retry the load or delete it with `neo4j-cli aura instance delete %s --rw`.\n", instanceID, instanceID) //nolint:errcheck // narration to stderr; write errors are not actionable
				return clierr.NewFatalError("download dataset dump: %s", err.Error())
			}
			defer cleanup()

			fmt.Fprintf(errOut, "Staging dataset into database %q via an ephemeral local Neo4j container...\n", database) //nolint:errcheck // narration to stderr; write errors are not actionable
			loadErr := stageViaDockerFn(ctx, cfg, datasetStageLoad{
				Database: database,
				Version:  version,
				Plugins:  spec.Plugins,
				DumpPath: dumpPath,
			}, target, errOut)

			loadStatus := "succeeded"
			if loadErr != nil {
				loadStatus = "failed"
				fmt.Fprintf(errOut, "Error: data load failed; the instance %q was created and left in place — retry the load or delete it with `neo4j-cli aura instance delete %s --rw`.\n", instanceID, instanceID) //nolint:errcheck // narration to stderr; write errors are not actionable
			}

			instance["load_status"] = loadStatus
			renderInstanceResult(cmd, cfg, instance, noCredentialPrint, noCredentialStorage, "load_status")

			return loadErr
		},
	}

	cmd.Flags().StringVar(&version, versionFlag, "5", "The Neo4j version of the instance. Also used to resolve the dataset manifest.")
	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'neo4j-cli aura api v1/tenants/<project-id>' to see the full list of supported regions for your project.")
	cmd.Flags().Var(&memory, memoryFlag, "The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes.")
	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01).")

	cmd.Flags().Var(&_type, typeFlag, `(required) The type of the instance. Must be one of "free", "professional", "business-critical", or "virtual-dedicated-cloud". The former names "free-db", "professional-db", and "enterprise-db" are still accepted.`)
	cmd.MarkFlagRequired(typeFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, `The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp".`)
	cmd.Flags().BoolVar(&vectorOptimized, vectorOptimizedFlag, false, "An optional vector optimization configuration to be set during instance creation")

	cmd.Flags().StringVar(&database, databaseFlag, "neo4j", "The target database the dataset is loaded into. The system database cannot be loaded into.")
	cmd.Flags().Int64Var(&maxSize, maxSizeFlag, dataset.DefaultMaxDumpBytes, "Maximum dump download size in bytes; the download is refused if exceeded.")

	cmd.Flags().StringVar(&credentialName, credentialNameFlag, "", "The name to use when storing the credentials locally. Defaults to <instance-id>-default.")
	cmd.Flags().BoolVar(&noCredentialStorage, noCredentialStorageFlag, false, "Skip storing the instance credentials locally after creation.")
	cmd.Flags().BoolVar(&noCredentialPrint, noCredentialPrintFlag, false, "Omit the password from the command output.")

	return cmd
}

// stageViaDocker loads the dump into a fresh ephemeral local Neo4j container,
// pushes the database into the Aura target over Bolt, then removes the ephemeral
// container and its stored credential. The teardown is deferred so it runs even
// when the push fails. The container is created with NEO4J_PLUGINS from the
// manifest so the staged database matches the dataset's plugin expectations.
func stageViaDocker(ctx context.Context, cfg *clicfg.Config, load datasetStageLoad, target deployTarget, warnOut io.Writer) error {
	client := docker.NewDeployClient()

	result, err := docker.LoadDumpIntoNewContainer(ctx, cfg, client, docker.NewContainerLoad{
		Name:       "neo4j-cli-aura-load",
		Database:   load.Database,
		Version:    load.Version,
		Plugins:    load.Plugins,
		DumpPath:   load.DumpPath,
		Wait:       true,
		WaitOut:    warnOut,
		PreflightO: warnOut,
	})
	if err != nil {
		return err
	}

	// Always remove the ephemeral container and its stored credential, even if
	// the push below fails.
	defer func() {
		_ = client.RemoveForce(ctx, result.Name)
		if cfg != nil && cfg.Credentials != nil {
			_ = cfg.Credentials.RemoveDbms(result.Name, warnOut)
		}
	}()

	return docker.PushToAura(ctx, cfg, client, result.Name, load.Database, docker.AuraTarget{
		URI:      target.URI,
		Username: target.Username,
		Password: target.Password,
	})
}

// containsPlugin reports whether plugins contains slug (case- and
// whitespace-insensitive).
func containsPlugin(plugins []string, slug string) bool {
	for _, p := range plugins {
		if strings.EqualFold(strings.TrimSpace(p), slug) {
			return true
		}
	}
	return false
}
