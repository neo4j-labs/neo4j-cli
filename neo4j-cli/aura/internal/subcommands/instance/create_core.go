// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"io"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
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
// them for free-db), validates the version, and enforces the credential-flag
// rules. Callers layer their own leaf-specific checks around it (create adds
// the --graph-analytics-plugin rule, deploy adds the --database system reject).
func validateInstanceFlags(cmd *cobra.Command, cfg *clicfg.Config, f instanceFlags) error {
	if f.instanceType != "free-db" {
		cmd.MarkFlagRequired("memory")         //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
		cmd.MarkFlagRequired("region")         //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
		cmd.MarkFlagRequired("cloud-provider") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	} else {
		if f.memory != "" {
			return fmt.Errorf(`invalid argument "%s" for "--memory" flag: must not be set when "--type" flag is set to "free-db"`, f.memory)
		}
		if f.region != "" {
			return fmt.Errorf(`invalid argument "%s" for "--region" flag: must not be set when "--type" flag is set to "free-db"`, f.region)
		}
		if f.cloudProvider != "" {
			return fmt.Errorf(`invalid argument "%s" for "--cloud-provider" flag: must not be set when "--type" flag is set to "free-db"`, f.cloudProvider)
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

// resolveInstanceName returns the explicit name when non-empty, otherwise it
// lists the project's instances (via the v2beta1 org/project-scoped path) and
// derives an unused default name (e.g. Instance01). Shared by the create and
// deploy leaves' auto-naming.
func resolveInstanceName(cfg *clicfg.Config, name, orgID, projectID string) (string, error) {
	if name != "" {
		return name, nil
	}

	listBody, _, listErr := api.MakeRequest(cfg, api.ScopedInstancesPath(orgID, projectID), &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion2,
	})
	if listErr != nil {
		return "", listErr
	}
	listData := api.ParseBody(listBody)
	existingNames := make([]string, 0, len(listData.AsArray()))
	for _, inst := range listData.AsArray() {
		if n, ok := inst["name"].(string); ok {
			existingNames = append(existingNames, n)
		}
	}
	return defaultInstanceName(existingNames), nil
}

// renderInstanceResult prints the standard instance result fields, renaming
// tenant_id -> project_id like the Aura API output convention. password is
// omitted when noCredentialPrint is set, credential_name when noCredentialStorage
// is set, and any extraFields are appended after the trailing cloud/region/type
// columns (deploy uses this for deploy_status).
func renderInstanceResult(cmd *cobra.Command, cfg *clicfg.Config, instance map[string]any, noCredentialPrint, noCredentialStorage bool, extraFields ...string) {
	if noCredentialPrint {
		delete(instance, "password")
	} else if pw, ok := instance["password"].(string); ok {
		// The password is printed (once) for the user, but on a later --wait
		// failure the captured output is teed to disk. Register the literal value
		// so tee redaction scrubs it from formats the shape-based regexes can't
		// reach (notably the table-cell layout).
		clievents.RegisterSecretValue(pw)
	}

	renamed := utils.RenameResponseField(api.NewSingleValueResponseData(instance), "tenant_id", "project_id")
	renamedInstance, _ := renamed.GetSingleOrError()

	fields := []string{"id", "name", "project_id", "connection_url", "username"}
	if !noCredentialPrint {
		fields = append(fields, "password")
	}
	if !noCredentialStorage {
		fields = append(fields, "credential_name")
	}
	fields = append(fields, "cloud_provider", "region", "type")
	fields = append(fields, extraFields...)

	output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(renamedInstance), fields)
}

// buildCreateInstanceBody assembles the POST /instances request body from the
// already-validated create flag values and the resolved project id. free-db
// targets ignore the caller's memory/region/cloud-provider/version and use
// fixed defaults, matching the Aura free-tier contract.
func buildCreateInstanceBody(
	version string,
	region string,
	name string,
	_type flags.InstanceType,
	cloudProvider flags.CloudProvider,
	customerManagedKeyId string,
	memory flags.Memory,
	vectorOptimized bool,
	graphAnalyticsPlugin bool,
	resolvedProjectID string,
) map[string]any {
	body := map[string]any{
		"version":        version,
		"region":         region,
		"name":           name,
		"type":           _type,
		"cloud_provider": cloudProvider,
		"tenant_id":      resolvedProjectID,
	}

	if _type == "free-db" {
		body["memory"] = "1GB"
		body["region"] = "europe-west1"
		body["cloud_provider"] = "gcp"
		body["version"] = "5"
	} else {
		body["memory"] = memory
		body["region"] = region
		body["vector_optimized"] = vectorOptimized
	}

	if _type == "professional-db" {
		body["graph_analytics_plugin"] = graphAnalyticsPlugin
	}

	if customerManagedKeyId != "" {
		body["customer_managed_key_id"] = customerManagedKeyId
	}

	return body
}

// credentialOptions carries the credential-storage flag values used when
// persisting the new instance's generated credentials.
type credentialOptions struct {
	// instanceType is the resolved --type value; it determines the stored
	// database name for free-db instances.
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

// createAndStoreInstance POSTs the create body to /instances, parses the single
// instance from the response, and (unless credOpts.noCredentialStorage) stores
// the generated dbms credential, recording the resolved credential name under
// the "credential_name" key of the returned instance map.
//
// A status other than 202 Accepted or 200 OK yields a nil map and nil error
// (mirroring the historic create behaviour, where only those paths produce
// output).
func createAndStoreInstance(cfg *clicfg.Config, body map[string]any, orgID, projectID string, credOpts credentialOptions) (map[string]any, error) {
	resBody, statusCode, err := api.MakeRequest(cfg, api.ScopedInstancesPath(orgID, projectID), &api.RequestConfig{
		PostBody: body,
		Method:   http.MethodPost,
		Version:  api.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}

	// NOTE: Instance create should not return OK (200), it always returns 202, checking both just in case
	if statusCode != http.StatusAccepted && statusCode != http.StatusOK {
		return nil, nil
	}

	responseData := api.ParseBody(resBody)
	instance, err := responseData.GetSingleOrError()
	if err != nil {
		return nil, err
	}

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
