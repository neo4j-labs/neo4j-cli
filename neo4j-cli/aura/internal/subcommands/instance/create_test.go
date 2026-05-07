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

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID")

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: this command writes; pass --rw to allow it")
}

func TestCreateFreeInstance(t *testing.T) {
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

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"1GB","name":"Instance01","region":"europe-west1","tenant_id":"YOUR_TENANT_ID","type":"free-db","version":"5"}`)

	helper.AssertErr("")
	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"credential_name": "db1d1234-default",
		"id": "db1d1234",
		"name": "Instance01",
		"password": "letMeIn123!",
		"region": "europe-west1",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateProfessionalInstance(t *testing.T) {
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
				"type": "professional-db",
				"name": "Instance01",
    			"vector_optimized": false
			}
		}`)

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --tenant-id YOUR_TENANT_ID --cloud-provider gcp --memory 4GB --rw")

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
		"region": "europe-west1",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "professional-db",
		"username": "neo4j",
    	"vector_optimized": false
	  }
	}`)
}

func TestCreateProfessionalInstanceVectorOptimizedGraphAnalyticsPlugin(t *testing.T) {
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
				"type": "professional-db",
				"name": "Instance01",
    			"vector_optimized": true
			}
		}`)

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --tenant-id YOUR_TENANT_ID --cloud-provider gcp --memory 4GB --vector-optimized --graph-analytics-plugin --rw")

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
		"region": "europe-west1",
		"tenant_id": "YOUR_TENANT_ID",
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

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --tenant-id YOUR_TENANT_ID --cloud-provider gcp --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: required flag(s) "memory" not set
`)
}

func TestCreateProfessionalInstanceNoTenant(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 1GB --cloud-provider gcp --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: required flag(s) "tenant-id" not set
`)
}

func TestCreateProfessionalInstanceInvalidCloudProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 1GB --cloud-provider invalid --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "invalid" for "--cloud-provider" flag: must be one of "aws", "azure", or "gcp"
`)
}

func TestCreateProfessionalInstanceInvalidMemory(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 3GB --cloud-provider gcp --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "3GB" for "--memory" flag: must be one of "1GB", "2GB", "4GB", "8GB", "16GB", "24GB", "32GB", "48GB", "64GB", "128GB", "192GB", "256GB", "384GB", or "512GB"
`)
}

func TestCreateProfessionalInstanceInvalidInstanceType(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type invalid-db --memory 1GB --cloud-provider gcp --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "invalid-db" for "--type" flag: must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds"
`)
}

func TestCreateProfessionalInstanceInvalidVersion(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --memory 1GB --cloud-provider gcp --tenant-id YOUR_TENANT_ID --version 6 --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "6" for "--version" flag: must be one of "4" or "5"
`)
}

func TestCreateFreeInstanceWithMemory(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --memory 1GB --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "1GB" for "--memory" flag: must not be set when "--type" flag is set to "free-db"
`)
}

func TestCreateFreeInstanceWithRegion(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "europe-west1" for "--region" flag: must not be set when "--type" flag is set to "free-db"
`)
}

func TestCreateFreeInstanceWithCloudProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --cloud-provider gcp --tenant-id YOUR_TENANT_ID --rw")

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: invalid argument "gcp" for "--cloud-provider" flag: must not be set when "--type" flag is set to "free-db"
`)
}

func TestCreateFreeInstanceWithGraphAnalyticsPlugin(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, "")

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --graph-analytics-plugin --tenant-id YOUR_TENANT_ID --rw")

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

			mockHandler := helper.NewRequestHandlerMock("/v1/instances", testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type professional-db --tenant-id YOUR_TENANT_ID --cloud-provider gcp --memory 4GB --rw")

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

	helper.ExecuteCommand("instance create --region europe-west1 --name Instance01 --type enterprise-db --tenant-id YOUR_TENANT_ID --cloud-provider gcp --memory 16GB --customer-managed-key-id UUID_OF_YOUR_KEY --rw")

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
		"region": "europe-west1",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "enterprise-db",
		"username": "neo4j"
	  }
	} `)
}

func TestCreateFreeInstanceWithConfigTenantId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.default-tenant", "YOUR_TENANT_ID")

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
		"region": "europe-west1",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "free-db",
		"username": "neo4j"
	  }
	}`)
}

func TestCreateFreeInstanceWithAwait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

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

	helper.ExecuteCommand("instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --await --rw")

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
		"region": "europe-west1",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "free-db",
		"username": "neo4j"
	}
}
Waiting for instance to be ready...
Instance Status: ready
	`)
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
			command:     "instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --credential-name myname --no-credential-storage --rw",
			expectedErr: `Error: "--credential-name" and "--no-credential-storage" cannot be used together`,
			wantHTTP:    0,
		},
		{
			name:        "explicit empty credential-name is rejected",
			command:     `instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --credential-name "" --rw`,
			expectedErr: `Error: invalid argument "" for "--credential-name" flag: name must not be empty`,
			wantHTTP:    0,
		},
		{
			name:        "valid flags proceed to HTTP call",
			command:     "instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --no-credential-storage --rw",
			expectedErr: "",
			wantHTTP:    1,
		},
		{
			name:        "credential-name without no-credential-storage proceeds to HTTP call",
			command:     "instance create --name Instance01 --type free-db --tenant-id YOUR_TENANT_ID --credential-name myname --rw",
			expectedErr: "",
			wantHTTP:    1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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

	fs, err := testfs.GetTestFs(`{"format":"json","aura":{"default-tenant":"YOUR_TENANT_ID"}}`, credentialsJSON)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	require.Nil(t, cfg.Credentials.Dbms, "expected Dbms to be nil with 'dbms: null' in credentials file")

	cmd := instance.NewCreateCmd(cfg)
	// Set required flags so PreRunE reaches the Dbms-nil check (not an earlier guard).
	require.NoError(t, cmd.Flags().Set("name", "Instance01"))
	require.NoError(t, cmd.Flags().Set("type", "free-db"))
	require.NoError(t, cmd.Flags().Set("tenant-id", "YOUR_TENANT_ID"))

	err = cmd.PreRunE(cmd, nil)
	require.EqualError(t, err, `credential storage is not available; use --no-credential-storage to skip storing credentials locally`)
}
