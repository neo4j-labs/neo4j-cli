// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
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
