// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

// workspaceEntry represents a single org/project pair in the workspace list output.
type workspaceEntry struct {
	Workspace      string `json:"workspace"`
	OrganizationId string `json:"organizationId"`
	ProjectId      string `json:"projectId"`
	ProjectName    string `json:"projectName"`
	Default        bool   `json:"default"`
}

// workspaceListResponse implements output.ResponseData for rendering workspace list output.
type workspaceListResponse struct {
	Data []workspaceEntry `json:"data"`
}

func (r workspaceListResponse) AsArray() []map[string]any {
	result := make([]map[string]any, len(r.Data))
	for i, e := range r.Data {
		result[i] = map[string]any{
			"workspace":      e.Workspace,
			"organizationId": e.OrganizationId,
			"projectId":      e.ProjectId,
			"projectName":    e.ProjectName,
			"default":        e.Default,
		}
	}
	return result
}

func (r workspaceListResponse) MarshalJSON() ([]byte, error) {
	type alias workspaceListResponse
	return json.Marshal(alias(r))
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns a flat list of all accessible organization/project workspaces",
		Long: `This subcommand lists all organization/project pairs accessible to the current user.
Each entry includes the workspace slug ({organizationId}/{projectId}), the organization and
project IDs and names, and whether this entry is the currently active default workspace.`,
		Example: `# List all accessible workspaces in table format
neo4j-cli aura workspace list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura workspace list --format json

# Find the active workspace via jq
neo4j-cli aura workspace list --format json | jq -r '.data[] | select(.default == true) | .workspace'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			orgs, err := api.ListOrganizations(cfg)
			if err != nil {
				return fmt.Errorf("failed to list organizations: %w", err)
			}

			defaultWorkspace := cfg.Aura.DefaultWorkspace()

			var entries []workspaceEntry
			for _, org := range orgs.Data {
				projects, err := api.ListProjects(cfg, org.Id)
				if err != nil {
					return fmt.Errorf("failed to list projects for organization %s: %w", org.Id, err)
				}
				for _, proj := range projects.Data {
					slug := org.Id + "/" + proj.Id
					entries = append(entries, workspaceEntry{
						Workspace:      slug,
						OrganizationId: org.Id,
						ProjectId:      proj.Id,
						ProjectName:    proj.Name,
						Default:        defaultWorkspace != "" && slug == defaultWorkspace,
					})
				}
			}

			if entries == nil {
				entries = []workspaceEntry{}
			}

			output.PrintBodyMap(cmd, cfg, workspaceListResponse{Data: entries}, []string{"workspace", "organizationId", "projectId", "projectName", "default"})

			return nil
		},
	}
}
