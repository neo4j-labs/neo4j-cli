// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuraCredentials_GetDefault_NoDefault_AuthError locks the reclassification
// from task-004: AuraCredentials.GetDefault() is exclusively called from the
// Aura HTTP request path; "default credential not set" therefore means
// "auth missing" (exit 4), not a usage error.
func TestAuraCredentials_GetDefault_NoDefault_AuthError(t *testing.T) {
	c := &credentials.AuraCredentials{}

	cred, err := c.GetDefault()
	require.Error(t, err)
	assert.Nil(t, cred)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code)
	assert.Contains(t, ce.Error(), "default credential not set")
}
