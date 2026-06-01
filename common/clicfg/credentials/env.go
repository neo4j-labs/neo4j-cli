// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"fmt"
	"io"
	"strconv"

	"github.com/neo4j/cli/common/clierr"
)

// envCredentialName is the reserved name used for the ephemeral credential
// synthesized in env mode for each credential type.
const envCredentialName = "env"

// errReservedEnvName is returned by the Add path of each credential type when a
// user tries to create a credential named "env", which is reserved for the
// ephemeral credential synthesized from environment variables in env mode.
func errReservedEnvName(name string) error {
	return clierr.NewUsageError(
		"credential name %q is reserved — it is used for the ephemeral credential synthesized from environment variables in 'credential-storage: env' mode. Pick a different name.",
		name)
}

const (
	envAuraClientID     = "NEO4J_AURA_CLIENT_ID"
	envAuraClientSecret = "NEO4J_AURA_CLIENT_SECRET"

	envDbmsURI      = "NEO4J_URI"
	envDbmsUsername = "NEO4J_USERNAME"
	envDbmsPassword = "NEO4J_PASSWORD"
	envDbmsDatabase = "NEO4J_DATABASE"

	envEmbedProvider   = "NEO4J_EMBED_PROVIDER"
	envEmbedModel      = "NEO4J_EMBED_MODEL"
	envEmbedBaseURL    = "NEO4J_EMBED_BASE_URL"
	envEmbedDimensions = "NEO4J_EMBED_DIMENSIONS"
	envEmbedAPIKey     = "NEO4J_EMBED_API_KEY"
)

// loadFromEnv synthesizes ephemeral default credentials from environment
// variables for each credential type that has its required vars present.
// It mutates the in-memory structs directly (never via Add/SetDefault) so no
// onUpdate→save side effect fires. Warnings about partial configuration are
// written to warnW.
func (c *Credentials) loadFromEnv(warnW io.Writer) {
	c.loadAuraFromEnv(warnW)
	c.loadDbmsFromEnv()
	c.loadEmbedFromEnv()
}

func (c *Credentials) loadAuraFromEnv(warnW io.Writer) {
	clientID := getenv(envAuraClientID)
	clientSecret := getenv(envAuraClientSecret)

	if clientID == "" && clientSecret == "" {
		return
	}
	if clientID == "" || clientSecret == "" {
		fmt.Fprintf(warnW, "Warning: env credential mode requires both %s and %s; Aura credential not synthesized\n", envAuraClientID, envAuraClientSecret) //nolint:errcheck
		return
	}

	cred := c.findOrAppendAura()
	cred.ClientId = clientID
	cred.ClientSecret = clientSecret
	if len(c.Aura.Credentials) == 1 || c.Aura.DefaultCredential == "" {
		c.Aura.DefaultCredential = envCredentialName
	}
}

func (c *Credentials) loadDbmsFromEnv() {
	password := getenv(envDbmsPassword)
	if password == "" {
		return
	}

	cred := c.findOrAppendDbms()
	cred.Password = password
	cred.URI = getenv(envDbmsURI)
	cred.Username = getenv(envDbmsUsername)
	cred.DatabaseName = getenv(envDbmsDatabase)
	if len(c.Dbms.Credentials) == 1 || c.Dbms.DefaultCredential == "" {
		c.Dbms.DefaultCredential = envCredentialName
	}
}

func (c *Credentials) loadEmbedFromEnv() {
	provider := getenv(envEmbedProvider)
	if provider == "" {
		return
	}

	cred := c.findOrAppendEmbed()
	cred.Provider = provider
	cred.Model = getenv(envEmbedModel)
	cred.BaseURL = getenv(envEmbedBaseURL)
	cred.APIKey = getenv(envEmbedAPIKey)
	if dims := getenv(envEmbedDimensions); dims != "" {
		if n, err := strconv.Atoi(dims); err == nil {
			cred.Dimensions = n
		}
	}
	if len(c.Embed.Credentials) == 1 || c.Embed.DefaultCredential == "" {
		c.Embed.DefaultCredential = envCredentialName
	}
}

// findOrAppendAura returns the existing in-memory "env" Aura credential,
// appending a fresh one if none exists, so synthesis overlays rather than
// duplicates.
func (c *Credentials) findOrAppendAura() *AuraCredential {
	for _, cred := range c.Aura.Credentials {
		if cred.Name == envCredentialName {
			return cred
		}
	}
	cred := &AuraCredential{Name: envCredentialName}
	c.Aura.Credentials = append(c.Aura.Credentials, cred)
	return cred
}

func (c *Credentials) findOrAppendDbms() *DbmsCredential {
	for _, cred := range c.Dbms.Credentials {
		if cred.Name == envCredentialName {
			return cred
		}
	}
	cred := &DbmsCredential{Name: envCredentialName}
	c.Dbms.Credentials = append(c.Dbms.Credentials, cred)
	return cred
}

func (c *Credentials) findOrAppendEmbed() *EmbedCredential {
	for _, cred := range c.Embed.Credentials {
		if cred.Name == envCredentialName {
			return cred
		}
	}
	cred := &EmbedCredential{Name: envCredentialName}
	c.Embed.Credentials = append(c.Embed.Credentials, cred)
	return cred
}
