// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset_test

import "fmt"

const (
	testOrgID     = "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	testProjectID = "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	testDatasetID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
)

// datasetsPath is the v2beta1 org/project-scoped evaluation-datasets collection path.
func datasetsPath() string {
	return fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/evaluation-datasets", testOrgID, testProjectID)
}

// datasetPath is the collection path plus one dataset id.
func datasetPath(id string) string {
	return datasetsPath() + "/" + id
}
