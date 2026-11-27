// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

var instanceFields = []string{
	"name", "namespace", "edition", "version", "mode", "phase", "ready", "ready_servers", "servers",
}

type instanceResult struct {
	instance engine.Neo4j
}

func (r instanceResult) AsArray() []map[string]any {
	return []map[string]any{r.instance.TableRow()}
}

// MarshalJSON emits the full server object, so JSON and TOON output match
// what Kubernetes returned.
func (r instanceResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.instance.Record)
}

type instanceListResult []engine.Neo4j

func (r instanceListResult) AsArray() []map[string]any {
	rows := make([]map[string]any, 0, len(r))
	for _, instance := range r {
		rows = append(rows, instance.TableRow())
	}
	return rows
}

func (r instanceListResult) MarshalJSON() ([]byte, error) {
	records := make([]map[string]any, 0, len(r))
	for _, instance := range r {
		records = append(records, instance.Record)
	}
	return json.Marshal(records)
}

func printInstance(cmd *cobra.Command, cfg *clicfg.Config, instance engine.Neo4j) {
	commonoutput.PrintBodyMap(cmd, cfg, instanceResult{instance: instance}, instanceFields)
}

func printInstances(cmd *cobra.Command, cfg *clicfg.Config, instances []engine.Neo4j) {
	commonoutput.PrintBodyMap(cmd, cfg, instanceListResult(instances), instanceFields)
}

var credentialFields = []string{"name", "instance_name", "context", "namespace", "uri", "username", "bolt_tls"}

type credentialResult struct {
	credential engine.InstanceCredential
}

func (r credentialResult) AsArray() []map[string]any {
	c := r.credential
	return []map[string]any{{
		"name":          c.Name,
		"instance_name": c.InstanceName,
		"context":       c.Context,
		"namespace":     c.Namespace,
		"uri":           c.URI,
		"username":      c.Username,
		"bolt_tls":      c.BoltTLS,
	}}
}

func (r credentialResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray()[0])
}

func printCredential(cmd *cobra.Command, cfg *clicfg.Config, credential engine.InstanceCredential) {
	commonoutput.PrintBodyMap(cmd, cfg, credentialResult{credential: credential}, credentialFields)
}
