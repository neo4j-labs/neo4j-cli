// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
)

// Neo4j is one Neo4j custom resource. Record is the complete server object
// used for JSON and TOON output. The typed fields support readiness checks and
// table rendering without remodelling the wire response.
type Neo4j struct {
	Record             map[string]any
	Name               string
	Namespace          string
	Edition            string
	Version            string
	Mode               string
	Phase              string
	Generation         int64
	ObservedGeneration int64
	Ready              bool
	ReadyServers       int32
	Servers            int32
	Message            string
}

// TableRow returns the summary columns used by instance get and list. It is
// separate from Record so structured output remains the full server object.
func (n Neo4j) TableRow() map[string]any {
	return map[string]any{
		"name":          n.Name,
		"namespace":     n.Namespace,
		"edition":       n.Edition,
		"version":       n.Version,
		"mode":          n.Mode,
		"phase":         n.Phase,
		"ready":         n.Ready,
		"ready_servers": n.ReadyServers,
		"servers":       n.Servers,
	}
}

type neo4jJSON struct {
	Metadata struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Generation int64  `json:"generation"`
	} `json:"metadata"`
	Spec struct {
		Edition  string `json:"edition"`
		Version  string `json:"version"`
		Topology struct {
			Mode string `json:"mode"`
		} `json:"topology"`
	} `json:"spec"`
	Status struct {
		Phase              string `json:"phase"`
		ObservedGeneration int64  `json:"observedGeneration"`
		ServerSummary      struct {
			Servers int32 `json:"servers"`
			Ready   int32 `json:"ready"`
		} `json:"serverSummary"`
		Conditions []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

// parseNeo4j decodes one Neo4j object twice: into a map for output, and
// into the typed fields the CLI reads.
func parseNeo4j(data []byte) (Neo4j, error) {
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return Neo4j{}, fmt.Errorf("parse Neo4j output: %w", err)
	}
	var raw neo4jJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return Neo4j{}, fmt.Errorf("parse Neo4j output: %w", err)
	}
	n := Neo4j{
		Record:             record,
		Name:               raw.Metadata.Name,
		Namespace:          raw.Metadata.Namespace,
		Edition:            raw.Spec.Edition,
		Version:            raw.Spec.Version,
		Mode:               raw.Spec.Topology.Mode,
		Phase:              raw.Status.Phase,
		Generation:         raw.Metadata.Generation,
		ObservedGeneration: raw.Status.ObservedGeneration,
		ReadyServers:       raw.Status.ServerSummary.Ready,
		Servers:            raw.Status.ServerSummary.Servers,
	}
	for _, condition := range raw.Status.Conditions {
		if condition.Type == "Ready" {
			n.Ready = condition.Status == "True" && raw.Status.ObservedGeneration >= raw.Metadata.Generation
			if n.Message == "" {
				n.Message = condition.Message
			}
		}
		if condition.Type == "Error" && condition.Status == "True" {
			n.Message = condition.Message
		}
	}
	return n, nil
}

// GetNeo4j implements Client.
func (c *execClient) GetNeo4j(ctx context.Context, name string) (Neo4j, error) {
	path, err := resolveKubectl()
	if err != nil {
		return Neo4j{}, err
	}
	args := kubectlGetArgs(c.settings, "neo4j", name)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return Neo4j{}, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseNeo4j([]byte(res.stdout))
}

// ListNeo4j implements Client. It returns only resources carrying the CLI's
// management label, so a pre-existing hand-written resource is not adopted by
// a list operation.
func (c *execClient) ListNeo4j(ctx context.Context) ([]Neo4j, error) {
	path, err := resolveKubectl()
	if err != nil {
		return nil, err
	}
	args := kubectlArgs(c.settings, "get", "neo4j", "-l", ManagedByLabel+"="+ManagedByValue, "-o", "json")
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return nil, classifyRunError(ctx, "kubectl", args, res)
	}
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &list); err != nil {
		return nil, fmt.Errorf("parse Neo4j list output: %w", err)
	}
	instances := make([]Neo4j, 0, len(list.Items))
	for _, item := range list.Items {
		instance, err := parseNeo4j(item)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// CreateNeo4j implements Client. kubectl create, rather than apply, keeps the
// command from overwriting an existing resource. The manifest travels on
// stdin, so no temporary file or secret-bearing argv is involved.
func (c *execClient) CreateNeo4j(ctx context.Context, manifest []byte) (Neo4j, error) {
	path, err := resolveKubectl()
	if err != nil {
		return Neo4j{}, err
	}
	args := kubectlArgs(c.settings, "create", "-f", "-", "-o", "json")
	res := c.execWithInput(ctx, "kubectl", path, args, manifest)
	if res.err != nil {
		return Neo4j{}, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseNeo4j([]byte(res.stdout))
}

// DeleteNeo4j implements Client. --wait=false leaves readiness polling to the
// command, which can then use the same context and timeout as create --wait.
func (c *execClient) DeleteNeo4j(ctx context.Context, name string) error {
	path, err := resolveKubectl()
	if err != nil {
		return err
	}
	args := kubectlArgs(c.settings, "delete", "neo4j", name, "--wait=false")
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return classifyRunError(ctx, "kubectl", args, res)
	}
	return nil
}
