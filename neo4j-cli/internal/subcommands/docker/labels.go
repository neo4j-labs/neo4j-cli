// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import "context"

// Label keys applied to every container created by `neo4j-cli docker create`.
// Docker is the source of truth for managed state (REQ-F-001, REQ-F-020):
// containers carrying LabelManaged=true are discoverable via
// `docker ps --filter label=...`, and the remaining keys carry per-container
// metadata that `list` / `get` re-render without a separate state file.
const (
	LabelManaged   = "org.neo4j.cli.managed"
	LabelEdition   = "org.neo4j.cli.edition"
	LabelVersion   = "org.neo4j.cli.version"
	LabelBoltPort  = "org.neo4j.cli.bolt-port"
	LabelHTTPPort  = "org.neo4j.cli.http-port"
	LabelEphemeral = "org.neo4j.cli.ephemeral"
)

// Container is the rendered metadata view of a managed neo4j-cli container.
// Fields mirror the columns documented in REQ-F-021 (`list`) and REQ-F-031
// (`get`); `URI` and `Image` are populated by `get` but left empty by `list`.
//
// Managed mirrors the `org.neo4j.cli.managed=true` label and lets the `get`
// leaf enforce REQ-F-032: a container that exists in Docker but lacks the
// label is treated as unknown. The field is NOT serialised into the rendered
// output — it is a per-call control bit that `get` consumes before rendering.
type Container struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Edition   string `json:"edition"`
	Version   string `json:"version"`
	BoltPort  string `json:"bolt-port"`
	HTTPPort  string `json:"http-port"`
	Ephemeral bool   `json:"ephemeral"`
	URI       string `json:"uri,omitempty"`
	Image     string `json:"image,omitempty"`
	Managed   bool   `json:"-"`
}

// Inspect is a thin convenience over dockerClient.Inspect so leaves (`get`,
// `start`, `stop`, `delete`) can fetch the metadata view in one call. The
// concrete parsing lives in client.go's parseInspectOutput.
func Inspect(ctx context.Context, c dockerClient, name string) (Container, error) {
	return c.Inspect(ctx, name)
}
