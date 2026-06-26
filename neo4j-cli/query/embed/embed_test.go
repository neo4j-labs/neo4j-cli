// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/dotenv"
	"github.com/neo4j/cli/test/utils/testfs"
)

// envAcceptEnvVars is the env var bound to the accept-env-vars config key.
// Tests that exercise the env-var path set it to "1".
const envAcceptEnvVars = "NEO4J_CLI_ACCEPT_ENV_VARS"

// Embed env-var name aliases for the tests. The literals are single-sourced in
// the credentials package; these short local names keep the table tests terse.
const (
	envEmbedProvider   = credentials.EnvEmbedProvider
	envEmbedModel      = credentials.EnvEmbedModel
	envEmbedBaseURL    = credentials.EnvEmbedBaseURL
	envEmbedDimensions = credentials.EnvEmbedDimensions
	envEmbedAPIKey     = credentials.EnvEmbedAPIKey
	envOpenAIKey       = credentials.EnvOpenAIKey
	envHFToken         = credentials.EnvHFToken
	envGeminiKey       = credentials.EnvGeminiKey
	envGoogleKey       = credentials.EnvGoogleKey
)

// newTestCmd builds a cobra.Command carrying the same persistent flag set the
// real `query` parent registers (so cmd.Flag(...).Changed semantics line up
// with production) plus a parsed flag set so flag values are observable.
// Persistent-flag access requires either Execute() or ParseFlags(); we go
// through ParseFlags so tests can drive flag arguments directly.
func newTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "query"}
	cmd.PersistentFlags().StringP("credential", "c", "", "")
	cmd.PersistentFlags().String("embed-credential", "", "")
	cmd.PersistentFlags().String("embed-provider", "", "")
	cmd.PersistentFlags().String("embed-model", "", "")
	cmd.PersistentFlags().String("embed-base-url", "", "")
	cmd.PersistentFlags().Int("embed-dimensions", 0, "")
	require.NoError(t, cmd.ParseFlags(args))
	return cmd
}

// newTestCfg returns an in-memory config with the supplied credentials.json
// body. Always uses testfs.GetTestFs (per AGENTS.md) so the test never touches
// real credentials at ~/Library/Preferences/neo4j/cli/credentials.json.
func newTestCfg(t *testing.T, credsJSON string) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, credsJSON)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", clicfg.QueryScope)
}

// clearEmbedEnv clears every env var Resolve consults, so tests start from a
// known-empty baseline regardless of the developer machine's shell.
func clearEmbedEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(envEmbedProvider, "")
	t.Setenv(envEmbedModel, "")
	t.Setenv(envEmbedBaseURL, "")
	t.Setenv(envEmbedDimensions, "")
	t.Setenv(envEmbedAPIKey, "")
	t.Setenv(envOpenAIKey, "")
	t.Setenv(envHFToken, "")
	t.Setenv(envGeminiKey, "")
	t.Setenv(envGoogleKey, "")
}

// withDotenvCwd writes a .env file into the supplied filesystem at <tmp>/.env
// and changes the test's working directory to tmp so loadDotenv finds it via
// findDotenv's walk-up.
func withDotenvCwd(t *testing.T, fs afero.Fs, body string) {
	t.Helper()
	tmp := t.TempDir()
	t.Chdir(tmp)
	require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"), []byte(body), 0644))
}

func TestResolve_StoredCredOnly(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{"embed":{"default-credential":"d","credentials":[` +
		`{"name":"d","provider":"openai","model":"text-embedding-3-small",` +
		`"base-url":"https://stored.example/v1","dimensions":256,"api-key":"stored-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "openai", got.Provider)
	assert.Equal(t, "text-embedding-3-small", got.Model)
	assert.Equal(t, "https://stored.example/v1", got.BaseURL)
	assert.Equal(t, 256, got.Dimensions)
	assert.Equal(t, "stored-key", got.APIKey)
	assert.Equal(t, "neo4j-cli/vtest", got.UserAgent)
}

func TestResolve_FlagsBeatEnvBeatsDotenvBeatsStored(t *testing.T) {
	clearEmbedEnv(t)
	t.Setenv(envAcceptEnvVars, "1")

	creds := `{"embed":{"default-credential":"d","credentials":[` +
		`{"name":"d","provider":"openai","model":"stored-model",` +
		`"base-url":"https://stored.example/v1","dimensions":100,"api-key":"stored-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	withDotenvCwd(t, cfg.Aura.Fs(), "NEO4J_EMBED_MODEL=dotenv-model\nNEO4J_EMBED_BASE_URL=https://dotenv.example/v1\nNEO4J_EMBED_DIMENSIONS=200\nNEO4J_EMBED_API_KEY=dotenv-key\n")

	t.Setenv(envEmbedModel, "env-model")
	t.Setenv(envEmbedBaseURL, "https://env.example/v1")
	// Leave dimensions unset in OS env so dotenv wins for that field.

	cmd := newTestCmd(t,
		"--embed-model=flag-model",
	)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)

	// model: flag wins
	assert.Equal(t, "flag-model", got.Model)
	// base-url: no flag, env wins over dotenv
	assert.Equal(t, "https://env.example/v1", got.BaseURL)
	// dimensions: no flag, no env, dotenv wins over stored
	assert.Equal(t, 200, got.Dimensions)
	// provider: nothing in env/dotenv/flag, stored wins
	assert.Equal(t, "openai", got.Provider)
	// API key: dotenv NEO4J_EMBED_API_KEY beats stored APIKey (no OS env)
	assert.Equal(t, "dotenv-key", got.APIKey)
}

func TestResolve_ExplicitEmbedCredential_MissingErrors(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	cfg := newTestCfg(t, "{}")
	cmd := newTestCmd(t, "--embed-credential=nope")

	_, err := Resolve(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
	assert.Contains(t, err.Error(), "credential embed list")
}

func TestResolve_ExplicitEmbedCredential_OverridesLinkedAndDefault(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{` +
		`"dbms":{"default-credential":"db","credentials":[` +
		`{"name":"db","username":"u","password":"p","database-name":"neo4j","uri":"neo4j://x:7687","embed-credential":"linked"}` +
		`]},` +
		`"embed":{"default-credential":"defaulted","credentials":[` +
		`{"name":"defaulted","provider":"openai","model":"def","base-url":"","dimensions":0,"api-key":"def-key"},` +
		`{"name":"linked","provider":"openai","model":"linked-model","base-url":"","dimensions":0,"api-key":"linked-key"},` +
		`{"name":"explicit","provider":"openai","model":"explicit-model","base-url":"","dimensions":0,"api-key":"explicit-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t, "--embed-credential=explicit")

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "explicit-model", got.Model)
	assert.Equal(t, "explicit-key", got.APIKey)
}

func TestResolve_LinkedFromDbms_OverridesDefault(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{` +
		`"dbms":{"default-credential":"db","credentials":[` +
		`{"name":"db","username":"u","password":"p","database-name":"neo4j","uri":"neo4j://x:7687","embed-credential":"linked"}` +
		`]},` +
		`"embed":{"default-credential":"defaulted","credentials":[` +
		`{"name":"defaulted","provider":"openai","model":"def","base-url":"","dimensions":0,"api-key":"def-key"},` +
		`{"name":"linked","provider":"openai","model":"linked-model","base-url":"","dimensions":0,"api-key":"linked-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "linked-model", got.Model)
	assert.Equal(t, "linked-key", got.APIKey)
}

func TestResolve_LinkedFromDbms_StaleLinkErrors(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{` +
		`"dbms":{"default-credential":"db","credentials":[` +
		`{"name":"db","username":"u","password":"p","database-name":"neo4j","uri":"neo4j://x:7687","embed-credential":"missing"}` +
		`]},` +
		`"embed":{"default-credential":"","credentials":[]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	_, err := Resolve(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
	assert.Contains(t, err.Error(), "set-embed")
}

func TestResolve_DefaultEmbedCred_WhenNoLink(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{"embed":{"default-credential":"defaulted","credentials":[` +
		`{"name":"defaulted","provider":"openai","model":"def","base-url":"","dimensions":0,"api-key":"def-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "openai", got.Provider)
	assert.Equal(t, "def", got.Model)
	assert.Equal(t, "def-key", got.APIKey)
}

func TestResolve_Empty_NoCredsNoEnvNoFlags(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	cfg := newTestCfg(t, "{}")
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "", got.Provider)
	assert.Equal(t, "", got.Model)
	assert.Equal(t, "", got.BaseURL)
	assert.Equal(t, 0, got.Dimensions)
	assert.Equal(t, "", got.APIKey)
}

func TestResolveAPIKey_OpenAIPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		osEnv    map[string]string
		dotenv   map[string]string
		stored   string
		provider string
		want     string
	}{
		{
			name:     "stored only",
			provider: ProviderOpenAI,
			stored:   "stored-key",
			want:     "stored-key",
		},
		{
			name:     "dotenv generic beats stored",
			provider: ProviderOpenAI,
			stored:   "stored-key",
			dotenv:   map[string]string{envEmbedAPIKey: "dotenv-generic"},
			want:     "dotenv-generic",
		},
		{
			name:     "dotenv OPENAI_API_KEY beats dotenv generic and stored",
			provider: ProviderOpenAI,
			stored:   "stored-key",
			dotenv:   map[string]string{envEmbedAPIKey: "dotenv-generic", envOpenAIKey: "dotenv-openai"},
			want:     "dotenv-openai",
		},
		{
			name:     "OS env OPENAI_API_KEY beats every dotenv entry",
			provider: ProviderOpenAI,
			stored:   "stored-key",
			osEnv:    map[string]string{envOpenAIKey: "env-openai"},
			dotenv:   map[string]string{envEmbedAPIKey: "dotenv-generic", envOpenAIKey: "dotenv-openai"},
			want:     "env-openai",
		},
		{
			name:     "OS env NEO4J_EMBED_API_KEY but provider-specific OS env wins",
			provider: ProviderOpenAI,
			stored:   "stored-key",
			osEnv:    map[string]string{envEmbedAPIKey: "env-generic", envOpenAIKey: "env-openai"},
			want:     "env-openai",
		},
		{
			name:     "OS env NEO4J_EMBED_API_KEY only — no per-provider override",
			provider: ProviderOpenAI,
			stored:   "stored-key",
			osEnv:    map[string]string{envEmbedAPIKey: "env-generic"},
			want:     "env-generic",
		},
		{
			name:     "huggingface uses HF_TOKEN over generic",
			provider: ProviderHuggingFace,
			stored:   "stored",
			osEnv:    map[string]string{envEmbedAPIKey: "env-generic", envHFToken: "env-hf"},
			want:     "env-hf",
		},
		{
			name:     "ollama ignores OPENAI_API_KEY, generic still applies",
			provider: ProviderOllama,
			stored:   "",
			osEnv:    map[string]string{envOpenAIKey: "env-openai", envEmbedAPIKey: "env-generic"},
			want:     "env-generic",
		},
		{
			name:     "gemini stored only",
			provider: ProviderGemini,
			stored:   "stored-gemini",
			want:     "stored-gemini",
		},
		{
			name:     "gemini no env, no stored",
			provider: ProviderGemini,
			want:     "",
		},
		{
			name:     "gemini OS env GEMINI_API_KEY beats GOOGLE_API_KEY beats generic beats stored",
			provider: ProviderGemini,
			stored:   "stored-gemini",
			osEnv: map[string]string{
				envEmbedAPIKey: "env-generic",
				envGoogleKey:   "env-google",
				envGeminiKey:   "env-gemini",
			},
			want: "env-gemini",
		},
		{
			name:     "gemini OS env GOOGLE_API_KEY wins when GEMINI_API_KEY unset",
			provider: ProviderGemini,
			stored:   "stored-gemini",
			osEnv:    map[string]string{envEmbedAPIKey: "env-generic", envGoogleKey: "env-google"},
			want:     "env-google",
		},
		{
			name:     "gemini OS env NEO4J_EMBED_API_KEY wins when no gemini-specific env",
			provider: ProviderGemini,
			stored:   "stored-gemini",
			osEnv:    map[string]string{envEmbedAPIKey: "env-generic"},
			want:     "env-generic",
		},
		{
			name:     "gemini OS env GEMINI_API_KEY beats every dotenv entry",
			provider: ProviderGemini,
			stored:   "stored-gemini",
			osEnv:    map[string]string{envGeminiKey: "env-gemini"},
			dotenv:   map[string]string{envEmbedAPIKey: "dotenv-generic", envGoogleKey: "dotenv-google", envGeminiKey: "dotenv-gemini"},
			want:     "env-gemini",
		},
		{
			name:     "gemini dotenv GEMINI_API_KEY beats dotenv GOOGLE_API_KEY beats dotenv generic beats stored",
			provider: ProviderGemini,
			stored:   "stored-gemini",
			dotenv:   map[string]string{envEmbedAPIKey: "dotenv-generic", envGoogleKey: "dotenv-google", envGeminiKey: "dotenv-gemini"},
			want:     "dotenv-gemini",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEmbedEnv(t)
			// OS env reads in resolveAPIKey are gated; enable so the table's
			// osEnv entries are honoured.
			t.Setenv(envAcceptEnvVars, "1")
			cfg := newTestCfg(t, "{}")
			for k, v := range tc.osEnv {
				t.Setenv(k, v)
			}
			got := resolveAPIKey(cfg, tc.provider, tc.stored, tc.dotenv)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolve_FlagsForProviderAndDimensions(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	cfg := newTestCfg(t, "{}")
	cmd := newTestCmd(t,
		"--embed-provider=ollama",
		"--embed-model=nomic-embed-text",
		"--embed-base-url=http://ollama.example:11434",
		"--embed-dimensions=768",
	)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "ollama", got.Provider)
	assert.Equal(t, "nomic-embed-text", got.Model)
	assert.Equal(t, "http://ollama.example:11434", got.BaseURL)
	assert.Equal(t, 768, got.Dimensions)
}

func TestNew_OpenAIReturnsProvider(t *testing.T) {
	p, err := New(Config{Provider: ProviderOpenAI})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNew_OllamaReturnsProvider(t *testing.T) {
	p, err := New(Config{Provider: ProviderOllama})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNew_HuggingFaceReturnsProvider(t *testing.T) {
	p, err := New(Config{Provider: ProviderHuggingFace})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNew_EmptyProviderUsageError(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing embed provider")
	assert.Contains(t, err.Error(), "--embed-provider")
}

// TestNew_EmptyProviderMessageIsGateAware verifies the missing-provider usage
// error is gate-aware (REQ-F-017): with accept-env-vars off it must NOT
// advertise NEO4J_EMBED_PROVIDER as an effective fix (only as something to
// enable), and with it on it MAY name the variable directly.
func TestNew_EmptyProviderMessageIsGateAware(t *testing.T) {
	t.Run("off mode does not advertise the gated env var as a fix", func(t *testing.T) {
		_, err := New(Config{AcceptEnvVars: false})
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "missing embed provider")
		assert.Contains(t, msg, "--embed-provider")
		assert.Contains(t, msg, ".env")
		assert.Contains(t, msg, "stored embed credential")
		assert.Contains(t, msg, "enable accept-env-vars")
		// The var name may appear, but only behind "enable accept-env-vars".
		assert.NotContains(t, msg, "set --embed-provider, NEO4J_EMBED_PROVIDER",
			"off-mode message must not advertise NEO4J_EMBED_PROVIDER as a direct fix")
	})

	t.Run("on mode may name the env var directly", func(t *testing.T) {
		_, err := New(Config{AcceptEnvVars: true})
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "missing embed provider")
		assert.Contains(t, msg, "--embed-provider")
		assert.Contains(t, msg, "NEO4J_EMBED_PROVIDER")
		assert.NotContains(t, msg, "enable accept-env-vars")
	})
}

func TestNew_InvalidProvider(t *testing.T) {
	_, err := New(Config{Provider: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid embed provider")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), ProviderGemini)
	assert.Contains(t, err.Error(), ProviderVertex)
}

func TestResolve_StoredCred_CopiesVertexFields(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{"embed":{"default-credential":"v","credentials":[` +
		`{"name":"v","provider":"vertex","model":"text-embedding-005",` +
		`"base-url":"","dimensions":0,"api-key":"",` +
		`"vertex-project":"my-project","vertex-location":"us-central1"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "vertex", got.Provider)
	assert.Equal(t, "my-project", got.VertexProject)
	assert.Equal(t, "us-central1", got.VertexLocation)
}

// TestResolve_EnvGate_OffIgnoresEnvVars verifies that with accept-env-vars off
// the embed env vars are ignored and the stored credential is used.
func TestResolve_EnvGate_OffIgnoresEnvVars(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	t.Setenv(envEmbedProvider, "huggingface")
	t.Setenv(envEmbedModel, "env-model")
	t.Setenv(envEmbedAPIKey, "env-key")

	creds := `{"embed":{"default-credential":"d","credentials":[` +
		`{"name":"d","provider":"openai","model":"stored-model",` +
		`"base-url":"https://stored.example/v1","dimensions":256,"api-key":"stored-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "openai", got.Provider, "env provider must be ignored when gate off")
	assert.Equal(t, "stored-model", got.Model)
	assert.Equal(t, "stored-key", got.APIKey, "env key must be ignored when gate off")
}

// TestResolve_EnvGate_OnUsesEnvKey verifies that with accept-env-vars on the
// NEO4J_EMBED_API_KEY overrides the stored credential's key.
func TestResolve_EnvGate_OnUsesEnvKey(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(envEmbedAPIKey, "env-key")

	creds := `{"embed":{"default-credential":"d","credentials":[` +
		`{"name":"d","provider":"openai","model":"stored-model",` +
		`"base-url":"https://stored.example/v1","dimensions":256,"api-key":"stored-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "env-key", got.APIKey, "env key must override stored when gate on")
	assert.Equal(t, "openai", got.Provider)
}

// TestResolve_EnvGate_OnMissingKeyErrors verifies that with accept-env-vars on
// and NEO4J_EMBED_PROVIDER=openai but no key var, a usage error is returned.
func TestResolve_EnvGate_OnMissingKeyErrors(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(envEmbedProvider, "openai")

	cfg := newTestCfg(t, "{}")
	cmd := newTestCmd(t)

	_, err := Resolve(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing embed API key")
}

// TestResolve_EnvGate_OnProviderFromEnvKeyFromDotenv is the bug case from the
// review: provider arrives via NEO4J_EMBED_PROVIDER while the key arrives via
// the .env walk-up. The resolved Config has a key, so it must resolve cleanly.
func TestResolve_EnvGate_OnProviderFromEnvKeyFromDotenv(t *testing.T) {
	clearEmbedEnv(t)
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(envEmbedProvider, "openai")

	cfg := newTestCfg(t, "{}")
	withDotenvCwd(t, cfg.Aura.Fs(), "NEO4J_EMBED_API_KEY=dotenv-key\n")

	cmd := newTestCmd(t)
	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "openai", got.Provider)
	assert.Equal(t, "dotenv-key", got.APIKey, "key from .env must satisfy the env-mode check")
}

// TestResolve_EnvGate_OnProviderFromEnvKeyFromStored covers the other resolved
// source: provider via env, key from the stored embed credential.
func TestResolve_EnvGate_OnProviderFromEnvKeyFromStored(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(envEmbedProvider, "openai")

	creds := `{"embed":{"default-credential":"d","credentials":[` +
		`{"name":"d","provider":"openai","model":"stored-model",` +
		`"base-url":"https://stored.example/v1","dimensions":256,"api-key":"stored-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	cmd := newTestCmd(t)

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "openai", got.Provider)
	assert.Equal(t, "stored-key", got.APIKey, "key from stored cred must satisfy the env-mode check")
}

// TestResolve_EnvGate_OnNoKeyProvidersExempt verifies Ollama and Vertex never
// require a key even in env-var mode with no key from any source.
func TestResolve_EnvGate_OnNoKeyProvidersExempt(t *testing.T) {
	for _, provider := range []string{"ollama", "vertex"} {
		t.Run(provider, func(t *testing.T) {
			clearEmbedEnv(t)
			t.Chdir(t.TempDir())
			t.Setenv(envAcceptEnvVars, "1")
			t.Setenv(envEmbedProvider, provider)

			cfg := newTestCfg(t, "{}")
			cmd := newTestCmd(t)

			got, err := Resolve(cmd, cfg)
			require.NoError(t, err)
			assert.Equal(t, provider, got.Provider)
			assert.Empty(t, got.APIKey)
		})
	}
}

// TestResolve_EnvGate_OffDotenvUnaffected verifies the .env walk-up still
// overlays values even when accept-env-vars is off.
func TestResolve_EnvGate_OffDotenvUnaffected(t *testing.T) {
	clearEmbedEnv(t)

	creds := `{"embed":{"default-credential":"d","credentials":[` +
		`{"name":"d","provider":"openai","model":"stored-model",` +
		`"base-url":"https://stored.example/v1","dimensions":100,"api-key":"stored-key"}` +
		`]}}`
	cfg := newTestCfg(t, creds)
	withDotenvCwd(t, cfg.Aura.Fs(), "NEO4J_EMBED_MODEL=dotenv-model\nNEO4J_EMBED_API_KEY=dotenv-key\n")

	cmd := newTestCmd(t)
	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "dotenv-model", got.Model, ".env must overlay regardless of gate")
	assert.Equal(t, "dotenv-key", got.APIKey, ".env key must overlay regardless of gate")
}

// TestResolve_EnvGate_FlagBeatsEnv verifies an explicit flag still wins over an
// env var when the gate is on.
func TestResolve_EnvGate_FlagBeatsEnv(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(envEmbedModel, "env-model")

	cfg := newTestCfg(t, "{}")
	cmd := newTestCmd(t, "--embed-model=flag-model")

	got, err := Resolve(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "flag-model", got.Model, "explicit flag must beat env var")
}

func TestProviderFactorySeam(t *testing.T) {
	calls := 0
	stub := func(_ Config) (Provider, error) {
		calls++
		return stubProvider{}, nil
	}
	restore := WithFactory(stub)
	defer restore()

	p, err := providerFactory(Config{Provider: ProviderOpenAI})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, 1, calls)
}

// stubProvider is the minimal Provider used to confirm WithFactory routes
// through the seam. The implementation never runs in this file.
type stubProvider struct{}

func (stubProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, nil
}

func TestResolveSelectedDbmsCred(t *testing.T) {
	clearEmbedEnv(t)
	t.Chdir(t.TempDir())

	creds := `{"dbms":{"default-credential":"def","credentials":[` +
		`{"name":"def","username":"u","password":"p","database-name":"neo4j","uri":"neo4j://def:7687"},` +
		`{"name":"other","username":"u","password":"p","database-name":"neo4j","uri":"neo4j://other:7687"}` +
		`]}}`
	cfg := newTestCfg(t, creds)

	t.Run("default when --credential not set", func(t *testing.T) {
		cmd := newTestCmd(t)
		got := resolveSelectedDbmsCred(cmd, cfg)
		require.NotNil(t, got)
		assert.Equal(t, "def", got.Name)
	})

	t.Run("named when --credential set", func(t *testing.T) {
		cmd := newTestCmd(t, "--credential=other")
		got := resolveSelectedDbmsCred(cmd, cfg)
		require.NotNil(t, got)
		assert.Equal(t, "other", got.Name)
	})

	t.Run("nil when --credential names missing cred", func(t *testing.T) {
		cmd := newTestCmd(t, "--credential=nope")
		got := resolveSelectedDbmsCred(cmd, cfg)
		assert.Nil(t, got)
	})
}

// TestLoadDotenv_StopsAtGitBoundary verifies the embed-side .env walk halts
// at the first .git ancestor — a poison .env above the repo root must NOT
// be loaded into the resolved Config.
func TestLoadDotenv_StopsAtGitBoundary(t *testing.T) {
	clearEmbedEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)

	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)

	// Poison .env one level above tmp; tmp itself carries a .git boundary.
	parent := filepath.Dir(tmp)
	require.NoError(t, afero.WriteFile(fs, filepath.Join(parent, ".env"),
		[]byte("NEO4J_EMBED_API_KEY=poison\n"), 0644))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".git"),
		[]byte(""), 0644))

	// Empty home so only the .git boundary is exercised.
	restore := dotenv.SetHomeDirFnForTest(func() (string, error) { return "", nil })
	defer restore()

	got := loadDotenv(clicfg.NewConfig(fs, "test", clicfg.QueryScope), nil)
	assert.Empty(t, got, "poison .env above .git boundary must not be loaded")
}

// TestLoadDotenv_AnnouncesOverlay verifies an info: line appears on stderr
// when .env lives strictly above cwd, and stays silent when .env is in cwd.
func TestLoadDotenv_AnnouncesOverlay(t *testing.T) {
	clearEmbedEnv(t)

	t.Run("above cwd emits info line", func(t *testing.T) {
		tmp := t.TempDir()
		sub := filepath.Join(tmp, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		t.Chdir(sub)

		fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"),
			[]byte("NEO4J_EMBED_API_KEY=found\n"), 0644))

		// Empty home + no .git so the walk reaches tmp.
		restore := dotenv.SetHomeDirFnForTest(func() (string, error) { return "", nil })
		defer restore()

		var buf bytes.Buffer
		got := loadDotenv(clicfg.NewConfig(fs, "test", clicfg.QueryScope), &buf)
		assert.Equal(t, "found", got["NEO4J_EMBED_API_KEY"])
		assert.Contains(t, buf.String(), "info: loading .env from")
		assert.Contains(t, buf.String(), filepath.Join(tmp, ".env"))
	})

	t.Run("in cwd is silent", func(t *testing.T) {
		tmp := t.TempDir()
		t.Chdir(tmp)

		fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"),
			[]byte("NEO4J_EMBED_API_KEY=found\n"), 0644))

		var buf bytes.Buffer
		got := loadDotenv(clicfg.NewConfig(fs, "test", clicfg.QueryScope), &buf)
		assert.Equal(t, "found", got["NEO4J_EMBED_API_KEY"])
		assert.Empty(t, buf.String(), "no info line when .env is in cwd")
	})
}
