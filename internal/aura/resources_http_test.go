// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestCustomerManagedKeysGet_ChecksProjectOwnership(t *testing.T) {
	const id = "cmk-xyz"
	path := "/v1/customer-managed-keys/" + id

	t.Run("a key of another project is a structured not-found", func(t *testing.T) {
		c := newTestClient(t, path, 200, `{"data":{"id":"cmk-xyz","tenant_id":"other-project"}}`)

		_, err := c.CustomerManagedKeys().Get(context.Background(), Scope{ProjectID: "my-project"}, id)

		require.Error(t, err)
		assert.Equal(t, fmt.Sprintf("could not find customer-managed-key %s in project my-project", id), err.Error())
		var ce *clierr.CLIError
		require.True(t, errors.As(err, &ce))
		assert.Equal(t, 3, ce.Code)
		assert.Equal(t, "customer-managed-key", ce.ResourceType)
		assert.Equal(t, id, ce.ResourceID)
		assert.Equal(t, "Run 'neo4j-cli aura customer-managed-key list --project-id <id>' to see keys in this project.", ce.Suggestion)
	})

	t.Run("an owned key is returned with tenant_id exposed as project_id", func(t *testing.T) {
		c := newTestClient(t, path, 200, `{"data":{"id":"cmk-xyz","name":"k","status":"ready","tenant_id":"my-project"}}`)

		k, err := c.CustomerManagedKeys().Get(context.Background(), Scope{ProjectID: "my-project"}, id)

		require.NoError(t, err)
		assert.Equal(t, "my-project", k.ProjectID)
		assert.Equal(t, "my-project", k.Record["project_id"])
		assert.NotContains(t, k.Record, "tenant_id")
	})

	t.Run("a hostile id is rejected before any request", func(t *testing.T) {
		_, err := cmkService{}.Get(context.Background(), Scope{ProjectID: "p"}, "../x")
		var ce *clierr.CLIError
		assert.True(t, errors.As(err, &ce))
	})
}

func TestSnapshotsGet_NotFoundNamesTheSnapshot(t *testing.T) {
	c := newTestClient(t, "/v1/instances/inst-1/snapshots/snap-9", 404, `{"errors":[{"message":"not found"}]}`)

	_, err := c.Snapshots().Get(context.Background(), "inst-1", "snap-9")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Equal(t, "snapshot", ce.ResourceType)
	assert.Equal(t, "snap-9", ce.ResourceID)
	assert.Contains(t, ce.Suggestion, "instance snapshot list --instance-id")
}

func TestSnapshotsValidateIDsBeforeAnyRequest(t *testing.T) {
	s := snapshotService{}
	_, err := s.Get(context.Background(), "inst-1", "../../x")
	assert.Error(t, err)
	_, err = s.List(context.Background(), "a/b", "")
	assert.Error(t, err)
	_, err = s.Create(context.Background(), "")
	assert.Error(t, err)
}
