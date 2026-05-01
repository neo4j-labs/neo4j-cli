// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"fmt"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig("{}")

	helper.ExecuteCommand("config list")

	helper.AssertOutJson(fmt.Sprintf(`{"auth-url": "%s","base-url": "%s","default-tenant": null}`, clicfg.DefaultAuraAuthUrl, clicfg.DefaultAuraBaseUrl))
}

func TestListConfigFiltersUnrecognisedKeys(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.OverwriteConfig(`{"aura": {"beta-enabled": true}}`)

	helper.ExecuteCommand("config list")

	helper.AssertOutJson(fmt.Sprintf(`{"auth-url": "%s","base-url": "%s","default-tenant": null}`, clicfg.DefaultAuraAuthUrl, clicfg.DefaultAuraBaseUrl))
}
