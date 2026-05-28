// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

// validProviders is the closed set of embed providers the CLI supports.
// Kept here (not exported) because add.go is the only validation site for
// provider names at storage time; the embed runtime in `neo4j-cli/query/embed`
// does its own switch over provider strings independently.
var validProviders = []string{"openai", "ollama", "huggingface"}

func newAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name       string
		provider   string
		model      string
		apiKey     string
		baseURL    string
		dimensions int
	)

	const (
		nameFlag       = "name"
		providerFlag   = "provider"
		modelFlag      = "model"
		apiKeyFlag     = "api-key"
		baseURLFlag    = "base-url"
		dimensionsFlag = "dimensions"
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Adds an embed credential",
		Long: "Add an embedding-provider credential. Provider must be one of openai, ollama, huggingface. " +
			"`--api-key` is optional for ollama (no auth required) and may be omitted for openai/huggingface if you intend to provide it via env var (`OPENAI_API_KEY` / `HF_TOKEN` / `NEO4J_EMBED_API_KEY`). " +
			"The first credential added becomes the default; switch later with `credential embed use <name>`.",
		Example: `# Add an OpenAI embed credential (becomes the default if it is the first one)
neo4j-cli credential embed add --name openai-small --provider openai --model text-embedding-3-small --api-key sk-... --rw

# Add a local Ollama embed credential (no api-key required)
neo4j-cli credential embed add --name ollama-nomic --provider ollama --model nomic-embed-text --base-url http://localhost:11434 --rw

# Add a HuggingFace embed credential with explicit dimensions
neo4j-cli credential embed add --name hf-bge --provider huggingface --model BAAI/bge-small-en-v1.5 --api-key hf_... --dimensions 384 --rw`,
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isValidProvider(provider) {
				return clierr.NewUsageError("invalid --provider %q: must be one of %v", provider, validProviders)
			}
			return cfg.Credentials.Embed.Add(name, provider, model, baseURL, apiKey, dimensions, "", "")
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&provider, providerFlag, "", "(required) Provider (one of: openai, ollama, huggingface)")
	cmd.MarkFlagRequired(providerFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&model, modelFlag, "", "(required) Model")
	cmd.MarkFlagRequired(modelFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&apiKey, apiKeyFlag, "", "API key for the provider")
	cmd.Flags().StringVar(&baseURL, baseURLFlag, "", "Base URL for the provider (overrides provider default)")
	cmd.Flags().IntVar(&dimensions, dimensionsFlag, 0, "Embedding dimensions (provider-specific; 0 means provider default)")

	return cmd
}

func isValidProvider(p string) bool {
	for _, v := range validProviders {
		if v == p {
			return true
		}
	}
	return false
}
