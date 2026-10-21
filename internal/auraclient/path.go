// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import "fmt"

// scopedResourcePath builds the v2beta1 project-scoped path of a resource
// collection, one of its members, or a member sub-resource:
//
//	/organizations/{org}/projects/{project}/{resource}[/{id}[/suffix...]]
//
// resource is the collection's path segment below the project ("agents",
// "agents/evaluation-datasets", ...) and idType names the resource in a
// validation error ("agent", "evaluation-dataset", ...). An empty id omits the
// member segment (the collection path) and is not validated; every ID that
// does land in the path — the scope's org and project included — is checked
// with ValidateResourceID, so a malformed one is a usage error before any
// request is made.
func scopedResourcePath(scope Scope, resource, idType, id string, suffix ...string) (string, error) {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return "", err
	}
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/organizations/%s/projects/%s/%s", scope.OrgID, scope.ProjectID, resource)
	if id != "" {
		if err := ValidateResourceID(idType, id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	for _, sfx := range suffix {
		p += "/" + sfx
	}
	return p, nil
}
