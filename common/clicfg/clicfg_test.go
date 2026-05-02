// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
)

func TestGetAuraBaseUrlConfigRemovesTrailingPath(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	cfgStr := fmt.Sprintf(`{
		"output": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s/v1"
			}
		}`, server.URL, server.URL)

	credentialsStr := `{
		"aura": {
			"credentials": [{
				"name": "test-cred",
				"access-token": "dsa",
				"token-expiry": 123
			}],
			"default-credential": "test-cred"
			}
		}`

	fs, err := testfs.GetTestFs(cfgStr, credentialsStr)
	assert.Nil(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	//The path parameter will be removed from GET base url
	assert.Equal(t, server.URL, cfg.Aura.BaseUrl())
}
