// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"fmt"
	"github.com/neo4j/cli/internal/auraclient"
	"io"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/flags"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/spf13/cobra"
)

// instanceFlags carries the create-mirror flag values shared by the create and
// deploy leaves' PreRunE validation. The validator reads these to decide which
// flags to require and which combinations to reject.
type instanceFlags struct {
	instanceType        flags.InstanceType
	memory              flags.Memory
	region              string
	cloudProvider       flags.CloudProvider
	version             string
	credentialName      string
	credentialNameSet   bool
	noCredentialStorage bool
}

// validateInstanceFlags is the shared PreRunE body for the create and deploy
// leaves: it marks the sizing flags required for non-free instances (rejecting
// them for free), validates the version, and enforces the credential-flag
// rules. Callers layer their own leaf-specific checks around it (create adds
// the --graph-analytics-plugin rule, deploy adds the --database system reject).
func validateInstanceFlags(cmd *cobra.Command, cfg *clicfg.Config, f instanceFlags) error {
	if f.instanceType != "free" {
		cmd.MarkFlagRequired("memory")         //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
		cmd.MarkFlagRequired("region")         //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
		cmd.MarkFlagRequired("cloud-provider") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	} else {
		if f.memory != "" {
			return fmt.Errorf(`invalid argument "%s" for "--memory" flag: must not be set when "--type" flag is set to "free"`, f.memory)
		}
		if f.region != "" {
			return fmt.Errorf(`invalid argument "%s" for "--region" flag: must not be set when "--type" flag is set to "free"`, f.region)
		}
		if f.cloudProvider != "" {
			return fmt.Errorf(`invalid argument "%s" for "--cloud-provider" flag: must not be set when "--type" flag is set to "free"`, f.cloudProvider)
		}
	}

	if f.version != "4" && f.version != "5" {
		return fmt.Errorf(`invalid argument "%s" for "--version" flag: must be one of "4" or "5"`, f.version)
	}

	if f.credentialNameSet && f.noCredentialStorage {
		return fmt.Errorf(`"--%s" and "--%s" cannot be used together`, "credential-name", "no-credential-storage")
	}

	if f.credentialNameSet && f.credentialName == "" {
		return fmt.Errorf(`invalid argument "" for "--%s" flag: name must not be empty`, "credential-name")
	}

	if !f.noCredentialStorage && (cfg.Credentials == nil || cfg.Credentials.Dbms == nil) {
		return fmt.Errorf("credential storage is not available; use --%s to skip storing credentials locally", "no-credential-storage")
	}

	return nil
}

// renderInstanceResult prints the standard instance result fields, renaming
// tenant_id -> project_id like the Aura API output convention. password is
// omitted when noCredentialPrint is set, credential_name when noCredentialStorage
// is set, and any extraFields are appended after the trailing cloud/region/type
// columns (deploy uses this for deploy_status).
func renderInstanceResult(cmd *cobra.Command, cfg *clicfg.Config, instance map[string]any, noCredentialPrint, noCredentialStorage bool, extraFields ...string) {
	// The password literal is already registered for redaction by the service
	// that received it, so a later --wait failure cannot tee it to disk.
	if noCredentialPrint {
		delete(instance, "password")
	}

	renamedInstance := auraclient.RenameKey(instance, "tenant_id", "project_id", false)

	fields := []string{"id", "name", "project_id", "connection_url", "username"}
	if !noCredentialPrint {
		fields = append(fields, "password")
	}
	if !noCredentialStorage {
		fields = append(fields, "credential_name")
	}
	fields = append(fields, "cloud_provider", "region", "type")
	fields = append(fields, extraFields...)

	output.PrintRecord(cmd, cfg, renamedInstance, fields)
}

// newInstanceCreate maps the already-validated create flag values to the
// service's create spec. The "type" value is the canonical v2beta1 tier name:
// flags.InstanceType.Set has already normalised any legacy v1 alias.
func newInstanceCreate(
	version string,
	region string,
	name string,
	_type flags.InstanceType,
	cloudProvider flags.CloudProvider,
	customerManagedKeyId string,
	memory flags.Memory,
	vectorOptimized bool,
	graphAnalyticsPlugin bool,
) auraclient.InstanceCreate {
	return auraclient.InstanceCreate{
		Name:                 name,
		Version:              version,
		Region:               region,
		Type:                 string(_type),
		CloudProvider:        string(cloudProvider),
		Memory:               string(memory),
		CustomerManagedKeyID: customerManagedKeyId,
		VectorOptimized:      vectorOptimized,
		GraphAnalyticsPlugin: graphAnalyticsPlugin,
	}
}

// credentialOptions carries the credential-storage flag values used when
// persisting the new instance's generated credentials.
type credentialOptions struct {
	// instanceType is the resolved (canonical) --type value; it determines the
	// stored database name for free instances.
	instanceType string
	// credentialName is the user-supplied --credential-name (may be empty).
	credentialName string
	// noCredentialStorage skips persisting credentials locally when true.
	noCredentialStorage bool
	// noCredentialPrint adjusts the failure-warning wording when true.
	noCredentialPrint bool
	// warnOut receives best-effort warnings (e.g. credential-store failures).
	warnOut io.Writer
}

// createAndStoreInstance creates the instance through the Aura client and
// (unless credOpts.noCredentialStorage) stores the generated dbms credential
// locally, recording the resolved credential name under the "credential_name"
// key of the returned instance record. Storing credentials is CLI state, not
// part of the Aura API, so it lives here rather than in the service.
func createAndStoreInstance(ctx context.Context, cfg *clicfg.Config, scope auraclient.Scope, spec auraclient.InstanceCreate, credOpts credentialOptions) (map[string]any, error) {
	created, err := auraclient.New(cfg).Instances().Create(ctx, scope, spec)
	if err != nil {
		return nil, err
	}
	instance := created.Record

	if !credOpts.noCredentialStorage {
		instanceID, _ := instance["id"].(string)
		username, _ := instance["username"].(string)
		password, _ := instance["password"].(string)
		uri, _ := instance["connection_url"].(string)

		base := baseCredentialName(instanceID, credOpts.credentialName)
		resolvedName := resolveCredentialName(cfg.Credentials.Dbms, base)
		instance["credential_name"] = resolvedName

		if addErr := cfg.Credentials.Dbms.Add(resolvedName, username, password, databaseName(credOpts.instanceType, username), uri); addErr != nil {
			if credOpts.noCredentialPrint {
				fmt.Fprintf(credOpts.warnOut, "Warning: failed to store credentials locally (%s). The password has been omitted from output; reset it via the Aura Console.\n", addErr) //nolint:errcheck // warning to stderr; write errors are not actionable
			} else {
				fmt.Fprintf(credOpts.warnOut, "Warning: failed to store credentials locally (%s). Save the printed password now — it cannot be retrieved later.\n", addErr) //nolint:errcheck // warning to stderr; write errors are not actionable
			}
		}
	}

	return instance, nil
}
