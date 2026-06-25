// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"strings"

	"github.com/neo4j/cli/common/clierr"
)

// Env var names for credential injection. Single-sourced here so a new
// credential type's vars are declared alongside its spec.
const (
	EnvAuraClientID     = "NEO4J_AURA_CLIENT_ID"
	EnvAuraClientSecret = "NEO4J_AURA_CLIENT_SECRET"

	EnvURI      = "NEO4J_URI"
	EnvUsername = "NEO4J_USERNAME"
	EnvPassword = "NEO4J_PASSWORD"
	EnvDatabase = "NEO4J_DATABASE"

	EnvEmbedProvider   = "NEO4J_EMBED_PROVIDER"
	EnvEmbedModel      = "NEO4J_EMBED_MODEL"
	EnvEmbedBaseURL    = "NEO4J_EMBED_BASE_URL"
	EnvEmbedDimensions = "NEO4J_EMBED_DIMENSIONS"
	EnvEmbedAPIKey     = "NEO4J_EMBED_API_KEY"
	EnvOpenAIKey       = "OPENAI_API_KEY"
	EnvHFToken         = "HF_TOKEN"
	EnvGeminiKey       = "GEMINI_API_KEY"
	EnvGoogleKey       = "GOOGLE_API_KEY"
)

// EnvCredentialSpec describes the env vars for one credential type.
type EnvCredentialSpec struct {
	// Sentinel is the single env var whose presence triggers the hint.
	// Convention: the primary identifying var for the type.
	Sentinel string
	// RequiredGroups: each inner slice is a set of vars that must ALL be
	// present if ANY one of them is set. Multiple groups are validated
	// independently.
	RequiredGroups [][]string
	// OptionalVars are vars that may be set independently (no completeness
	// check).
	OptionalVars []string
}

var AuraEnvSpec = EnvCredentialSpec{
	Sentinel:       EnvAuraClientID,
	RequiredGroups: [][]string{{EnvAuraClientID, EnvAuraClientSecret}},
}

var DBMSEnvSpec = EnvCredentialSpec{
	Sentinel:       EnvURI,
	RequiredGroups: [][]string{{EnvURI, EnvUsername, EnvPassword}},
	OptionalVars:   []string{EnvDatabase},
}

var EmbedEnvSpec = EnvCredentialSpec{
	Sentinel:       EnvEmbedProvider,
	RequiredGroups: [][]string{},
	OptionalVars: []string{
		EnvEmbedModel, EnvEmbedBaseURL, EnvEmbedDimensions, EnvEmbedAPIKey,
		EnvOpenAIKey, EnvHFToken, EnvGeminiKey, EnvGoogleKey,
	},
}

// allEnvSpecs is the registry consulted by HasAnyCredentialEnvVar. A new
// credential type must be added here for its sentinel to be reachable.
var allEnvSpecs = []EnvCredentialSpec{AuraEnvSpec, DBMSEnvSpec, EmbedEnvSpec}

// HasAnyCredentialEnvVar returns true if any spec's sentinel env var is set.
func HasAnyCredentialEnvVar(getenv func(string) string) bool {
	for _, spec := range allEnvSpecs {
		if getenv(spec.Sentinel) != "" {
			return true
		}
	}
	return false
}

// ValidateEnvCredentialSet checks each RequiredGroup: if any var in a group
// is set, all of them must be. Returns a clierr usage error naming the
// missing vars, or nil when every group is either fully set or fully empty.
func ValidateEnvCredentialSet(spec EnvCredentialSpec, getenv func(string) string) error {
	for _, group := range spec.RequiredGroups {
		var present, missing []string
		for _, name := range group {
			if getenv(name) != "" {
				present = append(present, name)
			} else {
				missing = append(missing, name)
			}
		}
		if len(present) > 0 && len(missing) > 0 {
			return clierr.NewUsageError(
				"incomplete credential environment: %s must be set when %s %s provided (missing: %s)",
				strings.Join(group, ", "),
				strings.Join(present, ", "),
				plural(len(present)),
				strings.Join(missing, ", "),
			)
		}
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
