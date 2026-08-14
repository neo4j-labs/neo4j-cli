// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/docker"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// deployTarget is the resolved Aura connection used by both source paths to
// restore the dumped database into the freshly-created instance.
type deployTarget struct {
	URI      string
	Username string
	Password string
}

// deployViaDockerFn pushes the source database from a local Neo4j container
// into the Aura target. Production wires the real docker client + PushToAura;
// deploy_test.go swaps a recorder so the leaf's orchestration can be exercised
// without a docker daemon or Bolt server.
var deployViaDockerFn = func(ctx context.Context, cfg *clicfg.Config, containerName, database string, target deployTarget) error {
	return docker.PushToAura(ctx, cfg, docker.NewDeployClient(), containerName, database, docker.AuraTarget{
		URI:      target.URI,
		Username: target.Username,
		Password: target.Password,
	})
}

// dockerSourceEditionFn inspects a local Neo4j container and returns its
// edition label ("community"/"enterprise"). Production wires the real docker
// client; deploy_test.go swaps a stub so the community-edition fast-fail guard
// can be exercised without a docker daemon.
var dockerSourceEditionFn = func(ctx context.Context, containerName string) (string, error) {
	container, err := docker.Inspect(ctx, docker.NewDeployClient(), containerName)
	if err != nil {
		return "", err
	}
	return container.Edition, nil
}

// deployViaDesktopFn clones a database managed by a local Neo4j Desktop 2
// install into the Aura target. Production wires the Desktop relate client and
// manages the source DBMS lifecycle; tests swap a recorder.
var deployViaDesktopFn = func(ctx context.Context, cfg *clicfg.Config, dbmsID, database string, port int, target deployTarget, warnOut io.Writer) error {
	return deployViaDesktop(ctx, cfg, dbmsID, database, port, target, warnOut)
}

// newDeployDesktopClient mirrors the desktop subtree's client factory: discover
// the relate API (honouring a pinned --desktop-port), resolve the data dir,
// load the salt, and sign the JWT. Kept as a package var so tests can avoid a
// live Desktop install.
var newDeployDesktopClient = func(ctx context.Context, fs afero.Fs, port int) (*desktopclient.Client, error) {
	probe, err := desktopclient.Discover(ctx, port)
	if err != nil {
		if errors.Is(err, desktopclient.ErrNoDesktop) {
			return nil, desktopclient.UnreachableError()
		}
		return nil, clierr.NewFatalError("desktop: probe failed: %s", err.Error())
	}
	dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return nil, clierr.NewFatalError("desktop: could not resolve relate data dir: %s", err.Error())
	}
	salt, err := desktopclient.LoadSalt(fs, dataDir)
	if err != nil {
		return nil, desktopclient.UnreachableError()
	}
	return desktopclient.NewClient(probe, salt)
}

const (
	dbmsStatusStarted = "started"
	dbmsStatusStopped = "stopped"
)

// desktopStopPollInterval / desktopStopPollTimeout bound the wait for the
// source DBMS to settle to "stopped" before the upload begins.
var (
	desktopStopPollInterval = 1 * time.Second
	desktopStopPollTimeout  = 30 * time.Second
)

func NewDeployCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		fromDocker  string
		fromDesktop string
		database    string
		desktopPort int

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
	)

	const (
		fromDockerFlag  = "from-docker"
		fromDesktopFlag = "from-desktop"
		databaseFlag    = "database"
		desktopPortFlag = "desktop-port"

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
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "deploy",
		Short:       "Creates a new Aura instance and clones a local database into it",
		Example: `# Deploy a local Docker container's neo4j database into a new free Aura instance
neo4j-cli aura instance deploy --rw --from-docker my-local-neo4j --type free --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Deploy a Neo4j Desktop 2 DBMS database into a new professional instance on AWS
neo4j-cli aura instance deploy --rw --from-desktop dbms-1234 --database movies --type professional --cloud-provider aws --region us-east-1 --memory 2GB --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111`,
		Long: `This subcommand creates a new Aura instance and clones a local Neo4j database into it.

The source database can come from a local Neo4j Docker container managed by 'neo4j-cli docker' (--from-docker) or from a DBMS managed by a local Neo4j Desktop 2 install (--from-desktop). Exactly one source must be specified.

deploy operates on Enterprise Neo4j sources only: Neo4j Desktop 2 manages only enterprise DBMSs, and the --from-docker path requires an enterprise container (the dump relies on the enterprise-only STOP DATABASE command).

A new Aura instance is provisioned using the same flags as 'instance create', then the named --database (default "neo4j") is dumped from the source and uploaded into the new instance, overwriting its contents. The "system" database cannot be cloned.

The command waits for the instance to be ready and for the data load to finish before returning. On success the structured output reports the instance connection details plus deploy_status=succeeded. If the data load fails after the instance was created, the instance is left in place (it is not deleted), deploy_status=failed is reported, and the instance id is printed so you can retry or delete it manually.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if strings.EqualFold(database, "system") {
				return clierr.NewUsageError(`invalid argument "system" for "--%s" flag: the system database cannot be cloned`, databaseFlag)
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

			resolvedOrgID, resolvedProjectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			// Fast-fail the docker source on a community-edition container BEFORE
			// creating any Aura instance: the docker path runs `STOP DATABASE` to
			// take a consistent offline dump, which is enterprise-only — community
			// would fail mid-deploy and orphan a billable instance.
			if fromDocker != "" {
				edition, err := dockerSourceEditionFn(ctx, fromDocker)
				if err != nil {
					return err
				}
				if strings.EqualFold(edition, "community") {
					return clierr.NewUsageError("deploy --from-docker requires an enterprise Neo4j container; community edition cannot take an online dump (STOP DATABASE is enterprise-only). Recreate the container with `neo4j-cli docker create --edition enterprise`.")
				}
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

			// Capture the generated target credentials BEFORE any --no-credential-print
			// scrubbing — they are needed to authenticate the upload.
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

			fmt.Fprintf(errOut, "Uploading database %q...\n", database) //nolint:errcheck // narration to stderr; write errors are not actionable

			var pushErr error
			switch {
			case fromDocker != "":
				pushErr = deployViaDockerFn(ctx, cfg, fromDocker, database, target)
			case fromDesktop != "":
				pushErr = deployViaDesktopFn(ctx, cfg, fromDesktop, database, desktopPort, target, errOut)
			}

			deployStatus := "succeeded"
			if pushErr != nil {
				deployStatus = "failed"
				fmt.Fprintf(errOut, "Error: data load failed; the instance %q was created and left in place — retry the upload or delete it with `neo4j-cli aura instance delete %s --rw`.\n", instanceID, instanceID) //nolint:errcheck // narration to stderr; write errors are not actionable
			}

			instance["deploy_status"] = deployStatus
			renderInstanceResult(cmd, cfg, instance, noCredentialPrint, noCredentialStorage, "deploy_status")

			return pushErr
		},
	}

	cmd.Flags().StringVar(&fromDocker, fromDockerFlag, "", "Name of a local Neo4j Docker container (managed by `neo4j-cli docker`) to clone the database from.")
	cmd.Flags().StringVar(&fromDesktop, fromDesktopFlag, "", "ID of a DBMS managed by a local Neo4j Desktop 2 install to clone the database from.")
	cmd.MarkFlagsMutuallyExclusive(fromDockerFlag, fromDesktopFlag)
	cmd.MarkFlagsOneRequired(fromDockerFlag, fromDesktopFlag)

	cmd.Flags().StringVar(&database, databaseFlag, "neo4j", "The name of the source database to clone. The system database cannot be cloned.")
	cmd.Flags().IntVar(&desktopPort, desktopPortFlag, 0, "Pin the Neo4j Desktop 2 relate API to a specific port instead of probing 44222..44232 (used only with --from-desktop).")

	cmd.Flags().StringVar(&version, versionFlag, "5", "The Neo4j version of the instance.")
	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'neo4j-cli aura api v1/tenants/<project-id>' to see the full list of supported regions for your project.")
	cmd.Flags().Var(&memory, memoryFlag, "The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes.")
	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01).")

	cmd.Flags().Var(&_type, typeFlag, `(required) The type of the instance. Must be one of "free", "professional", "business-critical", or "virtual-dedicated-cloud". The former names "free-db", "professional-db", and "enterprise-db" are still accepted.`)
	cmd.MarkFlagRequired(typeFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, `The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp".`)
	cmd.Flags().BoolVar(&vectorOptimized, vectorOptimizedFlag, false, "An optional vector optimization configuration to be set during instance creation")

	cmd.Flags().StringVar(&credentialName, credentialNameFlag, "", "The name to use when storing the credentials locally. Defaults to <instance-id>-default.")
	cmd.Flags().BoolVar(&noCredentialStorage, noCredentialStorageFlag, false, "Skip storing the instance credentials locally after creation.")
	cmd.Flags().BoolVar(&noCredentialPrint, noCredentialPrintFlag, false, "Omit the password from the command output.")

	return cmd
}

// stringField reads a string value from an instance map, returning "" when the
// key is absent or not a string.
func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// deployViaDesktop manages the source DBMS lifecycle around a Desktop upload:
// stop it (if running) for a clean dump, upload the database into the Aura
// target, wait for the task to settle, and restore the prior running state. The
// restore is deferred so it runs even when the upload fails.
func deployViaDesktop(ctx context.Context, cfg *clicfg.Config, dbmsID, database string, port int, target deployTarget, warnOut io.Writer) error {
	client, err := newDeployDesktopClient(ctx, cfg.Aura.Fs(), port)
	if err != nil {
		return err
	}

	info, err := client.GetDbms(ctx, dbmsID)
	if err != nil {
		return err
	}
	wasRunning := info.Status == dbmsStatusStarted

	if wasRunning {
		fmt.Fprintf(warnOut, "Stopping DBMS %q for a consistent dump...\n", dbmsID) //nolint:errcheck // narration to stderr; write errors are not actionable
		if err := client.StopDbms(ctx, dbmsID); err != nil {
			return err
		}
		if err := pollDesktopStatus(ctx, client, dbmsID, dbmsStatusStopped); err != nil {
			return err
		}
		// Restore the DBMS to its prior running state even if the upload fails.
		defer func() {
			fmt.Fprintf(warnOut, "Restarting DBMS %q...\n", dbmsID) //nolint:errcheck // narration to stderr; write errors are not actionable
			_ = client.StartDbms(ctx, dbmsID)
		}()
	}

	if err := client.UploadDatabase(ctx, dbmsID, desktopclient.UploadSource{DatabaseName: database}, desktopclient.UploadTarget{
		URI:       target.URI,
		Username:  target.Username,
		Password:  target.Password,
		Overwrite: true,
	}); err != nil {
		return err
	}

	return desktopclient.WaitForUploadTask(ctx, client, dbmsID)
}

// pollDesktopStatus polls GetDbms until the DBMS reports the target status or
// the timeout elapses.
func pollDesktopStatus(ctx context.Context, client *desktopclient.Client, id, target string) error {
	deadline := time.Now().Add(desktopStopPollTimeout)
	var lastStatus string
	for {
		info, err := client.GetDbms(ctx, id)
		if err != nil {
			return err
		}
		if info.Status == target {
			return nil
		}
		lastStatus = info.Status
		if time.Now().After(deadline) {
			return clierr.NewFatalError(
				"timed out after %s waiting for DBMS %q to reach status %q (last status: %q)",
				desktopStopPollTimeout, id, target, lastStatus)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(desktopStopPollInterval):
		}
	}
}
