// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/neo4j/cli/common/clierr"
)

type AuraCredentials struct {
	DefaultCredential string            `json:"default-credential"`
	Credentials       []*AuraCredential `json:"credentials"`
	onUpdate          func()
}

func (c *AuraCredentials) Printable() PrintableAuraCredentials {
	return c.Credentials
}

func (c *AuraCredentials) Add(name string, clientId string, clientSecret string) error {
	auraCredentials := c.Credentials
	for _, credential := range auraCredentials {
		if credential.Name == name {
			return clierr.NewUsageError("already have credential with name %s", name)
		}
	}

	c.Credentials = append(c.Credentials, &AuraCredential{Name: name, ClientId: clientId, ClientSecret: clientSecret})
	if len(c.Credentials) == 1 {
		c.SetDefault(name) //nolint:errcheck // credential was just appended, so it always exists; error is impossible here
	}
	c.onUpdate()
	return nil
}

func (c *AuraCredentials) Remove(name string) error {
	var indexToRemove = -1

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

func (c *AuraCredentials) SetDefault(name string) error {
	if !c.credentialExists(name) {
		return clierr.NewUsageError("could not find credential with name %s", name)
	}

	c.DefaultCredential = name
	c.onUpdate()
	return nil
}

func (c *AuraCredentials) GetDefault() (*AuraCredential, error) {
	if c.DefaultCredential == "" {
		return nil, clierr.NewUsageError("default credential not set, please follow the instructions at https://neo4j.com/docs/aura/classic/platform/api/authentication/#_creating_credentials and use the `credential add` subcommand to add the created credentials")
	}
	return c.Get(c.DefaultCredential)
}

func (c *AuraCredentials) Get(name string) (*AuraCredential, error) {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return credential, nil
		}
	}
	return nil, clierr.NewUsageError("could not find credential with name %s", name)
}

func (c *AuraCredentials) UpdateAccessToken(cred *AuraCredential, accessToken string, expiresInSeconds int64) *AuraCredential {
	credential, err := c.Get(cred.Name)
	if err != nil {
		panic(err)
	}
	const expireToleranceSeconds = 60

	now := time.Now().UnixMilli()

	credential.TokenExpiry = now + (expiresInSeconds-expireToleranceSeconds)*1000
	credential.AccessToken = accessToken
	c.onUpdate()
	return credential
}

func (c *AuraCredentials) ClearAccessToken(cred *AuraCredential) (*AuraCredential, error) {
	credential, err := c.Get(cred.Name)
	if err != nil {
		return nil, err
	}

	credential.TokenExpiry = 0
	credential.AccessToken = ""
	c.onUpdate()
	return credential, nil
}

func (c *AuraCredentials) credentialExists(name string) bool {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return true
		}
	}
	return false
}

// PrintableAuraCredentials wraps a slice of AuraCredential and satisfies the
// common/output.ResponseData interface (AsArray + GetSingleOrError) via
// structural typing, so PrintBodyMap can render it as a table or JSON.
type PrintableAuraCredentials []*AuraCredential

// AsArray returns each credential as a {"name": ..., "type": ..., "identifier": ...}
// map for table rendering. Sensitive fields (client-secret, access-token) are omitted.
func (d PrintableAuraCredentials) AsArray() []map[string]any {
	result := make([]map[string]any, len(d))
	for i, cred := range d {
		result[i] = map[string]any{
			"name":       cred.Name,
			"type":       "aura-client",
			"identifier": cred.ClientId,
		}
	}
	return result
}

// GetSingleOrError returns the single credential map, or an error if the slice
// does not contain exactly one entry.
func (d PrintableAuraCredentials) GetSingleOrError() (map[string]any, error) {
	if len(d) != 1 {
		return nil, fmt.Errorf("expected exactly 1 credential, got %d", len(d))
	}
	return map[string]any{
		"name":       d[0].Name,
		"type":       "aura-client",
		"identifier": d[0].ClientId,
	}, nil
}

// MarshalJSON renders CredentialData as a JSON array of objects with name, type,
// and identifier fields, matching what the table renders.
func (d PrintableAuraCredentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.AsArray())
}

type AuraCredential struct {
	Name         string `json:"name"`
	ClientId     string `json:"client-id"`
	ClientSecret string `json:"client-secret"`
	AccessToken  string `json:"access-token"`
	TokenExpiry  int64  `json:"token-expiry"`
}

func (credential *AuraCredential) HasValidAccessToken() bool {
	now := time.Now().UnixMilli()

	if credential.AccessToken == "" {
		return false
	}

	if now >= credential.TokenExpiry {
		return false
	}

	return true
}
