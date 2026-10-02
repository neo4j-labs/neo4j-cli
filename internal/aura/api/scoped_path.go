// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import "fmt"

// The following builders are the single source of truth for the v2beta1
// org/project-scoped resource paths. Every call site (instance list/create/
// get/delete, graph-analytics session ops, and the poll helpers) routes
// through them so the path prefix and scoping shape live in one place.

// ScopedInstancesPath returns the v2beta1 org/project-scoped instances
// collection path.
func ScopedInstancesPath(orgID, projectID string) string {
	return fmt.Sprintf("/organizations/%s/projects/%s/instances", orgID, projectID)
}

// ScopedInstancePath returns the v2beta1 org/project-scoped path for a single
// instance.
func ScopedInstancePath(orgID, projectID, instanceID string) string {
	return fmt.Sprintf("%s/%s", ScopedInstancesPath(orgID, projectID), instanceID)
}

// ScopedSessionsPath returns the v2beta1 org/project-scoped graph-analytics
// sessions collection path.
func ScopedSessionsPath(orgID, projectID string) string {
	return fmt.Sprintf("/organizations/%s/projects/%s/graph-analytics/sessions", orgID, projectID)
}

// ScopedSessionPath returns the v2beta1 org/project-scoped path for a single
// graph-analytics session.
func ScopedSessionPath(orgID, projectID, sessionID string) string {
	return fmt.Sprintf("%s/%s", ScopedSessionsPath(orgID, projectID), sessionID)
}

// ScopedVirtualGraphsPath returns the v2beta1 org/project-scoped virtual-graphs
// collection path.
func ScopedVirtualGraphsPath(orgID, projectID string) string {
	return fmt.Sprintf("/organizations/%s/projects/%s/virtual-graphs", orgID, projectID)
}

// ScopedVirtualGraphPath returns the v2beta1 org/project-scoped path for a
// single virtual graph.
func ScopedVirtualGraphPath(orgID, projectID, virtualGraphID string) string {
	return fmt.Sprintf("%s/%s", ScopedVirtualGraphsPath(orgID, projectID), virtualGraphID)
}

// ScopedVirtualGraphAllowedConfigsPath returns the v2beta1 org/project-scoped
// path listing the memory configurations selectable for a virtual graph.
func ScopedVirtualGraphAllowedConfigsPath(orgID, projectID string) string {
	return fmt.Sprintf("%s/allowed-configs", ScopedVirtualGraphsPath(orgID, projectID))
}
