// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"

	"github.com/neo4j/cli/common/clierr"
)

type EmbedCredentials struct {
	DefaultCredential string             `json:"default-credential"`
	Credentials       []*EmbedCredential `json:"credentials"`
	onUpdate          func()
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
		c.SetDefault(name) //nolint:errcheck // credential was just appended, so it always exists; error is impossible here
	}
	c.onUpdate()
	return nil
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
	c.onUpdate()
	return nil
}

func (c *EmbedCredentials) SetDefault(name string) error {
	if !c.credentialExists(name) {
		return clierr.NewUsageError("could not find credential with name %s", name)
	}

	c.DefaultCredential = name
	c.onUpdate()
	return nil
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
