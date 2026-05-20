// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"

	"github.com/neo4j/cli/common/clierr"
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

// SetEmbed links a named dbms credential to an embed credential. Passing an
// empty embedName clears the link. Returns a usage error when dbmsName does
// not exist.
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

// PrintableDbmsCredentials wraps a slice of DbmsCredential and satisfies the
// common/output.ResponseData interface (AsArray) via structural typing, so PrintBodyMap
// can render it as a table or JSON.
type PrintableDbmsCredentials struct {
	credentials       []*DbmsCredential
	defaultCredential string
}

// AsArray returns each credential as a map for table rendering.
// Password is intentionally omitted. The `embed-credential` key is always
// emitted (empty string when unset) so the column is stable across rows.
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

// MarshalJSON renders PrintableDbmsCredentials as a JSON array of objects,
// matching what the table renders. Password is intentionally omitted.
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
