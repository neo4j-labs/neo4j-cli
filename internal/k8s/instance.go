// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
)

// neo4jResource is the fully qualified kubectl resource for the operator's
// Neo4j kind. The bare short name `neo4j` is ambiguous on clusters that also
// carry the legacy Neo4j CRD families (neo4j.io/v1 and friends): kubectl then
// resolves `get neo4j <name>` to more than one resource and fails, while
// `get neo4j` without a name silently lists the wrong resource. The
// resource.group form is unambiguous on every cluster.
const neo4jResource = "neo4js.neo4j.com"

// Neo4j is one Neo4j custom resource. Record is the complete server object
// used for JSON and TOON output. The typed fields support readiness polling
// and table rendering without remodelling the wire response.
type Neo4j struct {
	Record             map[string]any
	Name               string
	Namespace          string
	Edition            string
	Version            string
	Mode               string
	BoltTLS            bool
	Phase              string
	Generation         int64
	ObservedGeneration int64
	Ready              bool
	ReadyServers       int32
	Servers            int32
	Message            string
}

// TableRow returns the summary columns for table output. Structured output
// uses Record so that it keeps the full server object.
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
		Trust *trustSpec `json:"trust"`
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

// parseNeo4j decodes one Neo4j object into a map for output and into the typed
// fields that the CLI reads.
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
		BoltTLS:            raw.Spec.Trust.hasBoltTLS(),
		Phase:              raw.Status.Phase,
		Generation:         raw.Metadata.Generation,
		ObservedGeneration: raw.Status.ObservedGeneration,
		ReadyServers:       raw.Status.ServerSummary.Ready,
		Servers:            raw.Status.ServerSummary.Servers,
	}
	for _, condition := range raw.Status.Conditions {
		if condition.Type == "Ready" {
			n.Ready = condition.Status == "True" && raw.Status.Phase != "Failed" && raw.Status.ObservedGeneration >= raw.Metadata.Generation
			if n.Message == "" {
				n.Message = condition.Message
			}
		}
		if condition.Type == "Error" && condition.Status == "True" && condition.Message != "" {
			n.Message = condition.Message
		}
	}
	return n, nil
}

// GetNeo4j implements Client.
func (c *execClient) GetNeo4j(ctx context.Context, name string) (Neo4j, error) {
	if err := ValidateResourceName("Neo4j name", name); err != nil {
		return Neo4j{}, err
	}
	path, err := resolveKubectl()
	if err != nil {
		return Neo4j{}, err
	}
	args := kubectlGetArgs(c.settings, neo4jResource, name)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return Neo4j{}, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseNeo4j([]byte(res.stdout))
}

// ListNeo4j implements Client. It selects the management label so that
// hand-written resources do not appear in the list.
func (c *execClient) ListNeo4j(ctx context.Context) ([]Neo4j, error) {
	path, err := resolveKubectl()
	if err != nil {
		return nil, err
	}
	args := kubectlArgs(c.settings, "get", neo4jResource, "-l", ManagedByLabel+"="+ManagedByValue, "-o", "json")
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

// CreateNeo4j implements Client. It runs kubectl create so that it never
// overwrites an existing resource, and pipes the manifest on stdin.
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

// DeleteNeo4j implements Client. It passes --wait=false so that the caller
// controls deletion polling and its timeout.
func (c *execClient) DeleteNeo4j(ctx context.Context, name string) error {
	if err := ValidateResourceName("Neo4j name", name); err != nil {
		return err
	}
	path, err := resolveKubectl()
	if err != nil {
		return err
	}
	args := kubectlArgs(c.settings, "delete", neo4jResource, name, "--wait=false")
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return classifyRunError(ctx, "kubectl", args, res)
	}
	return nil
}
