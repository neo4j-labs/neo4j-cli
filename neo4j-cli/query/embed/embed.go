// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package embed implements the embedding provider abstraction used by the
// `neo4j-cli query` tree. The package owns the Provider interface, the
// resolution of configuration (flags / env / .env / stored credentials), and
// the per-provider HTTP clients (added in subsequent tasks). The factory in
// New(cfg) translates a resolved Config into a concrete Provider; tests
// override the producer-side via the `providerFactory` seam.
package embed

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/subosito/gotenv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/dotenv"
	"github.com/neo4j/cli/common/clierr"
)

// Provider is the runtime interface implemented by each embedding backend.
// Embed is a one-shot text → vector call; the implementation owns its own
// HTTP client. Implementations honour ctx for cancellation; the production
// HTTP clients have no client-side timeout, so cancellation is the only
// supported abort path.
type Provider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Config is the resolved per-invocation embedding configuration. It is
// populated by Resolve and consumed by New (the provider factory) — Resolve
// does NOT validate (so the standalone :embed leaf can produce a clean usage
// error path); validation lives in New.
type Config struct {
	Provider       string
	Model          string
	BaseURL        string
	APIKey         string
	Dimensions     int
	UserAgent      string
	VertexProject  string
	VertexLocation string
}

// Provider name constants shared by the cobra flag-validator (see credential
// embed add) and the runtime factory.
const (
	ProviderOpenAI      = "openai"
	ProviderOllama      = "ollama"
	ProviderHuggingFace = "huggingface"
	ProviderGemini      = "gemini"
	ProviderVertex      = "vertex"
)

// Environment variable names. Keep these grouped here so a single grep across
// the package finds every external variable read.
const (
	envEmbedProvider   = "NEO4J_EMBED_PROVIDER"
	envEmbedModel      = "NEO4J_EMBED_MODEL"
	envEmbedBaseURL    = "NEO4J_EMBED_BASE_URL"
	envEmbedDimensions = "NEO4J_EMBED_DIMENSIONS"
	envEmbedAPIKey     = "NEO4J_EMBED_API_KEY"
	envOpenAIKey       = "OPENAI_API_KEY"
	envHFToken         = "HF_TOKEN"
	envGeminiKey       = "GEMINI_API_KEY"
	envGoogleKey       = "GOOGLE_API_KEY"
)

// providerFactory is the test seam for producer-side substitution. Production
// callers (run.go and the :embed leaf) call providerFactory(cfg); tests swap
// it for a closure that returns a stub Provider.
var providerFactory = New

// resolveSelectedDbmsCred returns the dbms credential the surrounding query
// command would use, using the same flag-precedence ladder as
// query.resolveConn but without performing any URI normalisation or
// connection logic. Returns nil when no credential applies.
//
//   - --credential <name> set explicitly → that named cred (or nil if missing)
//   - otherwise → cfg.Credentials.Dbms.GetDefault() (or nil if no default)
func resolveSelectedDbmsCred(cmd *cobra.Command, cfg *clicfg.Config) *credentials.DbmsCredential {
	if cfg == nil || cfg.Credentials == nil || cfg.Credentials.Dbms == nil {
		return nil
	}
	if f := cmd.Flag("credential"); f != nil && f.Changed {
		cred, err := cfg.Credentials.Dbms.Get(f.Value.String())
		if err != nil {
			return nil
		}
		return cred
	}
	cred, err := cfg.Credentials.Dbms.GetDefault()
	if err != nil {
		return nil
	}
	return cred
}

// pickBaseEmbedCred implements REQ-F-013 step 4 — the first-match list that
// chooses the stored embed credential whose fields seed Config before env / .env
// / flag overlay. Order:
//
//  1. --embed-credential <name> when set explicitly. Returns a usage error
//     when the named cred is missing (REQ-F-014).
//  2. The dbms cred selected by resolveSelectedDbmsCred has a non-empty
//     EmbedCredential link → that linked cred. A stale link returns a usage
//     error pointing the user at `credential dbms set-embed` (REQ-F-027).
//  3. cfg.Credentials.Embed.GetDefault().
//  4. nil (no stored cred).
func pickBaseEmbedCred(cmd *cobra.Command, cfg *clicfg.Config) (*credentials.EmbedCredential, error) {
	if cfg == nil || cfg.Credentials == nil || cfg.Credentials.Embed == nil {
		return nil, nil
	}

	if f := cmd.Flag("embed-credential"); f != nil && f.Changed {
		name := f.Value.String()
		cred, err := cfg.Credentials.Embed.Get(name)
		if err != nil {
			return nil, clierr.NewUsageError(
				"embed credential %q not found; run 'neo4j-cli credential embed list' to see available credentials",
				name)
		}
		return cred, nil
	}

	if dbms := resolveSelectedDbmsCred(cmd, cfg); dbms != nil && dbms.EmbedCredential != "" {
		linked, err := cfg.Credentials.Embed.Get(dbms.EmbedCredential)
		if err != nil {
			return nil, clierr.NewUsageError(
				"dbms credential %q references missing embed credential %q; run 'neo4j-cli credential dbms set-embed %s' to update",
				dbms.Name, dbms.EmbedCredential, dbms.Name)
		}
		return linked, nil
	}

	def, err := cfg.Credentials.Embed.GetDefault()
	if err != nil {
		return nil, nil
	}
	return def, nil
}

// Resolve produces a Config by merging (lowest → highest precedence):
//
//  1. Stored embed credential (picked via pickBaseEmbedCred).
//  2. .env file walked up from the current working directory.
//  3. OS environment variables.
//  4. CLI flags (--embed-{provider,model,base-url,dimensions,credential}).
//
// API-key resolution happens last and follows REQ-F-013 with a per-provider
// override path: provider-specific env (OPENAI_API_KEY / HF_TOKEN) →
// NEO4J_EMBED_API_KEY → stored credential's APIKey. .env entries override the
// stored credential but lose to OS env, matching the connection-side
// precedence applied in query/connect.go.
//
// Resolve does NOT validate required fields (Provider, Model, API key for
// OpenAI/HuggingFace). Validation lives in New so the standalone :embed leaf
// can surface a clean usage error path.
func Resolve(cmd *cobra.Command, cfg *clicfg.Config) (Config, error) {
	out := Config{}

	// 1. Stored embed credential (lowest precedence).
	base, err := pickBaseEmbedCred(cmd, cfg)
	if err != nil {
		return Config{}, err
	}
	if base != nil {
		out.Provider = base.Provider
		out.Model = base.Model
		out.BaseURL = base.BaseURL
		out.Dimensions = base.Dimensions
		out.APIKey = base.APIKey
		out.VertexProject = base.VertexProject
		out.VertexLocation = base.VertexLocation
	}

	// 2. .env walk-up (overrides stored cred, loses to env / flags).
	dotenvVals := loadDotenv(cfg, cmd.ErrOrStderr())

	apply := func(key string, dst *string) {
		if v, ok := dotenvVals[key]; ok && v != "" {
			*dst = v
		}
	}
	apply(envEmbedProvider, &out.Provider)
	apply(envEmbedModel, &out.Model)
	apply(envEmbedBaseURL, &out.BaseURL)
	if v, ok := dotenvVals[envEmbedDimensions]; ok && v != "" {
		if n, ok := parseDimensions(v); ok {
			out.Dimensions = n
		}
	}

	// 3. OS environment (gated behind accept-env-vars; the .env walk-up above
	// is intentionally NOT gated).
	if v := gatedGetenv(cfg, envEmbedProvider); v != "" {
		out.Provider = v
	}
	if v := gatedGetenv(cfg, envEmbedModel); v != "" {
		out.Model = v
	}
	if v := gatedGetenv(cfg, envEmbedBaseURL); v != "" {
		out.BaseURL = v
	}
	if v := gatedGetenv(cfg, envEmbedDimensions); v != "" {
		if n, ok := parseDimensions(v); ok {
			out.Dimensions = n
		}
	}

	// 4. Flags (highest precedence). Each "--embed-x" flag overrides only
	// when explicitly set so Resolve never clobbers a populated value with
	// the default empty string.
	if f := cmd.Flag("embed-provider"); f != nil && f.Changed {
		out.Provider = f.Value.String()
	}
	if f := cmd.Flag("embed-model"); f != nil && f.Changed {
		out.Model = f.Value.String()
	}
	if f := cmd.Flag("embed-base-url"); f != nil && f.Changed {
		out.BaseURL = f.Value.String()
	}
	if f := cmd.Flag("embed-dimensions"); f != nil && f.Changed {
		if n, ok := parseDimensions(f.Value.String()); ok {
			out.Dimensions = n
		}
	}

	// API key — provider-specific env beats generic embed env beats stored
	// credential. .env entries override the stored cred but not OS env, so
	// we read .env first then OS env.
	out.APIKey = resolveAPIKey(cfg, out.Provider, out.APIKey, dotenvVals)

	// In env-var mode, a provider that needs an API key but resolved none from
	// any source (flag / env / .env / stored) is a usage error (REQ-F-010 embed
	// group). Validate the RESOLVED Config — not raw os.Getenv — so a key
	// supplied via .env or a stored credential counts as present, matching the
	// DBMS check in dbconn.ResolveConn and the documented precedence. Gated to
	// env-var mode so the .env / stored-credential paths are otherwise
	// unaffected.
	if cfg != nil && cfg.Global.AcceptEnvVars() {
		if out.Provider != "" && providerNeedsKey(out.Provider) && out.APIKey == "" {
			return Config{}, clierr.NewUsageError(
				"missing embed API key for provider %q: set --embed-provider's key via "+
					"NEO4J_EMBED_API_KEY (or the provider-specific variable), a .env file, "+
					"or a stored embed credential",
				out.Provider)
		}
	}

	// User agent matches the rest of the query package (`neo4j-cli/v<version>`).
	version := "dev"
	if cfg != nil && cfg.Version != "" {
		version = cfg.Version
	}
	out.UserAgent = "neo4j-cli/v" + version

	return out, nil
}

// providerNeedsKey reports whether a provider requires an API key. Ollama
// needs none and Vertex authenticates via Application Default Credentials, so
// both are exempt — matching the resolveAPIKey logic below.
func providerNeedsKey(provider string) bool {
	switch strings.ToLower(provider) {
	case ProviderOllama, ProviderVertex:
		return false
	default:
		return true
	}
}

// resolveAPIKey applies provider-specific API key resolution. OpenAI,
// HuggingFace, and Gemini honour a provider-specific env, then the generic
// embed env; Ollama and Vertex are left untouched (Ollama needs no API key,
// Vertex authenticates via ADC). For Gemini, GEMINI_API_KEY beats
// GOOGLE_API_KEY beats NEO4J_EMBED_API_KEY beats stored within each stage.
// Returns the highest-precedence non-empty value, falling back to storedKey.
func resolveAPIKey(cfg *clicfg.Config, provider, storedKey string, dotenv map[string]string) string {
	out := storedKey

	// Stage values in lowest → highest precedence (.env < OS env). Apply
	// per-provider keys before the generic one in each stage so a per-
	// provider variable always wins inside its precedence tier.
	stages := []map[string]string{dotenv, osEnvSnapshot(cfg)}
	for _, stage := range stages {
		// Generic key applies to any provider that needs an API key.
		if v := stage[envEmbedAPIKey]; v != "" {
			out = v
		}
		switch provider {
		case ProviderOpenAI:
			if v := stage[envOpenAIKey]; v != "" {
				out = v
			}
		case ProviderHuggingFace:
			if v := stage[envHFToken]; v != "" {
				out = v
			}
		case ProviderGemini:
			// GOOGLE_API_KEY first, then GEMINI_API_KEY so the
			// gemini-specific var wins inside this stage.
			if v := stage[envGoogleKey]; v != "" {
				out = v
			}
			if v := stage[envGeminiKey]; v != "" {
				out = v
			}
		}
	}
	return out
}

// osEnvSnapshot returns the subset of os.Getenv values relevant to API-key
// resolution. Built on demand so a test that calls t.Setenv before Resolve
// sees the change. Reads are gated behind accept-env-vars; when the gate is
// off every value is empty so the stored credential / .env path wins.
func osEnvSnapshot(cfg *clicfg.Config) map[string]string {
	return map[string]string{
		envEmbedAPIKey: gatedGetenv(cfg, envEmbedAPIKey),
		envOpenAIKey:   gatedGetenv(cfg, envOpenAIKey),
		envHFToken:     gatedGetenv(cfg, envHFToken),
		envGeminiKey:   gatedGetenv(cfg, envGeminiKey),
		envGoogleKey:   gatedGetenv(cfg, envGoogleKey),
	}
}

// gatedGetenv returns os.Getenv(name) only when accept-env-vars is enabled;
// otherwise it returns "" so credential env vars are ignored. The dotenv
// (--env walk-up) mechanism is intentionally NOT gated by this helper.
func gatedGetenv(cfg *clicfg.Config, name string) string {
	if cfg == nil || !cfg.Global.AcceptEnvVars() {
		return ""
	}
	return os.Getenv(name)
}

// loadDotenv walks up from cwd looking for a `.env` file via the shared
// dotenv.Find helper (stops at the first .git ancestor or the $HOME
// boundary). Returns an empty (non-nil) map when no file is found or the FS
// is unavailable. Errors during read are swallowed — a malformed .env file
// should not block the embed path; the connection resolver will surface a
// clearer error if the file is also corrupt. When the discovered .env lives
// in a directory strictly above cwd an `info: loading .env from <path>` line
// is written to stderr so the overlay isn't silent.
func loadDotenv(cfg *clicfg.Config, stderr io.Writer) map[string]string {
	if cfg == nil || cfg.Aura == nil {
		return map[string]string{}
	}
	fs := cfg.Aura.Fs()
	if fs == nil {
		return map[string]string{}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return map[string]string{}
	}
	path, ok, aboveCWD := dotenv.Find(fs, cwd)
	if !ok {
		return map[string]string{}
	}
	if aboveCWD && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "info: loading .env from %s\n", path)
	}
	f, err := fs.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer f.Close() //nolint:errcheck // read-only close error is not actionable in a defer
	parsed := gotenv.Parse(f)
	out := make(map[string]string, len(parsed))
	for k, v := range parsed {
		out[k] = v
	}
	return out
}

// parseDimensions parses a Dimensions value, returning (0, false) on failure
// so callers can leave the default in place rather than wedging zero into the
// resolved Config.
func parseDimensions(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// New builds a concrete Provider from a resolved Config. Validation of
// required fields lives here (rather than in Resolve) so callers can produce
// a clear usage error path with the resolved values in hand.
//
// Per-provider implementations are added in subsequent tasks (006-008); this
// skeleton returns a "not implemented" error for each known provider so the
// runtime wiring in tasks 010 / 011 can compile and unit-test against the
// providerFactory seam without depending on the HTTP clients.
func New(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "":
		return nil, clierr.NewUsageError(
			"missing embed provider: set --embed-provider, NEO4J_EMBED_PROVIDER, or pick a stored embed credential")
	case ProviderOpenAI:
		return newOpenAIProvider(cfg), nil
	case ProviderOllama:
		return newOllamaProvider(cfg), nil
	case ProviderHuggingFace:
		return newHuggingFaceProvider(cfg), nil
	case ProviderGemini:
		return newGeminiProvider(cfg), nil
	case ProviderVertex:
		return newVertexProvider(cfg), nil
	default:
		return nil, clierr.NewUsageError(
			"invalid embed provider %q: must be one of %s, %s, %s, %s, %s",
			cfg.Provider, ProviderOpenAI, ProviderOllama, ProviderHuggingFace, ProviderGemini, ProviderVertex)
	}
}

// Factory returns the providerFactory seam value so external callers (run.go,
// :embed leaf) can route through it without exposing the unexported variable
// directly. Tests inside this package replace providerFactory; tests outside
// inject via WithFactory below.
func Factory() func(Config) (Provider, error) {
	return providerFactory
}

// WithFactory swaps providerFactory with fn for the lifetime of the returned
// restore function. Designed for use as
//
//	restore := embed.WithFactory(stubFactory)
//	defer restore()
//
// from tests in other packages (run_test.go, embed_test.go for the leaf).
// Restoring is via the returned closure rather than t.Cleanup so the helper
// stays test-framework-agnostic.
func WithFactory(fn func(Config) (Provider, error)) func() {
	prev := providerFactory
	providerFactory = fn
	return func() { providerFactory = prev }
}
