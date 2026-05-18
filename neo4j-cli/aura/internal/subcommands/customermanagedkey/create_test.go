// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateCustomerManagedKeys(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusAccepted, `{
		"data": {
		  "id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
		  "name": "Instance01",
		  "created": "2024-01-31T14:06:57Z",
		  "cloud_provider": "aws",
		  "key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
		  "region": "us-east-1",
		  "type": "enterprise-db",
		  "tenant_id": "YOUR_TENANT_ID",
		  "status": "pending"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf(`customer-managed-key create --region us-west-2 --name "Production Key" --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab --organization-id %s --project-id %s --rw`, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{
		"key_id": "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
		"name": "Production Key",
		"cloud_provider": "aws",
		"instance_type": "enterprise-db",
		"region": "us-west-2",
		"tenant_id": "YOUR_TENANT_ID"
	}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "aws",
		"created": "2024-01-31T14:06:57Z",
		"id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
		"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
		"name": "Instance01",
		"project_id": "YOUR_TENANT_ID",
		"region": "us-east-1",
		"status": "pending",
		"type": "enterprise-db"
	  }
	}`)
}

func TestCreateCustomerManagedKeysWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusAccepted, `{
		"data": {
		  "id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
		  "name": "Instance01",
		  "created": "2024-01-31T14:06:57Z",
		  "cloud_provider": "aws",
		  "key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
		  "region": "us-east-1",
		  "type": "enterprise-db",
		  "tenant_id": "YOUR_TENANT_ID",
		  "status": "pending"
		}
	  }`)

	helper.ExecuteCommand(`customer-managed-key create --region us-west-2 --name "Production Key" --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab --rw`)

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{
		"key_id": "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
		"name": "Production Key",
		"cloud_provider": "aws",
		"instance_type": "enterprise-db",
		"region": "us-west-2",
		"tenant_id": "YOUR_TENANT_ID"
	}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "aws",
		"created": "2024-01-31T14:06:57Z",
		"id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
		"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
		"name": "Instance01",
		"project_id": "YOUR_TENANT_ID",
		"region": "us-east-1",
		"status": "pending",
		"type": "enterprise-db"
	  }
	}`)
}

func TestCreateCustomerManagedKeysMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand(`customer-managed-key create --region us-west-2 --name "Production Key" --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab --rw`)

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestCreateCustomerManagedKeysMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand(fmt.Sprintf(`customer-managed-key create --region us-west-2 --name "Production Key" --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab --organization-id %s --rw`, testOrgID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestCreateCustomerManagedKeysProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand(fmt.Sprintf(`customer-managed-key create --region us-west-2 --name "Production Key" --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab --organization-id %s --project-id unknown-project --rw`, testOrgID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}
