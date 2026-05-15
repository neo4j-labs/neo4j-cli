// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package context

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
)

// ValidateAndSetDefaultContext parses an "{organizationId}/{projectId}" slug,
// validates the pair exists via the v2beta1 API, and on success writes
// aura.default-context to config.
//
// It returns an error when:
//   - the slug contains no '/'
//   - the organization or project portion is empty
//   - the list projects API call fails
//   - the project ID is not found in the organization's project list
func ValidateAndSetDefaultContext(cfg *clicfg.Config, slug string) error {
	idx := strings.Index(slug, "/")
	if idx < 0 {
		return fmt.Errorf("invalid context %q: expected format {organizationId}/{projectId}", slug)
	}

	orgID := slug[:idx]
	projectID := slug[idx+1:]

	if orgID == "" {
		return fmt.Errorf("invalid context %q: organization ID must not be empty", slug)
	}
	if projectID == "" {
		return fmt.Errorf("invalid context %q: project ID must not be empty", slug)
	}

	projects, err := api.ListProjects(cfg, orgID)
	if err != nil {
		return fmt.Errorf("failed to validate context %q: %w", slug, err)
	}

	found := false
	for _, p := range projects.Data {
		if p.Id == projectID {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("project %q not found in organization %q", projectID, orgID)
	}

	cfg.Aura.Set("default-context", slug)
	return nil
}
