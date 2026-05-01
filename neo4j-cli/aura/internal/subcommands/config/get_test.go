// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.auth-url", "test")

	helper.ExecuteCommand("config get auth-url")

	helper.AssertOut("test")
}

func TestGetConfigDefault(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config get output")

	helper.AssertOut("default")
}

func TestGetConfigBetaEnabled(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("config get beta-enabled")

	helper.AssertErr("Error: invalid argument \"beta-enabled\" for \"aura-cli config get\"")
}
