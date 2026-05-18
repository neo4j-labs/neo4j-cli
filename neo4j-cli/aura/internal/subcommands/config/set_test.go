// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestSetConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config set auth-url test --rw")

	helper.AssertConfigValue("aura.auth-url", "test")
}

func TestSetConfigWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand(fmt.Sprintf("config set %s\"\n\" test --rw", "auth-url"))

	helper.AssertConfigValue("aura.auth-url", "test")
}

// Regression guard for REQ-F-007: passing `auth-url\n` must NOT
// produce an "invalid argument" error from the manual config-key
// validation; the key is trimmed inside the Args func before
// IsValidConfigKey runs.
func TestSetConfigTrailingNewlineDoesNotSurfaceInvalidArgument(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand(fmt.Sprintf("config set %s\"\n\" test --rw", "auth-url"))

	helper.AssertErr("")
}

func TestSetConfigWithInvalidConfigKey(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config set invalid test --rw")

	helper.AssertErr("Error: invalid config key specified: invalid")
}

func TestSetConfigWithInvalidFormatValue(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config set format invalid --rw")

	// format is a valid global key; the error is about the invalid value, not the key
	helper.AssertErr("Error: invalid value for 'format': invalid (valid values: default, json, table, toon)")
}

func TestSetBetaEnabledConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config set beta-enabled true --rw")

	helper.AssertErr("Error: invalid config key specified: beta-enabled")

	helper.ExecuteCommand("config set beta-enabled false --rw")

	helper.AssertErr("Error: invalid config key specified: beta-enabled")
}

func TestSetDefaultWorkspace(t *testing.T) {
	const orgID = "org-111"
	const projectID = "proj-222"
	const slug = orgID + "/" + projectID

	for _, tc := range []struct {
		name        string
		slug        string
		status      int
		body        string
		wantContext string
		wantErr     string
	}{
		{
			name:        "success persists default-workspace",
			slug:        slug,
			status:      http.StatusOK,
			body:        fmt.Sprintf(`{"data": [{"id": "%s", "name": "My Project"}]}`, projectID),
			wantContext: slug,
		},
		{
			name:    "project not found in list returns clear error",
			slug:    slug,
			status:  http.StatusOK,
			body:    `{"data": [{"id": "other-proj", "name": "Other Project"}]}`,
			wantErr: fmt.Sprintf("project %q not found in organization %q", projectID, orgID),
		},
		{
			name:    "404 from list API returns error without persisting",
			slug:    slug,
			status:  http.StatusNotFound,
			body:    `{"errors": [{"message": "organization not found"}]}`,
			wantErr: "organization not found",
		},
		{
			name:    "API error returns error without persisting",
			slug:    slug,
			status:  http.StatusInternalServerError,
			body:    `{"errors": [{"message": "internal server error"}]}`,
			wantErr: "internal server error",
		},
		{
			name:    "invalid slug without slash returns error",
			slug:    "noslash",
			wantErr: "expected format {organizationId}/{projectId}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetConfigValue("flag.aura-beta", true)

			if tc.status != 0 {
				mockHandler := helper.NewRequestHandlerMock(
					fmt.Sprintf("/v2beta1/organizations/%s/projects", orgID),
					tc.status,
					tc.body,
				)

				helper.ExecuteCommand(fmt.Sprintf("config set default-workspace %s --rw", tc.slug))

				if tc.wantErr != "" {
					helper.AssertErrContainsStrings([]string{tc.wantErr})
					return
				}

				helper.AsssertOk()
				helper.AssertConfigValue("aura.default-workspace", tc.wantContext)
				mockHandler.AssertCalledTimes(1)
				mockHandler.AssertCalledWithMethod(http.MethodGet)
				return
			}

			helper.ExecuteCommand(fmt.Sprintf("config set default-workspace %s --rw", tc.slug))

			if tc.wantErr != "" {
				helper.AssertErrContainsStrings([]string{tc.wantErr})
			}
		})
	}
}
