// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tenant

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var organizationId string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of tenants",
		Long: `This subcommand returns a list containing a summary of each of your Aura Tenants.

When --organization-id is provided (or a default organization ID is stored on the active credential),
the request is scoped to that organization via GET v2beta1/organizations/{orgId}/projects.
Otherwise GET v1/tenants is used, which returns all tenants visible to the credential.

To find out more about a specific Tenant, retrieve the details using the get subcommand.`,
		Example: `# List all tenants the current user has access to
neo4j-cli aura tenant list

# List tenants in a specific organization
neo4j-cli aura tenant list --organization-id 3d6481bf-2df1-47cf-8392-0288b1ac215f

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura tenant list --format json

# Pipe tenant ids through jq for a follow-up command
neo4j-cli aura tenant list --format json | jq -r '.data[].id'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Fall back to the credential's stored default org when the flag is not set.
			if organizationId == "" {
				var cred *credentials.AuraCredential
				if active := cfg.Aura.ActiveCredential(); active != nil {
					cred = active
				} else if def, err := cfg.Credentials.Aura.GetDefault(); err == nil {
					cred = def
				}
				if cred != nil {
					organizationId = cred.OrganizationId
				}
			}

			var (
				path    string
				version api.AuraApiVersion
			)
			if organizationId != "" {
				path = "/organizations/" + organizationId + "/projects"
				version = api.AuraApiVersionBeta2
			} else {
				path = "/tenants"
				version = api.AuraApiVersion1
			}

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodGet,
				Version: version,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name"})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID to scope the tenant list; falls back to the default stored on the active credential")

	return cmd
}
