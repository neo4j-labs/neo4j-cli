// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun_test

import "fmt"

const (
	testOrgID     = "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	testProjectID = "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	testRunID     = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	testAgentID   = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	testDatasetID = "b1b2c3d4-e5f6-7890-abcd-ef1234567890"
)

// evalRunsPath is the v2beta1 scoped collection path for evaluation runs.
func evalRunsPath() string {
	return fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/evaluation-runs", testOrgID, testProjectID)
}

// evalRunPath is the v2beta1 scoped path of one evaluation run.
func evalRunPath(id string) string {
	return evalRunsPath() + "/" + id
}
