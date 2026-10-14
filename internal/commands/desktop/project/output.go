// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"github.com/neo4j/cli/internal/desktopclient"
)

// projectOutput is the snake_case CLI-rendered projection of a Project.
// Desktop's CreatedAt is a unix-milliseconds timestamp (omitted for legacy
// entries); the CLI renders it as an RFC3339 UTC string, "" when Desktop
// does not report one. This single projection is shared by list / create /
// update so `created_at` renders identically on every project output.
type projectOutput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
}

func toProjectOutput(p desktopclient.Project) projectOutput {
	return projectOutput{ID: p.ID, Name: p.Name, CreatedAt: desktopclient.FormatUnixMsRFC3339(p.CreatedAt)}
}
