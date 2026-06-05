// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

// This file holds OUTPUT projections only. The wire structs in types.go keep
// Desktop's camelCase tags because they both parse Desktop's HTTP responses and
// serialize request bodies. These projections re-tag the CLI-rendered output as
// snake_case (connection_uri, pending_restart, file_path, …) without touching
// the wire contract. Do NOT re-couple them by rendering the wire structs.

// DbmsInfoOutput is the snake_case CLI-rendered projection of DbmsInfo.
type DbmsInfoOutput struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Project       string         `json:"project,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ConnectionURI string         `json:"connection_uri,omitempty"`
	RootPath      string         `json:"root_path,omitempty"`
	Status        string         `json:"status,omitempty"`
	ServerStatus  string         `json:"server_status,omitempty"`
	Version       string         `json:"version,omitempty"`
	Edition       string         `json:"edition,omitempty"`
	Prerelease    string         `json:"prerelease,omitempty"`
}

// ToOutput projects a DbmsInfo onto its snake_case render shape. The structs
// share field shape and differ only in json tags, so a conversion suffices.
func (d DbmsInfo) ToOutput() DbmsInfoOutput { return DbmsInfoOutput(d) }

// DbmsInfoOutputs projects a slice, returning a non-nil empty slice for nil
// input so JSON renders `[]` rather than `null`.
func DbmsInfoOutputs(items []DbmsInfo) []DbmsInfoOutput {
	out := make([]DbmsInfoOutput, 0, len(items))
	for _, it := range items {
		out = append(out, it.ToOutput())
	}
	return out
}

// ConnectionOutput is the snake_case CLI-rendered projection of Connection.
type ConnectionOutput struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Project       string         `json:"project,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ConnectionURI string         `json:"connection_uri,omitempty"`
	CreatedAt     string         `json:"created_at,omitempty"`
	ManifestPath  string         `json:"manifest_path,omitempty"`
}

// ToOutput projects a Connection onto its snake_case render shape.
func (c Connection) ToOutput() ConnectionOutput { return ConnectionOutput(c) }

// ConnectionOutputs projects a slice, returning a non-nil empty slice for nil
// input so JSON renders `[]` rather than `null`.
func ConnectionOutputs(items []Connection) []ConnectionOutput {
	out := make([]ConnectionOutput, 0, len(items))
	for _, it := range items {
		out = append(out, it.ToOutput())
	}
	return out
}

// DbmsPluginOutput is the snake_case CLI-rendered projection of DbmsPlugin.
type DbmsPluginOutput struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	FilePath       string `json:"file_path"`
	PendingRestart bool   `json:"pending_restart"`
}

// ToOutput projects a DbmsPlugin onto its snake_case render shape.
func (p DbmsPlugin) ToOutput() DbmsPluginOutput { return DbmsPluginOutput(p) }

// DbmsPluginOutputs projects a slice, returning a non-nil empty slice for nil
// input so JSON renders `[]` rather than `null`.
func DbmsPluginOutputs(items []DbmsPlugin) []DbmsPluginOutput {
	out := make([]DbmsPluginOutput, 0, len(items))
	for _, it := range items {
		out = append(out, it.ToOutput())
	}
	return out
}
