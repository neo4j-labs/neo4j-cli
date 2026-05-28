// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (c *AuraCredential) zeroSensitiveFields() {
	c.ClientSecret = ""
	c.AccessToken = ""
}

// writeToKeyring writes non-empty sensitive fields to the keyring.
// If written is non-nil, each successfully written key is appended to it.
func (c *AuraCredential) writeToKeyring(provider KeyringProvider, written *[]string) error {
	if c.ClientSecret != "" {
		key := KeyringKey("aura", c.Name, "client-secret")
		if err := provider.Set(ServiceName, key, c.ClientSecret); err != nil {
			return fmt.Errorf("keyring set aura/%s/client-secret: %w", c.Name, err)
		}
		if written != nil {
			*written = append(*written, key)
		}
	}
	if c.AccessToken != "" {
		key := KeyringKey("aura", c.Name, "access-token")
		if err := provider.Set(ServiceName, key, c.AccessToken); err != nil {
			return fmt.Errorf("keyring set aura/%s/access-token: %w", c.Name, err)
		}
		if written != nil {
			*written = append(*written, key)
		}
	}
	return nil
}

func (c *AuraCredential) saveSensitiveFields() []string {
	return []string{c.ClientSecret, c.AccessToken}
}

func (c *AuraCredential) restoreSensitiveFields(fields []string) {
	c.ClientSecret = fields[0]
	c.AccessToken = fields[1]
}

func (c *AuraCredential) validateForMigration() error {
	if c.ClientSecret == "" {
		return clierr.NewUsageError(
			"cannot migrate credential %q: aura client-secret is empty; run `credential aura-client remove %s` and re-add it",
			c.Name, c.Name,
		)
	}
	return nil
}

// loadFromKeyring populates sensitive fields from the keyring (startup/SetStorageMode path).
// ClientSecret: ErrNotFound + JSON value present → auto-migrate to keyring (returns migrated=true);
// ErrNotFound + no JSON value → warn to warnW.
// AccessToken: optional; ErrNotFound is fine; auto-migrate if JSON value present.
// Returns true if at least one field was successfully written to the keyring during auto-migration.
func (c *AuraCredential) loadFromKeyring(provider KeyringProvider, warnW io.Writer) (migrated bool) {
	secret, err := provider.Get(ServiceName, KeyringKey("aura", c.Name, "client-secret"))
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return false
		}
		if c.ClientSecret == "" {
			fmt.Fprintf(warnW, "Warning: keyring entry missing for credential %q (aura client-secret); run `credential aura-client remove %s` and re-add it\n", c.Name, c.Name) //nolint:errcheck
		} else if setErr := provider.Set(ServiceName, KeyringKey("aura", c.Name, "client-secret"), c.ClientSecret); setErr == nil {
			migrated = true
		}
	} else {
		c.ClientSecret = secret
	}

	token, err := provider.Get(ServiceName, KeyringKey("aura", c.Name, "access-token"))
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return migrated
		}
		if c.AccessToken != "" {
			if setErr := provider.Set(ServiceName, KeyringKey("aura", c.Name, "access-token"), c.AccessToken); setErr == nil {
				migrated = true
			}
		}
	} else {
		c.AccessToken = token
	}

	return migrated
}

// migrateFromKeyring reads sensitive fields from the keyring (MigrateToInsecure path).
// ClientSecret is required: ErrNotFound + in-memory non-empty → no-op (REQ-F-018);
// ErrNotFound + empty → clierr.UsageError.
// AccessToken is optional: ErrNotFound silently skips.
// Successfully populated fields are appended to filled so the caller can zero them on failure
// or delete keyring entries on success.
func (c *AuraCredential) migrateFromKeyring(provider KeyringProvider, filled *[]migratedField) error {
	secret, err := provider.Get(ServiceName, KeyringKey("aura", c.Name, "client-secret"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if c.ClientSecret != "" {
				return nil
			}
			return clierr.NewUsageError(
				"cannot migrate credential %q: aura client-secret not found in keyring; run `credential aura-client remove %s` and re-add it",
				c.Name, c.Name,
			)
		}
		return fmt.Errorf("keyring get aura/%s/client-secret: %w", c.Name, err)
	}
	c.ClientSecret = secret
	*filled = append(*filled, migratedField{ptr: &c.ClientSecret, key: KeyringKey("aura", c.Name, "client-secret")})

	token, err := provider.Get(ServiceName, KeyringKey("aura", c.Name, "access-token"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("keyring get aura/%s/access-token: %w", c.Name, err)
	}
	c.AccessToken = token
	*filled = append(*filled, migratedField{ptr: &c.AccessToken, key: KeyringKey("aura", c.Name, "access-token")})

	return nil
}
