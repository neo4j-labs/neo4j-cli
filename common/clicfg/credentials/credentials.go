// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"errors"
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

// StorageModeInsecure stores sensitive fields as plaintext in credentials.json.
const StorageModeInsecure = "insecure"

// StorageModeKeyring stores sensitive fields in the OS keyring.
const StorageModeKeyring = "keyring"

type CredentialsFile struct {
	Aura  *AuraCredentials  `json:"aura"`
	Dbms  *DbmsCredentials  `json:"dbms,omitempty"`
	Embed *EmbedCredentials `json:"embed,omitempty"`
}

type Credentials struct {
	fs          afero.Fs
	Aura        *AuraCredentials
	Dbms        *DbmsCredentials
	Embed       *EmbedCredentials
	filePath    string
	storageMode string
}

func NewCredentials(fs afero.Fs, configPrefix string) *Credentials {
	configPath := filepath.Join(configPrefix, "neo4j", "cli", "credentials.json")
	c := Credentials{
		fs:          fs,
		filePath:    configPath,
		storageMode: StorageModeInsecure,
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

// SetStorageMode sets the credential storage mode and reloads sensitive fields.
// mode must be either StorageModeInsecure or StorageModeKeyring. This is called
// from clicfg.NewConfig after the storage mode is resolved from config.
// In insecure mode sensitive fields are already loaded from JSON during
// NewCredentials, so reloading is a no-op. In keyring mode this populates
// sensitive fields from the OS keyring.
func (c *Credentials) SetStorageMode(mode string) error {
	c.storageMode = mode
	if mode == StorageModeKeyring {
		return c.loadSensitiveFieldsFromKeyring()
	}
	return nil
}

// StorageMode returns the current storage mode.
func (c *Credentials) StorageMode() string {
	return c.storageMode
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
	if c.storageMode == StorageModeKeyring {
		c.saveWithKeyring()
		return
	}

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

// saveWithKeyring writes sensitive fields to the OS keyring and zeroes them
// in the JSON file. In-memory values remain populated so the current process
// can continue using them.
func (c *Credentials) saveWithKeyring() {
	// Write sensitive fields to keyring before zeroing them in the JSON snapshot.
	for _, cred := range c.Aura.Credentials {
		if cred.ClientSecret != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("aura", cred.Name, "client-secret"), cred.ClientSecret); err != nil {
				panic(fmt.Errorf("keyring set aura/%s/client-secret: %w", cred.Name, err))
			}
		}
		if cred.AccessToken != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("aura", cred.Name, "access-token"), cred.AccessToken); err != nil {
				panic(fmt.Errorf("keyring set aura/%s/access-token: %w", cred.Name, err))
			}
		}
	}
	for _, cred := range c.Dbms.Credentials {
		if cred.Password != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("dbms", cred.Name, "password"), cred.Password); err != nil {
				panic(fmt.Errorf("keyring set dbms/%s/password: %w", cred.Name, err))
			}
		}
	}
	for _, cred := range c.Embed.Credentials {
		if cred.APIKey != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("embed", cred.Name, "api-key"), cred.APIKey); err != nil {
				panic(fmt.Errorf("keyring set embed/%s/api-key: %w", cred.Name, err))
			}
		}
	}

	// Build scrubbed snapshots for JSON serialisation.
	auraSnap := &AuraCredentials{
		DefaultCredential: c.Aura.DefaultCredential,
		Credentials:       make([]*AuraCredential, len(c.Aura.Credentials)),
	}
	for i, cred := range c.Aura.Credentials {
		cp := *cred
		cp.ClientSecret = ""
		cp.AccessToken = ""
		auraSnap.Credentials[i] = &cp
	}

	dbmsSnap := &DbmsCredentials{
		DefaultCredential: c.Dbms.DefaultCredential,
		Credentials:       make([]*DbmsCredential, len(c.Dbms.Credentials)),
	}
	for i, cred := range c.Dbms.Credentials {
		cp := *cred
		cp.Password = ""
		dbmsSnap.Credentials[i] = &cp
	}

	embedSnap := &EmbedCredentials{
		DefaultCredential: c.Embed.DefaultCredential,
		Credentials:       make([]*EmbedCredential, len(c.Embed.Credentials)),
	}
	for i, cred := range c.Embed.Credentials {
		cp := *cred
		cp.APIKey = ""
		embedSnap.Credentials[i] = &cp
	}

	data, err := json.Marshal(CredentialsFile{
		Aura:  auraSnap,
		Dbms:  dbmsSnap,
		Embed: embedSnap,
	})
	if err != nil {
		panic(err)
	}
	fileutils.WriteFile(c.fs, c.filePath, data)
}

// loadSensitiveFieldsFromKeyring populates in-memory sensitive fields from the
// OS keyring. It is called by SetStorageMode when switching to keyring mode.
// For each sensitive field:
//   - If found in keyring, the in-memory value is overwritten.
//   - If ErrNotFound but the JSON still holds a value (pre-migration state),
//     the JSON value is used silently.
//   - If ErrNotFound and JSON value is also absent, a clierr.UsageError is
//     returned (REQ-NF-004).
//
// Non-ErrNotFound errors are returned as-is.
func (c *Credentials) loadSensitiveFieldsFromKeyring() error {
	for _, cred := range c.Aura.Credentials {
		secret, err := defaultKeyring.Get(ServiceName, KeyringKey("aura", cred.Name, "client-secret"))
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return fmt.Errorf("keyring get aura/%s/client-secret: %w", cred.Name, err)
			}
			// ErrNotFound: fall back to JSON value if present, error if not
			if cred.ClientSecret == "" {
				return clierr.NewUsageError("keyring entry missing for credential %q (aura client-secret); run `credential aura-client remove %s` and re-add it", cred.Name, cred.Name)
			}
			// JSON fallback (pre-migration state) — use silently
		} else {
			cred.ClientSecret = secret
		}

		token, err := defaultKeyring.Get(ServiceName, KeyringKey("aura", cred.Name, "access-token"))
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return fmt.Errorf("keyring get aura/%s/access-token: %w", cred.Name, err)
			}
			// AccessToken is optional — ErrNotFound is fine; keep JSON value if any
		} else {
			cred.AccessToken = token
		}
	}

	for _, cred := range c.Dbms.Credentials {
		pwd, err := defaultKeyring.Get(ServiceName, KeyringKey("dbms", cred.Name, "password"))
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return fmt.Errorf("keyring get dbms/%s/password: %w", cred.Name, err)
			}
			if cred.Password == "" {
				return clierr.NewUsageError("keyring entry missing for credential %q (dbms password); run `credential dbms remove %s` and re-add it", cred.Name, cred.Name)
			}
		} else {
			cred.Password = pwd
		}
	}

	for _, cred := range c.Embed.Credentials {
		key, err := defaultKeyring.Get(ServiceName, KeyringKey("embed", cred.Name, "api-key"))
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return fmt.Errorf("keyring get embed/%s/api-key: %w", cred.Name, err)
			}
			// APIKey is optional — ErrNotFound is fine; keep JSON value if any
		} else {
			cred.APIKey = key
		}
	}

	return nil
}
