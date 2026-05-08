// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"path/filepath"

	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/spf13/afero"
)

type CredentialsFile struct {
	Aura  *AuraCredentials  `json:"aura"`
	Dbms  *DbmsCredentials  `json:"dbms,omitempty"`
	Embed *EmbedCredentials `json:"embed,omitempty"`
}

type Credentials struct {
	fs       afero.Fs
	Aura     *AuraCredentials
	Dbms     *DbmsCredentials
	Embed    *EmbedCredentials
	filePath string
}

func NewCredentials(fs afero.Fs, configPrefix string) *Credentials {
	configPath := filepath.Join(configPrefix, "neo4j", "cli", "credentials.json")
	c := Credentials{
		fs:       fs,
		filePath: configPath,
	}
	c.load()
	return &c
}

func (c *Credentials) load() {
	data := fileutils.ReadFileSafe(c.fs, c.filePath)
	fileHasData := len(data) != 0

	credentials := CredentialsFile{
		Aura: &AuraCredentials{
			Credentials: []*AuraCredential{},
			onUpdate:    c.save,
		},
		Dbms: &DbmsCredentials{
			Credentials: []*DbmsCredential{},
			onUpdate:    c.save,
		},
		Embed: &EmbedCredentials{
			Credentials: []*EmbedCredential{},
			onUpdate:    c.save,
		},
	}
	if fileHasData {
		if err := json.Unmarshal(data, &credentials); err != nil {
			panic(err)
		}
	}

	c.Aura = credentials.Aura
	c.Dbms = credentials.Dbms
	c.Embed = credentials.Embed

	// Ensure onUpdate callbacks are wired even when loaded from file
	if c.Aura != nil {
		c.Aura.onUpdate = c.save
	}
	if c.Dbms != nil {
		c.Dbms.onUpdate = c.save
	}
	if c.Embed == nil {
		c.Embed = &EmbedCredentials{
			Credentials: []*EmbedCredential{},
		}
	}
	c.Embed.onUpdate = c.save

	if !fileHasData {
		c.save()
	}
}

func (c *Credentials) save() {
	data, err := json.Marshal(CredentialsFile{
		Aura:  c.Aura,
		Dbms:  c.Dbms,
		Embed: c.Embed,
	})
	if err != nil {
		panic(err)
	}

	fileutils.WriteFile(c.fs, c.filePath, data)
}
