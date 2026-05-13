// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuraCredentials_GetDefault_NoDefaultErrorBody locks the wording of the
// "default credential not set" error returned by AuraCredentials.GetDefault()
// when no default is configured. Asserts the substring invariants from the
// CLI-80 PRD (REQ-F-002 through REQ-F-005) so a future careless string edit
// fails locally rather than in user-facing copy.
func TestAuraCredentials_GetDefault_NoDefaultErrorBody(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)
	cfg := credentials.NewCredentials(fs, clicfg.ConfigPrefix)

	cred, err := cfg.Aura.GetDefault()

	require.Error(t, err)
	assert.Nil(t, cred)

	msg := err.Error()
	assert.Contains(t, msg, "https://console.neo4j.io/account", "REQ-F-002: primary Aura Console minting URL")
	assert.Contains(t, msg, "https://neo4j.com/docs/aura/api/authentication/", "REQ-F-003: working docs URL")
	assert.Contains(t, msg, "credential aura-client add", "REQ-F-004: canonical shipped subcommand path")
	assert.NotContains(t, msg, "https://neo4j.com/docs/aura/classic/platform/api/authentication", "REQ-F-005: legacy broken URL absent")
}
