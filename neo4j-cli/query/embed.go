// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/query/embed"
)

// embedVector is the rendered output of the `:embed` leaf — a single vector
// of floats. It implements commonoutput.ResponseData so PrintBodyMap can
// render it in any of the supported formats:
//
//   - json/toon: a raw JSON array of floats (via MarshalJSON; printToon
//     round-trips through MarshalJSON before encoding to TOON).
//   - table: a single-row, single-cell table whose cell value is the
//     compact JSON representation of the vector — printTable looks up the
//     "embedding" column on the AsArray result and getNestedField stringifies
//     a non-string value via fmt.Sprintf, so we pre-format the cell as a
//     stringified JSON array instead. The header text is the field passed to
//     PrintBodyMap (we use "embedding").
type embedVector []float32

// AsArray satisfies commonoutput.ResponseData. The returned slice is a single
// row whose only field is "embedding" mapped to the compact JSON
// representation of the vector. The string form keeps `printTable`'s
// getNestedField helper happy without the JSON-MarshalIndent path that triggers
// for raw slice values.
func (v embedVector) AsArray() []map[string]any {
	bytes, err := json.Marshal([]float32(v))
	if err != nil {
		// json.Marshal of a []float32 cannot fail; fall back to a printable
		// form rather than panicking the render path.
		return []map[string]any{{"embedding": fmt.Sprintf("%v", []float32(v))}}
	}
	return []map[string]any{{"embedding": string(bytes)}}
}

// MarshalJSON returns the vector as a raw JSON array of floats. Used by both
// --format json (directly) and --format toon (via the json→any→toon round-trip
// in commonoutput.printToon).
func (v embedVector) MarshalJSON() ([]byte, error) {
	if v == nil {
		return json.Marshal([]float32{})
	}
	return json.Marshal([]float32(v))
}

// newEmbedCmd builds the `:embed` cobra leaf. The `Use` is the literal
// `:embed` (matches the cypher-shell-style colon-prefix `:schema` sibling).
// The leaf reads text from a positional arg or piped stdin, runs it through
// the resolved embedding provider, and renders the resulting vector via
// PrintBodyMap. It does NOT open a Bolt driver and does NOT prompt for a
// password — embedding is provider-side only.
func newEmbedCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   ":embed [text]",
		Short: "Compute an embedding vector for the given text",
		Long: "Compute an embedding vector for the supplied text using the " +
			"configured embed provider. Text is taken from the positional " +
			"argument, or from stdin when no argument is provided and stdin " +
			"is piped. The embed provider configuration follows the same " +
			"--embed-* flags as the parent `query` command. No Bolt connection " +
			"is opened.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEmbed(cmd, args, cfg)
		},
	}
}

// runEmbed is the `:embed` RunE body. It resolves the input text, builds a
// Provider from the resolved embed config, runs Embed, and renders the
// resulting vector via PrintBodyMap. Errors from the provider get the
// `query: embed:` prefix to match the rest of the package's error wording.
func runEmbed(cmd *cobra.Command, args []string, cfg *clicfg.Config) error {
	cmd.SilenceUsage = true

	text, err := readPositionalOrStdin(cmd, args, "text")
	if err != nil {
		return err
	}

	ec, err := embed.Resolve(cmd, cfg)
	if err != nil {
		return err
	}
	provider, err := embed.Factory()(ec)
	if err != nil {
		return err
	}

	vec, err := provider.Embed(cmd.Context(), text)
	if err != nil {
		return fmt.Errorf("query: embed: %w", err)
	}

	commonoutput.PrintBodyMap(cmd, cfg, embedVector(vec), []string{"embedding"})
	return nil
}
