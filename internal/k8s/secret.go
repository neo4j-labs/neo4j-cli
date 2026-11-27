// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/neo4j/cli/internal/clierr"
)

// GetSecret implements Client. Kubernetes returns Secret values as base64.
// Decode them here, and never include the data in a debug trace.
func (c *execClient) GetSecret(ctx context.Context, name string) (map[string][]byte, error) {
	path, err := resolveKubectl()
	if err != nil {
		return nil, err
	}
	args := kubectlGetArgs(c.settings, "secret", name)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return nil, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseSecret(res.stdout)
}

func parseSecret(stdout string) (map[string][]byte, error) {
	var raw struct {
		Kind string            `json:"kind"`
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return nil, clierr.NewFatalError("parse Secret output: %s", err)
	}
	if raw.Kind != "Secret" {
		return nil, clierr.NewFatalError("expected a Secret from Kubernetes, got %q", raw.Kind)
	}
	data := make(map[string][]byte, len(raw.Data))
	for key, value := range raw.Data {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, clierr.NewFatalError("Secret key %q contains invalid base64 data", key)
		}
		data[key] = decoded
	}
	return data, nil
}
