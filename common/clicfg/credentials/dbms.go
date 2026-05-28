// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neo4j/cli/common/clierr"
)

// Reserved: `query --credential` prefix dispatch intercepts these before persisted-store lookup, so a stored credential of either form would be unreachable.
const (
	reservedNameDesktop      = "desktop"
	reservedConnectionPrefix = "desktop-connection:"
)

type DbmsCredentials struct {
	DefaultCredential string            `json:"default-credential"`
	Credentials       []*DbmsCredential `json:"credentials"`
	onUpdate          func() error
}

func (c *DbmsCredentials) Printable() PrintableDbmsCredentials {
	return PrintableDbmsCredentials{
		credentials:       c.Credentials,
		defaultCredential: c.DefaultCredential,
	}
}

func (c *DbmsCredentials) Add(name, username, password, databaseName, uri string) error {
	if name == reservedNameDesktop || strings.HasPrefix(name, reservedConnectionPrefix) {
		return clierr.NewUsageError(
			"credential name %q is reserved — 'query --credential desktop' and 'query --credential desktop-connection:<uuid>' resolve against the running Neo4j Desktop 2 instance, not the persisted store. Pick a different name.",
			name)
	}
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return clierr.NewUsageError("already have credential with name %s", name)
		}
	}

	c.Credentials = append(c.Credentials, &DbmsCredential{
		Name:         name,
		Username:     username,
		Password:     password,
		DatabaseName: databaseName,
		URI:          uri,
	})
	if len(c.Credentials) == 1 {
		c.SetDefault(name) //nolint:errcheck // not-found error impossible here; any keyring error surfaces in the c.onUpdate() call below
	}
	return c.onUpdate()
}

func (c *DbmsCredentials) Remove(name string) error {
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

// SetEmbed links a dbms credential to an embed credential; empty embedName clears the link.
func (c *DbmsCredentials) SetEmbed(dbmsName, embedName string) error {
	for _, credential := range c.Credentials {
		if credential.Name == dbmsName {
			credential.EmbedCredential = embedName
			return c.onUpdate()
		}
	}
	return clierr.NewUsageError("could not find credential with name %s", dbmsName)
}

func (c *DbmsCredentials) SetDefault(name string) error {
	if !c.credentialExists(name) {
		return clierr.NewUsageError("could not find credential with name %s", name)
	}

	c.DefaultCredential = name
	return c.onUpdate()
}

func (c *DbmsCredentials) GetDefault() (*DbmsCredential, error) {
	if c.DefaultCredential == "" {
		return nil, nil
	}
	return c.Get(c.DefaultCredential)
}

func (c *DbmsCredentials) Get(name string) (*DbmsCredential, error) {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return credential, nil
		}
	}
	return nil, clierr.NewUsageError("could not find credential with name %s", name)
}

func (c *DbmsCredentials) List() []*DbmsCredential {
	return c.Credentials
}

func (c *DbmsCredentials) credentialExists(name string) bool {
	for _, credential := range c.Credentials {
		if credential.Name == name {
			return true
		}
	}
	return false
}

// PrintableDbmsCredentials renders DbmsCredentials as a table or JSON via common/output.
type PrintableDbmsCredentials struct {
	credentials       []*DbmsCredential
	defaultCredential string
}

// AsArray emits each credential as a map; Password is omitted, `embed-credential` is always present (empty when unset) so the column stays stable.
func (d PrintableDbmsCredentials) AsArray() []map[string]any {
	result := make([]map[string]any, len(d.credentials))
	for i, cred := range d.credentials {
		result[i] = map[string]any{
			"name":             cred.Name,
			"username":         cred.Username,
			"database-name":    cred.DatabaseName,
			"uri":              cred.URI,
			"embed-credential": cred.EmbedCredential,
			"default":          cred.Name == d.defaultCredential,
		}
	}
	return result
}

// MarshalJSON emits the same shape as AsArray; Password is omitted.
func (d PrintableDbmsCredentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.AsArray())
}

type DbmsCredential struct {
	Name            string `json:"name"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	DatabaseName    string `json:"database-name"`
	URI             string `json:"uri"`
	EmbedCredential string `json:"embed-credential,omitempty"`
}

// deleteFromKeyring removes the keyring entry for the named Dbms credential.
// ErrNotFound is silently ignored; other errors are returned.
func (c *DbmsCredentials) deleteFromKeyring(provider KeyringProvider, name string) error {
	err := provider.Delete(ServiceName, KeyringKey("dbms", name, "password"))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("keyring delete dbms/%s/password: %w", name, err)
	}
	return nil
}

func (c *DbmsCredential) zeroSensitiveFields() {
	c.Password = ""
}

// writeToKeyring writes the non-empty Password to the keyring.
// If written is non-nil, each successfully written key is appended to it.
func (c *DbmsCredential) writeToKeyring(provider KeyringProvider, written *[]string) error {
	if c.Password != "" {
		key := KeyringKey("dbms", c.Name, "password")
		if err := provider.Set(ServiceName, key, c.Password); err != nil {
			return fmt.Errorf("keyring set dbms/%s/password: %w", c.Name, err)
		}
		if written != nil {
			*written = append(*written, key)
		}
	}
	return nil
}

func (c *DbmsCredential) saveSensitiveFields() []string {
	return []string{c.Password}
}

func (c *DbmsCredential) restoreSensitiveFields(fields []string) {
	c.Password = fields[0]
}

func (c *DbmsCredential) validateForMigration() error {
	if c.Password == "" {
		return clierr.NewUsageError(
			"cannot migrate credential %q: dbms password is empty; run `credential dbms remove %s` and re-add it",
			c.Name, c.Name,
		)
	}
	return nil
}

// loadFromKeyring populates the Password from the keyring (startup/SetStorageMode path).
// ErrNotFound + JSON value present → auto-migrate to keyring (returns migrated=true);
// ErrNotFound + no JSON value → warn to warnW.
// Returns true if the field was successfully written to the keyring during auto-migration.
func (c *DbmsCredential) loadFromKeyring(provider KeyringProvider, warnW io.Writer) (migrated bool) {
	pwd, err := provider.Get(ServiceName, KeyringKey("dbms", c.Name, "password"))
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return false
		}
		if c.Password == "" {
			fmt.Fprintf(warnW, "Warning: keyring entry missing for credential %q (dbms password); run `credential dbms remove %s` and re-add it\n", c.Name, c.Name) //nolint:errcheck
		} else if setErr := provider.Set(ServiceName, KeyringKey("dbms", c.Name, "password"), c.Password); setErr == nil {
			migrated = true
		}
	} else {
		c.Password = pwd
	}
	return migrated
}

// migrateFromKeyring reads the Password from the keyring (MigrateToInsecure path).
// ErrNotFound + in-memory non-empty → no-op (REQ-F-018); ErrNotFound + empty → clierr.UsageError.
// Successfully populated fields are appended to filled so the caller can zero them on failure
// or delete keyring entries on success.
func (c *DbmsCredential) migrateFromKeyring(provider KeyringProvider, filled *[]migratedField) error {
	pwd, err := provider.Get(ServiceName, KeyringKey("dbms", c.Name, "password"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if c.Password != "" {
				return nil
			}
			return clierr.NewUsageError(
				"cannot migrate credential %q: dbms password not found in keyring; run `credential dbms remove %s` and re-add it",
				c.Name, c.Name,
			)
		}
		return fmt.Errorf("keyring get dbms/%s/password: %w", c.Name, err)
	}
	c.Password = pwd
	*filled = append(*filled, migratedField{ptr: &c.Password, key: KeyringKey("dbms", c.Name, "password")})
	return nil
}
