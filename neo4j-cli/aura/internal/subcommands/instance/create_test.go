// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/instance"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/require"
)

const (
	testCreateOrgID     = "test-org-id"
	testCreateProjectID = "YOUR_TENANT_ID"
)

// registerCreateProjectsMock registers a v2beta1 projects mock for the standard create test org/project.
func registerCreateProjectsMock(helper *testutils.AuraTestHelper) {
	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testCreateOrgID+"/projects",
		http.StatusOK,
		`{"data": [{"id": "`+testCreateProjectID+`", "name": "Test Project"}]}`,
	)
}

func TestCreateFreeInstanceRequiresRw(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "free-db",
				"name": "Instance01"
			}
		}`)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID)

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: this command writes; pass --rw to allow it")
}

func TestCreateFreeInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "alice123",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "free-db",
				"name": "Instance01"
			}
		}`)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"1GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`)

	helper.AssertErr("")
	// For free-db with a non-"neo4j" username, the database name stored in credentials is the username.
	helper.AssertCredentialsValue("dbms.credentials.0.database-name", "alice123")
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "alice123"
	  }
	}`)
}

func TestCreateProfessionalInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "professional-db",
				"name": "Instance01",
    			"vector_optimized": false
			}
		}`)

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --cloud-provider gcp --memory 4GB --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"4GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"professional-db","version":"5","vector_optimized":false,"graph_analytics_plugin":false}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "professional-db",
		"username": "neo4j",
    	"vector_optimized": false
	  }
	}`)
}

func TestCreateProfessionalInstanceVectorOptimizedGraphAnalyticsPlugin(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "professional-db",
				"name": "Instance01",
    			"vector_optimized": true
			}
		}`)

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --cloud-provider gcp --memory 4GB --vector-optimized --graph-analytics-plugin --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"4GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"professional-db","version":"5","vector_optimized":true,"graph_analytics_plugin":true}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "professional-db",
		"username": "neo4j",
		"vector_optimized": true
	  }
	}`)
}

func TestCreateProfessionalInstanceNoMemory(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --cloud-provider gcp --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: required flag(s) "memory" not set
`)
}

func TestCreateInstanceMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestCreateInstanceMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestCreateProfessionalInstanceInvalidCloudProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 1GB --cloud-provider invalid --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "invalid" for "--cloud-provider" flag: must be one of "aws", "azure", or "gcp"
`)
}

func TestCreateProfessionalInstanceInvalidMemory(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 3GB --cloud-provider gcp --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "3GB" for "--memory" flag: must be one of "1GB", "2GB", "4GB", "8GB", "16GB", "24GB", "32GB", "48GB", "64GB", "128GB", "192GB", "256GB", "384GB", or "512GB"
`)
}

func TestCreateProfessionalInstanceInvalidInstanceType(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type invalid-db --memory 1GB --cloud-provider gcp --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "invalid-db" for "--type" flag: must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds"
`)
}

func TestCreateProfessionalInstanceInvalidVersion(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 1GB --cloud-provider gcp --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --version 6 --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "6" for "--version" flag: must be one of "4" or "5"
`)
}

func TestCreateFreeInstanceWithMemory(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --memory 1GB --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "1GB" for "--memory" flag: must not be set when "--type" flag is set to "free-db"
`)
}

func TestCreateFreeInstanceWithRegion(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "europe-west1" for "--region" flag: must not be set when "--type" flag is set to "free-db"
`)
}

func TestCreateFreeInstanceWithCloudProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --cloud-provider gcp --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "gcp" for "--cloud-provider" flag: must not be set when "--type" flag is set to "free-db"
`)
}

func TestCreateFreeInstanceWithGraphAnalyticsPlugin(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --graph-analytics-plugin --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: "--graph-analytics-plugin" flag can only be set when "--type" flag is set to "professional-db"
`)
}

func TestCreateInstanceError(t *testing.T) {
	testCases := []struct {
		statusCode    int
		expectedError string
		returnBody    string
	}{
		{
			statusCode:    http.StatusBadRequest,
			expectedError: "Error: [You must provide billing details in the Aura Console before creating an instance]",
			returnBody: `{
				"errors": [
					{
					"message": "You must provide billing details in the Aura Console before creating an instance",
					"reason": "missing-billing-details"
					}
				]
			}`,
		},
		{
			statusCode:    http.StatusMethodNotAllowed,
			expectedError: "Error: [string]",
			returnBody: `{
				"errors": [
					{
					"message": "string",
					"reason": "string",
					"field": "string"
					}
				]
			}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("StatusCode%d", testCase.statusCode), func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerCreateProjectsMock(&helper)

			mockHandler := helper.NewRequestHandlerMock("/v1/instances", testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --cloud-provider gcp --memory 4GB --rw")

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPost)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}

func TestInstanceWithCmkId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "enterprise-db",
				"name": "Instance01"
			}
		}`)

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type enterprise-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --cloud-provider gcp --memory 16GB --customer-managed-key-id UUID_OF_YOUR_KEY --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"16GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"enterprise-db","version":"5","customer_managed_key_id":"UUID_OF_YOUR_KEY","vector_optimized":false}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "enterprise-db",
		"username": "neo4j"
	  }
	} `)
}

func TestCreateFreeInstanceWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testCreateOrgID, testCreateProjectID)

	registerCreateProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "free-db",
				"name": "Instance01"
			}
		}`)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"1GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateFreeInstanceWithWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	createMock := helper.NewRequestHandlerMock("POST /v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "free-db",
				"name": "Instance01"
			}
		}`)

	getMock := helper.NewRequestHandlerMock("GET /v1/instances/db1d1234", http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "creating"
			}
		}`).AddResponse(http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "ready"
			}
		}`)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --wait --rw")

	createMock.AssertCalledTimes(1)
	createMock.AssertCalledWithMethod(http.MethodPost)
	createMock.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"1GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`)

	getMock.AssertCalledTimes(2)
	getMock.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
{
	"data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	}
}
	`)
	helper.AssertErrContainsStrings([]string{
		"Waiting for instance to be ready...",
		"Instance Status: ready",
	})
}

// TestCreateFreeInstanceWithWait_StdoutIsValidJSON is the CLI-82 regression-pin
// for the wait narration: pre-fix, stdout had narration mixed with the JSON
// body and would fail to unmarshal. Reverting any of the Pattern A fmt.Fprintln
// replacements to cmd.Println causes this test to fail.
func TestCreateFreeInstanceWithWait_StdoutIsValidJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	helper.NewRequestHandlerMock("POST /v1/instances", http.StatusAccepted, `{
			"data": {
				"id": "db1d1234",
				"connection_url": "YOUR_CONNECTION_URL",
				"username": "neo4j",
				"password": "letMeIn123!",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"type": "free-db",
				"name": "Instance01"
			}
		}`)

	helper.NewRequestHandlerMock("GET /v1/instances/db1d1234", http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "creating"
			}
		}`).AddResponse(http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "ready"
			}
		}`)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --wait --rw --format json")

	helper.AssertOutIsValidJSON()
}

func TestCreateCredentialFlagValidation(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		expectedErr string
		wantHTTP    int
	}{
		{
			name:        "credential-name and no-credential-storage are mutually exclusive",
			command:     "instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --credential-name myname --no-credential-storage --rw",
			expectedErr: `Error: "--credential-name" and "--no-credential-storage" cannot be used together`,
			wantHTTP:    0,
		},
		{
			name:        "explicit empty credential-name is rejected",
			command:     `instance create --name Instance01 --type free-db --organization-id ` + testCreateOrgID + ` --project-id ` + testCreateProjectID + ` --credential-name "" --rw`,
			expectedErr: `Error: invalid argument "" for "--credential-name" flag: name must not be empty`,
			wantHTTP:    0,
		},
		{
			name:        "valid flags proceed to HTTP call",
			command:     "instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --rw",
			expectedErr: "",
			wantHTTP:    1,
		},
		{
			name:        "credential-name without no-credential-storage proceeds to HTTP call",
			command:     "instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --credential-name myname --rw",
			expectedErr: "",
			wantHTTP:    1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			if tc.wantHTTP > 0 {
				registerCreateProjectsMock(&helper)
			}

			mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, `{
				"data": {
					"id": "db1d1234",
					"connection_url": "YOUR_CONNECTION_URL",
					"username": "neo4j",
					"password": "letMeIn123!",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp",
					"region": "europe-west1",
					"type": "free-db",
					"name": "Instance01"
				}
			}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(tc.wantHTTP)
			if tc.expectedErr != "" {
				helper.AssertErr(tc.expectedErr)
			} else {
				helper.AssertErr("")
			}
		})
	}
}

func TestCreatePreRunERejectsNilDbms(t *testing.T) {
	// credentials.json with "dbms": null causes cfg.Credentials.Dbms to be nil after
	// JSON unmarshal, exercising the "credential storage is not available" guard.
	credentialsJSON := `{
		"aura": {
			"credentials": [{"name": "test-cred", "access-token": "dsa", "token-expiry": 123}],
			"default-credential": "test-cred"
		},
		"dbms": null
	}`

	fs, err := testfs.GetTestFs(`{"format":"json","aura":{"default-workspace":"test-org-id/YOUR_TENANT_ID"}}`, credentialsJSON)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	require.Nil(t, cfg.Credentials.Dbms, "expected Dbms to be nil with 'dbms: null' in credentials file")

	cmd := instance.NewCreateCmd(cfg)
	// Set required flags so PreRunE reaches the Dbms-nil check (not an earlier guard).
	require.NoError(t, cmd.Flags().Set("name", "Instance01"))
	require.NoError(t, cmd.Flags().Set("type", "free-db"))

	err = cmd.PreRunE(cmd, nil)
	require.EqualError(t, err, `credential storage is not available; use --no-credential-storage to skip storing credentials locally`)
}

// createAPIResponse is the shared response body used across credential storage tests.
const createAPIResponse = `{
	"data": {
		"id": "db1d1234",
		"connection_url": "YOUR_CONNECTION_URL",
		"username": "neo4j",
		"password": "letMeIn123!",
		"tenant_id": "YOUR_TENANT_ID",
		"cloud_provider": "gcp",
		"region": "europe-west1",
		"type": "free-db",
		"name": "Instance01"
	}
}`

func TestCreateDefaultCredentialStorage(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	helper.AssertErr("")

	// Verify the credential was stored with the expected name.
	helper.AssertCredentialsValue("dbms.credentials.0.name", "db1d1234-default")
	helper.AssertCredentialsValue("dbms.credentials.0.username", "neo4j")
	helper.AssertCredentialsValue("dbms.credentials.0.uri", "YOUR_CONNECTION_URL")
	helper.AssertCredentialsValue("dbms.credentials.0.database-name", "neo4j")

	// credential_name must appear in JSON output.
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

// TestCreateDatabaseNameStorage verifies that the correct database name is stored
// depending on the instance type and the username returned by the API.
func TestCreateDatabaseNameStorage(t *testing.T) {
	testCases := []struct {
		name             string
		instanceType     string
		apiUsername      string
		command          string
		wantDatabaseName string
	}{
		{
			name:             "free-db with non-neo4j username stores username as database-name",
			instanceType:     "free-db",
			apiUsername:      "tenant-user-abc",
			command:          "instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw",
			wantDatabaseName: "tenant-user-abc",
		},
		{
			name:             "free-db with neo4j username stores neo4j as database-name",
			instanceType:     "free-db",
			apiUsername:      "neo4j",
			command:          "instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw",
			wantDatabaseName: "neo4j",
		},
		{
			name:             "professional-db always stores neo4j as database-name regardless of username",
			instanceType:     "professional-db",
			apiUsername:      "tenant-user-abc",
			command:          "instance create --region europe-west1 --name Instance01 --type professional-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --cloud-provider gcp --memory 4GB --rw",
			wantDatabaseName: "neo4j",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerCreateProjectsMock(&helper)

			helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, fmt.Sprintf(`{
				"data": {
					"id": "db1d1234",
					"connection_url": "YOUR_CONNECTION_URL",
					"username": %q,
					"password": "letMeIn123!",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp",
					"region": "europe-west1",
					"type": %q,
					"name": "Instance01",
					"vector_optimized": false
				}
			}`, tc.apiUsername, tc.instanceType))

			helper.ExecuteCommand(tc.command)

			helper.AssertErr("")
			helper.AssertCredentialsValue("dbms.credentials.0.database-name", tc.wantDatabaseName)
		})
	}
}

func TestCreateCollisionResolution(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	// Pre-populate a DBMS credential with the default name so the new create is forced
	// to use the collision-resolved suffix "-1".
	helper.SetCredentialsValue("dbms.credentials", []map[string]string{
		{
			"name":          "db1d1234-default",
			"username":      "neo4j",
			"password":      "old-pass",
			"database-name": "neo4j",
			"uri":           "old-url",
		},
	})
	helper.SetCredentialsValue("dbms.default-credential", "db1d1234-default")

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --rw")

	helper.AssertErr("")

	// Collision resolved: stored as db1d1234-default-1 (not -default, which already exists).
	helper.AssertCredentialsValue("dbms.credentials.1.name", "db1d1234-default-1")
	helper.AssertCredentialsValue("dbms.credentials.1.username", "neo4j")

	// Output must reflect the resolved name.
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default-1",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateCustomCredentialName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --credential-name myinstance --rw")

	helper.AssertErr("")

	// Stored with the custom name.
	helper.AssertCredentialsValue("dbms.credentials.0.name", "myinstance")

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "myinstance",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateCustomCredentialNameCollision(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	// Pre-populate with the custom name so collision handling kicks in.
	helper.SetCredentialsValue("dbms.credentials", []map[string]string{
		{
			"name":          "myinstance",
			"username":      "neo4j",
			"password":      "old-pass",
			"database-name": "neo4j",
			"uri":           "old-url",
		},
	})
	helper.SetCredentialsValue("dbms.default-credential", "myinstance")

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --credential-name myinstance --rw")

	helper.AssertErr("")

	// Stored as myinstance-1 because myinstance is taken.
	helper.AssertCredentialsValue("dbms.credentials.1.name", "myinstance-1")

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "myinstance-1",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateNoCredentialStorage(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --rw")

	helper.AssertErr("")

	// No DBMS credential must have been stored.
	helper.AssertCredentialsValue("dbms.credentials", "[]")

	// credential_name must be absent; password must be present.
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateNoCredentialPrint(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-print --rw")

	helper.AssertErr("")

	// Credential must still be stored.
	helper.AssertCredentialsValue("dbms.credentials.0.name", "db1d1234-default")

	// password must be absent; credential_name must be present.
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateNoCredentialStorageAndNoPrint(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)
	helper.NewRequestHandlerMock("/v1/instances", http.StatusAccepted, createAPIResponse)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --no-credential-print --rw")

	helper.AssertErr("")

	// No credential stored.
	helper.AssertCredentialsValue("dbms.credentials", "[]")

	// Neither password nor credential_name in output.
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "db1d1234",
		"name": "Instance01",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateCredentialStoredBeforeWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	helper.NewRequestHandlerMock("POST /v1/instances", http.StatusAccepted, createAPIResponse)

	helper.NewRequestHandlerMock("GET /v1/instances/db1d1234", http.StatusOK, `{
		"data": {
			"id": "db1d1234",
			"status": "creating"
		}
	}`).AddResponse(http.StatusOK, `{
		"data": {
			"id": "db1d1234",
			"status": "ready"
		}
	}`)

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --wait --rw")

	helper.AssertErrContainsStrings([]string{
		"Waiting for instance to be ready...",
		"Instance Status: ready",
	})

	// Credential must be stored even though --wait polling followed.
	helper.AssertCredentialsValue("dbms.credentials.0.name", "db1d1234-default")
}

func TestCreateDefaultNameGeneration(t *testing.T) {
	testCases := []struct {
		name             string
		command          string
		listResponseBody string
		listCalledTimes  int
		expectedPostBody string
	}{
		{
			name:    "no existing instances generates Instance01",
			command: "instance create --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --rw",
			listResponseBody: `{
				"data": []
			}`,
			listCalledTimes:  1,
			expectedPostBody: `{"cloud_provider":"gcp","memory":"1GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`,
		},
		{
			name:    "Instance01 already exists generates Instance02",
			command: "instance create --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --rw",
			listResponseBody: `{
				"data": [
					{"id": "abc123", "name": "Instance01", "tenant_id": "YOUR_TENANT_ID"}
				]
			}`,
			listCalledTimes:  1,
			expectedPostBody: `{"cloud_provider":"gcp","memory":"1GB","name":"Instance02","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`,
		},
		{
			name:             "explicit --name skips the list GET call",
			command:          "instance create --name MyInstance --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --rw",
			listResponseBody: `{"data": []}`,
			listCalledTimes:  0,
			expectedPostBody: `{"cloud_provider":"gcp","memory":"1GB","name":"MyInstance","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerCreateProjectsMock(&helper)

			listMock := helper.NewRequestHandlerMock("GET /v1/instances", http.StatusOK, tc.listResponseBody)
			postMock := helper.NewRequestHandlerMock("POST /v1/instances", http.StatusAccepted, `{
				"data": {
					"id": "db1d1234",
					"connection_url": "YOUR_CONNECTION_URL",
					"username": "neo4j",
					"password": "letMeIn123!",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp",
					"region": "europe-west1",
					"type": "free-db",
					"name": "Instance01"
				}
			}`)

			helper.ExecuteCommand(tc.command)

			listMock.AssertCalledTimes(tc.listCalledTimes)
			postMock.AssertCalledTimes(1)
			postMock.AssertCalledWithMethod(http.MethodPost)
			postMock.AssertCalledWithBody(tc.expectedPostBody)
			helper.AssertErr("")
		})
	}
}

func TestCreateDefaultNameListAPIError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerCreateProjectsMock(&helper)

	listMock := helper.NewRequestHandlerMock("GET /v1/instances", http.StatusInternalServerError, `{
		"errors": [
			{
				"message": "internal server error",
				"reason": "server-error"
			}
		]
	}`)
	postMock := helper.NewRequestHandlerMock("POST /v1/instances", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand("instance create --type free-db --organization-id " + testCreateOrgID + " --project-id " + testCreateProjectID + " --no-credential-storage --rw")

	listMock.AssertCalledTimes(1)
	postMock.AssertCalledTimes(0)
	helper.AssertErr("Error: [internal server error]")
}
