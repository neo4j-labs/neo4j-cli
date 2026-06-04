// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/spf13/cobra"
)

// newListCmd builds the `neo4j-cli dataset list` leaf. It renders the curated
// suggestion set (slug, title, description, repo) honoring --format. The data
// is embedded in the binary, so the leaf performs no network access.
func newListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List curated example dataset suggestions",
		Long: "List a curated set of suggested example datasets, each with a slug, title, " +
			"description, and GitHub `<owner>/<repo>`. Pass a repo to one of the load verbs " +
			"(`docker load`, `desktop dbms load`, `aura instance load`). The suggestions are " +
			"not a constraint — any repo carrying a relate.project-install.json manifest works.",
		Example: `# List dataset suggestions as a table
neo4j-cli dataset list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli dataset list --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows := make([]map[string]any, 0, len(dataset.List()))
			for _, s := range dataset.List() {
				rows = append(rows, map[string]any{
					"slug":        s.Slug,
					"title":       s.Title,
					"description": s.Description,
					"repo":        s.OwnerRepo,
				})
			}

			fields := []string{"slug", "title", "description", "repo"}
			commonoutput.PrintBodyMap(cmd, cfg, listRows(rows), fields)
			return nil
		},
	}

	return cmd
}

// listRows adapts a []map[string]any into a commonoutput.ResponseData and
// renders the empty case as `[]` rather than `null`.
type listRows []map[string]any

// AsArray implements commonoutput.ResponseData.
func (r listRows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

// MarshalJSON returns the JSON array form so the empty case renders as `[]`.
func (r listRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}
