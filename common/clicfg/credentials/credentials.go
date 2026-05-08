// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
)

// nowUnix is overridable in tests so the timestamp suffix on the corrupt
// backup file is deterministic.
var nowUnix = func() int64 { return time.Now().Unix() }

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
	if err := c.load(); err != nil {
		// Surface the error via panic to match the existing fatal-error flow
		// (main()'s recover prints and exits). The backup-and-reset side
		// effects have already run inside load(), so the next invocation
		// will succeed with empty credentials.
		panic(err)
	}
	return &c
}

// load reads credentials.json into the Credentials struct.
//
// On a malformed file, load backs up the corrupt bytes to
// <path>.corrupt-<unix-ts>, resets the in-memory state to empty
// credentials, persists the empty state to disk, and returns a
// clierr.FatalError naming the backup path. The current invocation
// fails fast; subsequent invocations succeed with empty credentials.
func (c *Credentials) load() error {
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
			backupPath, backupErr := c.backupCorruptFile(data)
			// Reset to empty credentials regardless of backup outcome so
			// the next invocation does not re-trip the same parse error.
			c.resetToEmpty()
			c.save()
			if backupErr != nil {
				return clierr.NewFatalError("credentials file %s is corrupt and could not be parsed (%v); additionally failed to back it up: %v. The file has been reset to empty credentials.", c.filePath, err, backupErr)
			}
			return clierr.NewFatalError("credentials file %s is corrupt and could not be parsed (%v); a copy has been saved to %s and the file has been reset to empty credentials.", c.filePath, err, backupPath)
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
	return nil
}

// resetToEmpty wires fresh empty credentials with onUpdate callbacks.
func (c *Credentials) resetToEmpty() {
	c.Aura = &AuraCredentials{
		Credentials: []*AuraCredential{},
		onUpdate:    c.save,
	}
	c.Dbms = &DbmsCredentials{
		Credentials: []*DbmsCredential{},
		onUpdate:    c.save,
	}
	c.Embed = &EmbedCredentials{
		Credentials: []*EmbedCredential{},
		onUpdate:    c.save,
	}
}

// backupCorruptFile writes the original (corrupt) bytes to
// <path>.corrupt-<unix-ts> and returns the backup path.
func (c *Credentials) backupCorruptFile(data []byte) (string, error) {
	backupPath := fmt.Sprintf("%s.corrupt-%d", c.filePath, nowUnix())
	if err := afero.WriteFile(c.fs, backupPath, data, 0o600); err != nil {
		return "", err
	}
	return backupPath, nil
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
