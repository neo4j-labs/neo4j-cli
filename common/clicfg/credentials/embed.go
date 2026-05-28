// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/neo4j/cli/common/clierr"
)

type EmbedCredentials struct {
	DefaultCredential string             `json:"default-credential"`
	Credentials       []*EmbedCredential `json:"credentials"`
	onUpdate          func() error
}

func (c *EmbedCredentials) Printable() PrintableEmbedCredentials {
	return PrintableEmbedCredentials{
		credentials:       c.Credentials,
		defaultCredential: c.DefaultCredential,
	}
}

func (c *EmbedCredentials) Add(name, provider, model, baseURL, apiKey string, dimensions int) error {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return clierr.NewUsageError("already have credential with name %s", name)
		}
	}

	c.Credentials = append(c.Credentials, &EmbedCredential{
		Name:       name,
		Provider:   provider,
		Model:      model,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Dimensions: dimensions,
	})
	if len(c.Credentials) == 1 {
		c.SetDefault(name) //nolint:errcheck // not-found error impossible here; any keyring error surfaces in the c.onUpdate() call below
	}
	return c.onUpdate()
}

func (c *EmbedCredentials) Remove(name string) error {
	indexToRemove := -1

	for i, credential := range c.Credentials {
		if credential.Name == name {
			indexToRemove = i
			break
		}
	}

	if indexToRemove == -1 {
		return clierr.NewUsageError("could not find credential with name %s to remove", name)
	}

	if c.DefaultCredential == name {
		c.DefaultCredential = ""
	}

	c.Credentials = append(c.Credentials[:indexToRemove], c.Credentials[indexToRemove+1:]...)
	return c.onUpdate()
}

func (c *EmbedCredentials) SetDefault(name string) error {
	if !c.credentialExists(name) {
		return clierr.NewUsageError("could not find credential with name %s", name)
	}

	c.DefaultCredential = name
	return c.onUpdate()
}

func (c *EmbedCredentials) GetDefault() (*EmbedCredential, error) {
	if c.DefaultCredential == "" {
		return nil, nil
	}
	return c.Get(c.DefaultCredential)
}

func (c *EmbedCredentials) Get(name string) (*EmbedCredential, error) {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return credential, nil
		}
	}
	return nil, clierr.NewUsageError("could not find credential with name %s", name)
}

func (c *EmbedCredentials) List() []*EmbedCredential {
	return c.Credentials
}

func (c *EmbedCredentials) credentialExists(name string) bool {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return true
		}
	}
	return false
}

// PrintableEmbedCredentials wraps a slice of EmbedCredential and satisfies the
// common/output.ResponseData interface (AsArray) via structural typing, so PrintBodyMap
// can render it as a table or JSON.
type PrintableEmbedCredentials struct {
	credentials       []*EmbedCredential
	defaultCredential string
}

// AsArray returns each credential as a map for table rendering.
// APIKey is intentionally omitted.
func (d PrintableEmbedCredentials) AsArray() []map[string]any {
	result := make([]map[string]any, len(d.credentials))
	for i, cred := range d.credentials {
		result[i] = map[string]any{
			"name":       cred.Name,
			"provider":   cred.Provider,
			"model":      cred.Model,
			"base-url":   cred.BaseURL,
			"dimensions": cred.Dimensions,
			"default":    cred.Name == d.defaultCredential,
		}
	}
	return result
}

// MarshalJSON renders PrintableEmbedCredentials as a JSON array of objects,
// matching what the table renders. APIKey is intentionally omitted.
func (d PrintableEmbedCredentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.AsArray())
}

type EmbedCredential struct {
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	BaseURL    string `json:"base-url"`
	Dimensions int    `json:"dimensions"`
	APIKey     string `json:"api-key"`
}

// deleteFromKeyring removes the keyring entry for the named Embed credential.
// ErrNotFound is silently ignored; other errors are returned.
func (c *EmbedCredentials) deleteFromKeyring(provider KeyringProvider, name string) error {
	err := provider.Delete(ServiceName, KeyringKey("embed", name, "api-key"))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("keyring delete embed/%s/api-key: %w", name, err)
	}
	return nil
}

func (c *EmbedCredential) zeroSensitiveFields() {
	c.APIKey = ""
}

// writeToKeyring writes the non-empty APIKey to the keyring.
// If written is non-nil, each successfully written key is appended to it.
func (c *EmbedCredential) writeToKeyring(provider KeyringProvider, written *[]string) error {
	if c.APIKey != "" {
		key := KeyringKey("embed", c.Name, "api-key")
		if err := provider.Set(ServiceName, key, c.APIKey); err != nil {
			return fmt.Errorf("keyring set embed/%s/api-key: %w", c.Name, err)
		}
		if written != nil {
			*written = append(*written, key)
		}
	}
	return nil
}

func (c *EmbedCredential) saveSensitiveFields() []string {
	return []string{c.APIKey}
}

func (c *EmbedCredential) restoreSensitiveFields(fields []string) {
	c.APIKey = fields[0]
}

func (c *EmbedCredential) validateForMigration() error {
	return nil
}

// loadFromKeyring populates the APIKey from the keyring (startup/SetStorageMode path).
// APIKey is optional: ErrNotFound is fine; auto-migrate to keyring if JSON value present.
// Returns true if the field was successfully written to the keyring during auto-migration.
func (c *EmbedCredential) loadFromKeyring(provider KeyringProvider, _ io.Writer) (migrated bool) {
	key, err := provider.Get(ServiceName, KeyringKey("embed", c.Name, "api-key"))
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return false
		}
		if c.APIKey != "" {
			if setErr := provider.Set(ServiceName, KeyringKey("embed", c.Name, "api-key"), c.APIKey); setErr == nil {
				migrated = true
			}
		}
	} else {
		c.APIKey = key
	}
	return migrated
}

// migrateFromKeyring reads the APIKey from the keyring (MigrateToInsecure path).
// APIKey is optional: ErrNotFound silently skips.
// Successfully populated fields are appended to filled so the caller can delete keyring entries on success.
func (c *EmbedCredential) migrateFromKeyring(provider KeyringProvider, filled *[]migratedField) error {
	key, err := provider.Get(ServiceName, KeyringKey("embed", c.Name, "api-key"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("keyring get embed/%s/api-key: %w", c.Name, err)
	}
	c.APIKey = key
	*filled = append(*filled, migratedField{ptr: &c.APIKey, key: KeyringKey("embed", c.Name, "api-key")})
	return nil
}
