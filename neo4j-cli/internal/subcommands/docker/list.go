// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"encoding/json"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// newListCmd builds the `neo4j-cli docker list` leaf. It enumerates every
// container carrying the `org.neo4j.cli.managed=true` label (REQ-F-020) and
// renders the seven documented metadata columns per REQ-F-021. The filter is
// passed through to `docker ps --filter` for efficiency on real installs AND
// also re-applied in Go so unit tests (which drive an in-memory fake that
// does not honour docker-side filters) verify the contract end-to-end.
//
// Empty result renders as an empty table / empty JSON array with exit 0
// (REQ-F-022) — no error is returned.
func newListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Neo4j containers managed by neo4j-cli",
		Long: "List all Neo4j Docker containers carrying the `org.neo4j.cli.managed=true` label. " +
			"Renders one row per container with name, status (Docker's human-readable state), edition, version, " +
			"bolt-port, http-port, and ephemeral. Unmanaged containers (no label) are excluded. " +
			"An empty result renders as an empty table or empty JSON array (exit 0).",
		Example: `# List managed Neo4j containers as a table
neo4j-cli docker list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli docker list --format json

# Emit TOON for token-efficient ingestion by agents
neo4j-cli docker list --format toon`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := clientFactory()
			ctx := cmd.Context()

			entries, err := client.PsAll(ctx, []string{"label=" + LabelManaged + "=true"})
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			rows := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				labels := parseLabels(entry.Labels)
				// Defensive in-Go filter — execClient passes the same filter
				// through to docker, but unit tests against fakeDockerClient
				// do not honour docker-side filters, so the contract needs to
				// hold here too.
				if labels[LabelManaged] != "true" {
					continue
				}
				rows = append(rows, map[string]any{
					"name":      firstName(entry.Names),
					"status":    entry.Status,
					"edition":   labels[LabelEdition],
					"version":   labels[LabelVersion],
					"bolt-port": labels[LabelBoltPort],
					"http-port": labels[LabelHTTPPort],
					"ephemeral": labels[LabelEphemeral] == "true",
				})
			}

			fields := []string{"name", "status", "edition", "version", "bolt-port", "http-port", "ephemeral"}
			commonoutput.PrintBodyMap(cmd, cfg, listRows(rows), fields)
			return nil
		},
	}

	// --format is registered as a persistent flag on the neo4j-cli root
	// (see neo4j-cli/app/app.go) and inherited by every subcommand via
	// cobra's persistent-flag propagation, so no per-leaf registration is
	// needed here.
	return cmd
}

// listRows adapts a []map[string]any into a commonoutput.ResponseData so
// PrintBodyMap can render an array of rows. Distinct from create.go's
// singleRow which always emits a one-element array; listRows preserves
// cardinality (including empty → `[]`).
type listRows []map[string]any

// AsArray implements commonoutput.ResponseData.
func (r listRows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

// MarshalJSON returns the JSON array form so the empty case renders as `[]`
// rather than `null` (the default for a nil []map slice).
func (r listRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}

// parseLabels turns Docker's `key1=val1,key2=val2` Labels string (as emitted
// by `docker ps --format '{{json .}}'`) into a lookup map. Empty input yields
// an empty map. Entries without `=` are skipped silently — the metadata we
// care about is always written with a value by `create`.
func parseLabels(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	for _, kv := range strings.Split(s, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		out[kv[:eq]] = kv[eq+1:]
	}
	return out
}

// firstName extracts the first non-empty name from Docker's comma-separated
// Names field, stripping any leading `/` that the daemon may prepend. The
// containers `create` produces always have a single name, but defending
// against multi-name entries keeps the rendering predictable.
func firstName(s string) string {
	for _, n := range strings.Split(s, ",") {
		n = strings.TrimSpace(n)
		n = strings.TrimPrefix(n, "/")
		if n != "" {
			return n
		}
	}
	return ""
}
