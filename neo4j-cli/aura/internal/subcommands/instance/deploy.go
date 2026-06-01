// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
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
		Long: `This subcommand creates a new Aura instance and clones a local Neo4j database into it.

The source database can come from a local Neo4j Docker container managed by 'neo4j-cli docker' (--from-docker) or from a DBMS managed by a local Neo4j Desktop 2 install (--from-desktop). Exactly one source must be specified.

A new Aura instance is provisioned using the same flags as 'instance create', then the named --database (default "neo4j") is dumped from the source and uploaded into the new instance, overwriting its contents. The "system" database cannot be cloned.

The command waits for the instance to be ready and for the data load to finish before returning. On success the structured output reports the instance connection details plus deploy_status=succeeded. If the data load fails after the instance was created, the instance is left in place (it is not deleted), deploy_status=failed is reported, and the instance id is printed so you can retry or delete it manually.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if database == "system" {
				return clierr.NewUsageError(`invalid argument "system" for "--%s" flag: the system database cannot be cloned`, databaseFlag)
			}

			if _type != "free-db" {
				cmd.MarkFlagRequired(memoryFlag)        //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				cmd.MarkFlagRequired(regionFlag)        //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			} else {
				if memory != "" {
					return fmt.Errorf(`invalid argument "%s" for "--memory" flag: must not be set when "--type" flag is set to "free-db"`, memory)
				}
				if region != "" {
					return fmt.Errorf(`invalid argument "%s" for "--region" flag: must not be set when "--type" flag is set to "free-db"`, region)
				}
				if cloudProvider != "" {
					return fmt.Errorf(`invalid argument "%s" for "--cloud-provider" flag: must not be set when "--type" flag is set to "free-db"`, cloudProvider)
				}
			}

			if version != "4" && version != "5" {
				return fmt.Errorf(`invalid argument "%s" for "--version" flag: must be one of "4" or "5"`, version)
			}

			credentialNameChanged := cmd.Flags().Changed(credentialNameFlag)

			if credentialNameChanged && noCredentialStorage {
				return fmt.Errorf(`"--%s" and "--%s" cannot be used together`, credentialNameFlag, noCredentialStorageFlag)
			}

			if credentialNameChanged && credentialName == "" {
				return fmt.Errorf(`invalid argument "" for "--%s" flag: name must not be empty`, credentialNameFlag)
			}

			if !noCredentialStorage && (cfg.Credentials == nil || cfg.Credentials.Dbms == nil) {
				return fmt.Errorf("credential storage is not available; use --%s to skip storing credentials locally", noCredentialStorageFlag)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			errOut := cmd.ErrOrStderr()

			_, resolvedProjectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if name == "" {
				listBody, _, listErr := api.MakeRequest(cfg, "/instances", &api.RequestConfig{
					Method:      http.MethodGet,
					QueryParams: map[string]string{"tenantId": resolvedProjectID},
				})
				if listErr != nil {
					return listErr
				}
				listData := api.ParseBody(listBody)
				existingNames := make([]string, 0, len(listData.AsArray()))
				for _, inst := range listData.AsArray() {
					if n, ok := inst["name"].(string); ok {
						existingNames = append(existingNames, n)
					}
				}
				name = defaultInstanceName(existingNames)
			}

			body := buildCreateInstanceBody(version, region, name, _type, cloudProvider, customerManagedKeyId, memory, vectorOptimized, graphAnalyticsPlugin, resolvedProjectID)

			fmt.Fprintln(errOut, "Creating instance...") //nolint:errcheck // narration to stderr; write errors are not actionable

			instance, err := createAndStoreInstance(cfg, body, credentialOptions{
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
			if _, err := api.PollInstance(cfg, instanceID, api.InstanceStatusCreating); err != nil {
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

			printDeployResult(cmd, cfg, instance, deployStatus, noCredentialPrint, noCredentialStorage)

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
	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'project get' to see the full list of supported regions for your project.")
	cmd.Flags().Var(&memory, memoryFlag, "The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes.")
	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01).")

	cmd.Flags().Var(&_type, typeFlag, `(required) The type of the instance. Must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds".`)
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

// printDeployResult renders the instance fields plus the discrete deploy_status
// field, renaming tenant_id -> project_id like instance create does.
func printDeployResult(cmd *cobra.Command, cfg *clicfg.Config, instance map[string]any, deployStatus string, noCredentialPrint, noCredentialStorage bool) {
	if noCredentialPrint {
		delete(instance, "password")
	}
	instance["deploy_status"] = deployStatus

	renamed := utils.RenameResponseField(api.NewSingleValueResponseData(instance), "tenant_id", "project_id")
	renamedInstance, _ := renamed.GetSingleOrError()

	fields := []string{"id", "name", "project_id", "connection_url", "username"}
	if !noCredentialPrint {
		fields = append(fields, "password")
	}
	if !noCredentialStorage {
		fields = append(fields, "credential_name")
	}
	fields = append(fields, "cloud_provider", "region", "type", "deploy_status")

	output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(renamedInstance), fields)
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
