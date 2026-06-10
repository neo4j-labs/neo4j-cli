// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
)

type DetailedBody struct {
	Data DataBody `json:"data"`
}

type DataBody struct {
	Security SecurityBody `json:"security"`
}

type SecurityBody struct {
	CorsPolicy CorsPolicyBody `json:"cors_policy"`
}

type CorsPolicyBody struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

func getExistingOrigins(cfg *clicfg.Config, dataApiId, instanceId string) ([]string, error) {
	getPath := fmt.Sprintf("/instances/%s/data-apis/graphql/%s", instanceId, dataApiId)
	getResBody, statusCode, err := api.MakeRequest(cfg, getPath, &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersionBeta1,
	})
	if err != nil {
		return nil, err
	}
	if statusCode != http.StatusOK {
		panic(clierr.NewFatalError("unexpected status code %d running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL))
	}

	var parsedGetResBody DetailedBody
	err = json.Unmarshal(getResBody, &parsedGetResBody)
	if err != nil {
		panic(err)
	}

	return parsedGetResBody.Data.Security.CorsPolicy.AllowedOrigins, nil
}
