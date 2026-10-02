// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// ValidateResourceID rejects an ID that would break out of, or malform, the
// URL path segment it is interpolated into (empty, "." / "..", or containing a
// path, query, fragment or percent-escape character).
func ValidateResourceID(resourceType, id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\?#%`) {
		return clierr.NewValidationError("invalid %s id %q", resourceType, id)
	}
	return nil
}
