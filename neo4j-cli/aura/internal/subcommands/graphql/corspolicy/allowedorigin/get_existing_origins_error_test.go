// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddAllowedOrigin_GetExistingOriginsDoesNotPanic is the CLI-233
// regression pin for getExistingOrigins: pre-fix, an unexpected 2xx status or
// a malformed CORS body panicked the command. Reverting utils.go to panic on
// either branch causes this test to fail.
func TestAddAllowedOrigin_GetExistingOriginsDoesNotPanic(t *testing.T) {
	tests := map[string]struct {
		statusCode int
		body       string
		wantMsg    string
	}{
		"202 accepted status": {
			statusCode: http.StatusAccepted,
			body:       `{}`,
			wantMsg:    "202",
		},
		"204 no content status": {
			statusCode: http.StatusNoContent,
			body:       ``,
			wantMsg:    "204",
		},
		"200 malformed body": {
			statusCode: http.StatusOK,
			body:       `not valid json`,
			wantMsg:    "not valid json",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)
			helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), tc.statusCode, tc.body)

			var err error
			require.NotPanics(t, func() {
				err = helper.ExecuteCommandE(fmt.Sprintf("graphql cors-policy allowed-origin add %s --instance-id %s --data-api-id %s --organization-id %s --project-id %s --rw", allowedOrigin, instanceId, dataApiId, testOrgID, testProjectID))
			})

			require.Error(t, err)

			var ce *clierr.CLIError
			require.ErrorAs(t, err, &ce)
			assert.Contains(t, clierr.Codes, ce.Code)
			assert.Equal(t, 8, ce.Code)
			assert.NotContains(t, ce.Error(), "please report")
			assert.Contains(t, ce.Error(), tc.wantMsg)
		})
	}
}
