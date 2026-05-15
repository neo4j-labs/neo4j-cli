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
//   - the API call returns an error (including 404)
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

	_, err := api.GetProject(cfg, orgID, projectID)
	if err != nil {
		return fmt.Errorf("failed to validate context %q: %w", slug, err)
	}

	cfg.Aura.Set("default-context", slug)
	return nil
}
