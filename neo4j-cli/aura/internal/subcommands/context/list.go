// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package context

import (
	"encoding/json"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

// contextEntry represents a single org/project pair in the context list output.
type contextEntry struct {
	Context        string `json:"context"`
	OrganizationId string `json:"organizationId"`
	ProjectId      string `json:"projectId"`
	ProjectName    string `json:"projectName"`
	Default        bool   `json:"default"`
}

// contextListResponse implements output.ResponseData for rendering context list output.
type contextListResponse struct {
	Data []contextEntry `json:"data"`
}

func (r contextListResponse) AsArray() []map[string]any {
	result := make([]map[string]any, len(r.Data))
	for i, e := range r.Data {
		result[i] = map[string]any{
			"context":        e.Context,
			"organizationId": e.OrganizationId,
			"projectId":      e.ProjectId,
			"projectName":    e.ProjectName,
			"default":        e.Default,
		}
	}
	return result
}

func (r contextListResponse) MarshalJSON() ([]byte, error) {
	type alias contextListResponse
	return json.Marshal(alias(r))
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns a flat list of all accessible organization/project contexts",
		Long: `This subcommand lists all organization/project pairs accessible to the current user.
Each entry includes the context slug ({organizationId}/{projectId}), the organization and
project IDs and names, and whether this entry is the currently active default context.`,
		Example: `# List all accessible contexts in table format
neo4j-cli aura context list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura context list --format json

# Find the active context via jq
neo4j-cli aura context list --format json | jq -r '.data[] | select(.default == true) | .context'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			orgs, err := api.ListOrganizations(cfg)
			if err != nil {
				return fmt.Errorf("failed to list organizations: %w", err)
			}

			defaultContext := cfg.Aura.DefaultContext()

			var entries []contextEntry
			for _, org := range orgs.Data {
				projects, err := api.ListProjects(cfg, org.Id)
				if err != nil {
					return fmt.Errorf("failed to list projects for organization %s: %w", org.Id, err)
				}
				for _, proj := range projects.Data {
					slug := org.Id + "/" + proj.Id
					entries = append(entries, contextEntry{
						Context:        slug,
						OrganizationId: org.Id,
						ProjectId:      proj.Id,
						ProjectName:    proj.Name,
						Default:        defaultContext != "" && slug == defaultContext,
					})
				}
			}

			if entries == nil {
				entries = []contextEntry{}
			}

			output.PrintBodyMap(cmd, cfg, contextListResponse{Data: entries}, []string{"context", "organizationId", "projectId", "projectName", "default"})

			return nil
		},
	}
}
