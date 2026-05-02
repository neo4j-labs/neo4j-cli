// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
)

func TestListConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config list")

	// standalone config list with default output renders as a table via PrintBodyMap
	// includes both global keys (output) and aura-scoped keys
	outStr := helper.PrintOut()
	assert.Contains(t, outStr, "KEY")
	assert.Contains(t, outStr, "VALUE")
	assert.Contains(t, outStr, "output")
	assert.Contains(t, outStr, "auth-url")
	assert.Contains(t, outStr, clicfg.DefaultAuraAuthUrl)
	assert.Contains(t, outStr, "base-url")
	assert.Contains(t, outStr, clicfg.DefaultAuraBaseUrl)
}

func TestListConfigFiltersUnrecognisedKeys(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig(`{"aura": {"beta-enabled": true}}`)

	helper.ExecuteCommand("config list")

	// standalone config list with default output renders as a table; unrecognised keys are filtered out
	outStr := helper.PrintOut()
	assert.Contains(t, outStr, "KEY")
	assert.Contains(t, outStr, "VALUE")
	assert.Contains(t, outStr, "output")
	assert.Contains(t, outStr, "auth-url")
	assert.Contains(t, outStr, "base-url")
	assert.NotContains(t, outStr, "beta-enabled")
}
