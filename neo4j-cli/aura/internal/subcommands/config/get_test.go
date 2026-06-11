// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"fmt"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.auth-url", "test")

	helper.ExecuteCommand("config get auth-url")

	helper.AssertOutJson(`{"auth-url": "test"}`)
}

func TestGetConfigWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.auth-url", "test")

	helper.ExecuteCommand(fmt.Sprintf("config get %s\"\n\"", "auth-url"))

	helper.AssertOutJson(`{"auth-url": "test"}`)
}

// Regression guard for REQ-F-007: passing `auth-url\n` must NOT
// produce an "invalid argument" error from cobra's OnlyValidArgs gate;
// the key is trimmed inside the Args func before validation runs.
func TestGetConfigTrailingNewlineDoesNotSurfaceInvalidArgument(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.auth-url", "test")

	helper.ExecuteCommand(fmt.Sprintf("config get %s\"\n\"", "auth-url"))

	helper.AssertErr("")
}

func TestGetConfigDefault(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config get format")

	// format is a global key; default value is "default"
	// "default" auto-detects: non-TTY test stdout → JSON rendering
	helper.AssertOutJson(`{"format": "default"}`)
}

func TestGetConfigBetaEnabled(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("config get beta-enabled")

	helper.AssertErr("Error: invalid argument \"beta-enabled\" for \"aura-cli config get\"")
}
