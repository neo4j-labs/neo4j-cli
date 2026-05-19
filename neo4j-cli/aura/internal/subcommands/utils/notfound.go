// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import (
	"errors"

	"github.com/neo4j/cli/common/clierr"
)

// WithNotFoundContext rewrites a not-found (*clierr.CLIError with Code == 3)
// in place to overwrite ResourceType, ResourceID and Suggestion, and returns
// the same error. Non-matching errors (nil, non-CLIError, or CLIError with a
// different Code) pass through unchanged.
//
// Used at call sites where api.parseResourceFromRequest cannot extract the
// correct resource segment from a nested URL path (e.g. instance snapshots
// and graph-analytics sessions), so the caller knows the real resource type
// and can attach a useful next-action hint.
func WithNotFoundContext(err error, resourceType, resourceID, suggestion string) error {
	if err == nil {
		return nil
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		return err
	}
	if ce.Code != 3 {
		return err
	}
	ce.ResourceType = resourceType
	ce.ResourceID = resourceID
	ce.Suggestion = suggestion
	return err
}
