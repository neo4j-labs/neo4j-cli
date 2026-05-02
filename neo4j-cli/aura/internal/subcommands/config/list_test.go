// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"strings"
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

func TestListConfigOutputAppearsOnce(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("output", "table")

	helper.ExecuteCommand("config list")

	// "output" must appear exactly once as a global key — not duplicated as an aura-scoped key
	outStr := helper.PrintOut()
	assert.Equal(t, 1, strings.Count(outStr, "output"), "expected \"output\" to appear exactly once in list output")
	// aura keys must appear without the "aura." prefix
	assert.Contains(t, outStr, "default-tenant")
	assert.NotContains(t, outStr, "aura.default-tenant")
}
