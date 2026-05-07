// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
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
	t.Setenv(envEmbedProvider, "")
	t.Setenv(envEmbedModel, "")
	t.Setenv(envEmbedBaseURL, "")
	t.Setenv(envEmbedDimensions, "")
	t.Setenv(envEmbedAPIKey, "")
	t.Setenv(envOpenAIKey, "")
	t.Setenv(envHFToken, "")
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearEmbedEnv(t)
			for k, v := range tc.osEnv {
				t.Setenv(k, v)
			}
			got := resolveAPIKey(tc.provider, tc.stored, tc.dotenv)
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

func TestNew_InvalidProvider(t *testing.T) {
	_, err := New(Config{Provider: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid embed provider")
	assert.Contains(t, err.Error(), "bogus")
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
