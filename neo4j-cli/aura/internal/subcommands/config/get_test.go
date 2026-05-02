// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
)

func TestGetConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.auth-url", "test")

	helper.ExecuteCommand("config get auth-url")

	helper.AssertOutJson(`{"auth-url": "test"}`)
}

func TestGetConfigDefault(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config get output")

	// output is a global key exposed by the standalone config command; default value is "default"
	// with no output setting in config, cfg.Global.Output() returns "default" which now renders as a table
	outStr := helper.PrintOut()
	assert.Contains(t, outStr, "KEY")
	assert.Contains(t, outStr, "VALUE")
	assert.Contains(t, outStr, "output")
	assert.Contains(t, outStr, "default")
}

func TestGetConfigBetaEnabled(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("config get beta-enabled")

	helper.AssertErr("Error: invalid argument \"beta-enabled\" for \"aura-cli config get\"")
}
