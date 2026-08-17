// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
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
		return nil, clierr.NewUpstreamError("unexpected status code %d from GraphQL Data API CORS policy response: %s", statusCode, api.ScrubbedBodyTrunc(getResBody))
	}

	var parsedGetResBody DetailedBody
	if err := json.Unmarshal(getResBody, &parsedGetResBody); err != nil {
		return nil, clierr.NewUpstreamError("could not parse GraphQL Data API CORS policy response: %s", api.ScrubbedBodyTrunc(getResBody))
	}

	return parsedGetResBody.Data.Security.CorsPolicy.AllowedOrigins, nil
}
