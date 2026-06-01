// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"io"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
)

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
func createAndStoreInstance(cfg *clicfg.Config, body map[string]any, credOpts credentialOptions) (map[string]any, error) {
	resBody, statusCode, err := api.MakeRequest(cfg, "/instances", &api.RequestConfig{
		PostBody: body,
		Method:   http.MethodPost,
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
