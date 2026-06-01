// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/cli/common/clierr"
)

type AuraCredentials struct {
	DefaultCredential string            `json:"default-credential"`
	Credentials       []*AuraCredential `json:"credentials"`
	onUpdate          func() error
}

func (c *AuraCredentials) Printable() PrintableAuraCredentials {
	return PrintableAuraCredentials{
		credentials:       c.Credentials,
		defaultCredential: c.DefaultCredential,
	}
}

func (c *AuraCredentials) Add(name string, clientId string, clientSecret string) error {
	if name == envCredentialName {
		return errReservedEnvName(name)
	}
	auraCredentials := c.Credentials
	for _, credential := range auraCredentials {
		if credential.Name == name {
			return clierr.NewUsageError("already have credential with name %s", name)
		}
	}

	c.Credentials = append(c.Credentials, &AuraCredential{Name: name, ClientId: clientId, ClientSecret: clientSecret})
	if len(c.Credentials) == 1 {
		c.SetDefault(name) //nolint:errcheck // not-found error impossible here; any keyring error surfaces in the c.onUpdate() call below
	}
	return c.onUpdate()
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
	return c.onUpdate()
}

func (c *AuraCredentials) SetDefault(name string) error {
	if !c.credentialExists(name) {
		return clierr.NewUsageError("could not find credential with name %s", name)
	}

	c.DefaultCredential = name
	return c.onUpdate()
}

func (c *AuraCredentials) GetDefault() (*AuraCredential, error) {
	if c.DefaultCredential == "" {
		return nil, clierr.NewAuthError("default credential not set, create Aura API credentials at https://console.neo4j.io/account (see https://neo4j.com/docs/aura/api/authentication/) and run `credential aura-client add` to store them")
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

func (c *AuraCredentials) UpdateAccessToken(cred *AuraCredential, accessToken string, expiresInSeconds int64) (*AuraCredential, error) {
	credential, err := c.Get(cred.Name)
	if err != nil {
		panic(err)
	}
	const expireToleranceSeconds = 60

	now := time.Now().UnixMilli()

	credential.TokenExpiry = now + (expiresInSeconds-expireToleranceSeconds)*1000
	credential.AccessToken = accessToken
	return credential, c.onUpdate()
}

func (c *AuraCredentials) ClearAccessToken(cred *AuraCredential) (*AuraCredential, error) {
	credential, err := c.Get(cred.Name)
	if err != nil {
		return nil, err
	}

	credential.TokenExpiry = 0
	credential.AccessToken = ""
	if err := c.onUpdate(); err != nil {
		return nil, err
	}
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
// common/output.ResponseData interface (AsArray) via
// structural typing, so PrintBodyMap can render it as a table or JSON.
type PrintableAuraCredentials struct {
	credentials       []*AuraCredential
	defaultCredential string
}

// AsArray returns each credential as a {"name": ..., "type": ..., "identifier": ...}
// map for table rendering. Sensitive fields (client-secret, access-token) are omitted.
func (d PrintableAuraCredentials) AsArray() []map[string]any {
	result := make([]map[string]any, len(d.credentials))
	for i, cred := range d.credentials {
		result[i] = map[string]any{
			"name":       cred.Name,
			"type":       "aura-client",
			"identifier": cred.ClientId,
			"default":    cred.Name == d.defaultCredential,
		}
	}
	return result
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

// deleteFromKeyring removes all keyring entries for this Aura credential.
// ErrNotFound is silently ignored; other errors are joined and returned.
func (c *AuraCredential) deleteFromKeyring(provider KeyringProvider) error {
	deleteOne := func(field string) error {
		err := provider.Delete(ServiceName, KeyringKey("aura", c.Name, field))
		if err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("keyring delete aura/%s/%s: %w", c.Name, field, err)
		}
		return nil
	}
	return errors.Join(deleteOne("client-secret"), deleteOne("access-token"))
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

func (c *AuraCredential) sensitiveFields() []sensitiveField {
	return []sensitiveField{
		{ptr: &c.ClientSecret, key: KeyringKey("aura", c.Name, "client-secret"), required: true},
		{ptr: &c.AccessToken, key: KeyringKey("aura", c.Name, "access-token"), required: false},
	}
}
