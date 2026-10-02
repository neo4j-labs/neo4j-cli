// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package workspace

import (
	"context"
	"fmt"
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
)

// ValidateAndSetDefaultWorkspace parses an "{organizationId}/{projectId}" slug,
// validates the pair exists via the v2beta1 API, and on success writes
// aura.default-workspace to config.
//
// It returns an error when:
//   - the slug contains no '/'
//   - the organization or project portion is empty
//   - the list projects API call fails
//   - the project ID is not found in the organization's project list
func ValidateAndSetDefaultWorkspace(ctx context.Context, cfg *clicfg.Config, slug string) error {
	idx := strings.Index(slug, "/")
	if idx < 0 {
		return fmt.Errorf("invalid workspace %q: expected format {organizationId}/{projectId}", slug)
	}

	orgID := slug[:idx]
	projectID := slug[idx+1:]

	if orgID == "" {
		return fmt.Errorf("invalid workspace %q: organization ID must not be empty", slug)
	}
	if projectID == "" {
		return fmt.Errorf("invalid workspace %q: project ID must not be empty", slug)
	}

	projects, err := auraclient.New(cfg).Projects().List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to validate workspace %q: %w", slug, err)
	}

	found := false
	for _, p := range projects {
		if p.ID == projectID {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("project %q not found in organization %q", projectID, orgID)
	}

	cfg.Aura.Set("default-workspace", slug)
	return nil
}
