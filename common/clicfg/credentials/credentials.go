// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// HasAnyCredentials reports whether at least one credential exists across all
// three credential types (Aura, Dbms, Embed). This is used by the first-run
// default-detection logic to choose between insecure (existing user) and
// keyring (fresh install) as the initial storage mode.
func (c *Credentials) HasAnyCredentials() bool {
	return len(c.Aura.Credentials) > 0 ||
		len(c.Dbms.Credentials) > 0 ||
		len(c.Embed.Credentials) > 0
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
			_ = c.save() //nolint:errcheck // storageMode is insecure during load(); keyring path never reached
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
		_ = c.save() //nolint:errcheck // storageMode is insecure during load(); keyring path never reached
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

func (c *Credentials) save() error {
	if c.storageMode == StorageModeKeyring {
		return c.saveWithKeyring()
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
	return nil
}

// saveWithKeyring writes sensitive fields to the OS keyring and zeroes them
// in the JSON file. In-memory values remain populated so the current process
// can continue using them. Returns an error if a keyring.Set call fails (e.g.
// the OS keyring daemon is unavailable); file I/O and JSON marshal errors
// still panic, consistent with the insecure-mode save() path.
func (c *Credentials) saveWithKeyring() error {
	// Write sensitive fields to keyring before zeroing them in the JSON snapshot.
	for _, cred := range c.Aura.Credentials {
		if cred.ClientSecret != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("aura", cred.Name, "client-secret"), cred.ClientSecret); err != nil {
				return fmt.Errorf("keyring set aura/%s/client-secret: %w", cred.Name, err)
			}
		}
		if cred.AccessToken != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("aura", cred.Name, "access-token"), cred.AccessToken); err != nil {
				return fmt.Errorf("keyring set aura/%s/access-token: %w", cred.Name, err)
			}
		}
	}
	for _, cred := range c.Dbms.Credentials {
		if cred.Password != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("dbms", cred.Name, "password"), cred.Password); err != nil {
				return fmt.Errorf("keyring set dbms/%s/password: %w", cred.Name, err)
			}
		}
	}
	for _, cred := range c.Embed.Credentials {
		if cred.APIKey != "" {
			if err := defaultKeyring.Set(ServiceName, KeyringKey("embed", cred.Name, "api-key"), cred.APIKey); err != nil {
				return fmt.Errorf("keyring set embed/%s/api-key: %w", cred.Name, err)
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
	return nil
}

// MigrateToKeyring moves all sensitive credential fields from in-memory (and
// JSON) to the OS keyring. It iterates all three credential types.
//
// The keyring is probed for availability before any credentials are touched.
// If the keyring daemon is unreachable, a clierr.UsageError is returned
// immediately, before any keyring writes, with a hint to use insecure mode.
//
// Required fields (ClientSecret for Aura, Password for Dbms): if empty, the
// migration is aborted, all keyring entries written so far are rolled back via
// keyring.Delete(), and a clierr.UsageError is returned naming the credential
// and suggesting removal.
//
// Optional fields (AccessToken for Aura, APIKey for Embed): silently skipped
// when empty.
//
// Any keyring.Set() failure triggers a full rollback.
//
// On full success: sensitive fields are zeroed in the in-memory structs and
// save() is called to scrub them from credentials.json.
//
// The caller is responsible for persisting the "credential-storage" config key.
// This method must be called while storageMode is still StorageModeInsecure so
// that the final save() writes the scrubbed (zeroed) values to JSON rather than
// dispatching to saveWithKeyring(). The caller should call SetStorageMode after
// persisting the config key.
func (c *Credentials) MigrateToKeyring() error {
	// Probe keyring availability before writing any entries.
	if err := ProbeKeyringAvailability(); err != nil {
		return clierr.NewUsageError(
			"keyring is unavailable (%v); run `neo4j-cli config set credential-storage insecure --rw` to use plaintext storage instead",
			err,
		)
	}

	// written tracks every (user-key) pair written so far so we can roll back
	// on partial failure.
	type entry struct{ user string }
	var written []entry

	rollback := func() {
		for _, e := range written {
			// best-effort; ignore errors on rollback
			_ = defaultKeyring.Delete(ServiceName, e.user)
		}
	}

	setKey := func(user, value string) error {
		if err := defaultKeyring.Set(ServiceName, user, value); err != nil {
			rollback()
			return fmt.Errorf("keyring set %s: %w", user, err)
		}
		written = append(written, entry{user: user})
		return nil
	}

	// --- Aura credentials ---
	for _, cred := range c.Aura.Credentials {
		// ClientSecret is required
		if cred.ClientSecret == "" {
			rollback()
			return clierr.NewUsageError(
				"cannot migrate credential %q: aura client-secret is empty; run `credential aura-client remove %s` and re-add it",
				cred.Name, cred.Name,
			)
		}
		if err := setKey(KeyringKey("aura", cred.Name, "client-secret"), cred.ClientSecret); err != nil {
			return err
		}
		// AccessToken is optional
		if cred.AccessToken != "" {
			if err := setKey(KeyringKey("aura", cred.Name, "access-token"), cred.AccessToken); err != nil {
				return err
			}
		}
	}

	// --- Dbms credentials ---
	for _, cred := range c.Dbms.Credentials {
		// Password is required
		if cred.Password == "" {
			rollback()
			return clierr.NewUsageError(
				"cannot migrate credential %q: dbms password is empty; run `credential dbms remove %s` and re-add it",
				cred.Name, cred.Name,
			)
		}
		if err := setKey(KeyringKey("dbms", cred.Name, "password"), cred.Password); err != nil {
			return err
		}
	}

	// --- Embed credentials ---
	for _, cred := range c.Embed.Credentials {
		// APIKey is optional
		if cred.APIKey != "" {
			if err := setKey(KeyringKey("embed", cred.Name, "api-key"), cred.APIKey); err != nil {
				return err
			}
		}
	}

	// Zero sensitive in-memory fields and persist scrubbed JSON.
	for _, cred := range c.Aura.Credentials {
		cred.ClientSecret = ""
		cred.AccessToken = ""
	}
	for _, cred := range c.Dbms.Credentials {
		cred.Password = ""
	}
	for _, cred := range c.Embed.Credentials {
		cred.APIKey = ""
	}
	if err := c.save(); err != nil {
		return err
	}

	return nil
}

// MigrateToInsecure moves all sensitive credential fields from the OS keyring
// into credentials.json (plaintext). It iterates all three credential types
// and calls keyring.Get() for each sensitive field.
//
// Required fields (ClientSecret for Aura, Password for Dbms): if ErrNotFound,
// the migration is aborted (any in-memory fields already populated are
// re-zeroed) and a clierr.UsageError is returned naming the credential and
// suggesting removal.
//
// Optional fields (AccessToken for Aura, APIKey for Embed): if ErrNotFound,
// the field is silently skipped (empty value written to JSON).
//
// Any non-ErrNotFound keyring error is a hard error for all field types and
// aborts migration.
//
// On full success: save() is called to persist the secrets to JSON, then
// keyring.Delete() is called for all non-empty entries (best-effort: errors are
// ignored).
//
// The caller is responsible for persisting the "credential-storage" config key
// and calling SetStorageMode(StorageModeInsecure) afterwards. This method
// temporarily sets storageMode to insecure internally to flush secrets to JSON,
// then restores it so the caller can observe the original mode and switch it
// explicitly.
func (c *Credentials) MigrateToInsecure() error {
	// Phase 1: read all sensitive fields from the keyring into in-memory structs.
	// If any required field is missing (ErrNotFound) or any Get returns a
	// non-ErrNotFound error, abort immediately.

	// Track which in-memory fields we have populated so we can zero them on
	// failure.
	type populated struct {
		ptr   *string
		field string
	}
	var filled []populated

	zero := func() {
		for _, p := range filled {
			*p.ptr = ""
		}
	}

	getRequired := func(ptr *string, credType, credName, field string) error {
		val, err := defaultKeyring.Get(ServiceName, KeyringKey(credType, credName, field))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// REQ-F-018: if the in-memory value is already non-empty (populated
				// via the REQ-F-016 JSON fallback during load), the secret is already
				// in JSON — treat as a no-op (no keyring entry to delete either).
				if *ptr != "" {
					return nil
				}
				zero()
				return clierr.NewUsageError(
					"cannot migrate credential %q: %s %s not found in keyring; run `credential %s remove %s` and re-add it",
					credName, credType, field, credType, credName,
				)
			}
			zero()
			return fmt.Errorf("keyring get %s/%s/%s: %w", credType, credName, field, err)
		}
		*ptr = val
		filled = append(filled, populated{ptr: ptr, field: KeyringKey(credType, credName, field)})
		return nil
	}

	getOptional := func(ptr *string, credType, credName, field string) error {
		val, err := defaultKeyring.Get(ServiceName, KeyringKey(credType, credName, field))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// Silently skip — write empty value to JSON
				return nil
			}
			zero()
			return fmt.Errorf("keyring get %s/%s/%s: %w", credType, credName, field, err)
		}
		*ptr = val
		filled = append(filled, populated{ptr: ptr, field: KeyringKey(credType, credName, field)})
		return nil
	}

	// --- Aura credentials ---
	for _, cred := range c.Aura.Credentials {
		if err := getRequired(&cred.ClientSecret, "aura", cred.Name, "client-secret"); err != nil {
			return err
		}
		if err := getOptional(&cred.AccessToken, "aura", cred.Name, "access-token"); err != nil {
			return err
		}
	}

	// --- Dbms credentials ---
	for _, cred := range c.Dbms.Credentials {
		if err := getRequired(&cred.Password, "dbms", cred.Name, "password"); err != nil {
			return err
		}
	}

	// --- Embed credentials ---
	for _, cred := range c.Embed.Credentials {
		if err := getOptional(&cred.APIKey, "embed", cred.Name, "api-key"); err != nil {
			return err
		}
	}

	// Phase 2: persist secrets to JSON. Temporarily switch storageMode to
	// insecure so save() writes the full plaintext values instead of calling
	// saveWithKeyring() again.
	prevMode := c.storageMode
	c.storageMode = StorageModeInsecure
	_ = c.save() //nolint:errcheck // temporarily insecure; keyring path never reached
	c.storageMode = prevMode

	// Phase 3: delete keyring entries for all fields we successfully read
	// (best-effort — errors are ignored).
	for _, p := range filled {
		_ = defaultKeyring.Delete(ServiceName, p.field)
	}

	return nil
}

// DeleteKeyringEntries removes all keyring entries associated with a credential
// identified by credType ("aura", "dbms", or "embed") and name. It is called
// by the credential remove commands after the credential has been removed from
// the JSON file. Deletions are best-effort: ErrNotFound is silently ignored
// (the entry may already be absent). Any other error is returned as a warning
// but does not block the caller.
//
// Sensitive field mapping:
//
//	aura:  client-secret (required), access-token (optional)
//	dbms:  password (required)
//	embed: api-key (optional)
func (c *Credentials) DeleteKeyringEntries(credType, name string) error {
	deleteOne := func(field string) error {
		err := defaultKeyring.Delete(ServiceName, KeyringKey(credType, name, field))
		if err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("keyring delete %s/%s/%s: %w", credType, name, field, err)
		}
		return nil
	}

	switch credType {
	case "aura":
		var errs []error
		if err := deleteOne("client-secret"); err != nil {
			errs = append(errs, err)
		}
		if err := deleteOne("access-token"); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	case "dbms":
		return deleteOne("password")
	case "embed":
		return deleteOne("api-key")
	default:
		return fmt.Errorf("unknown credential type %q", credType)
	}
}

// RemoveAura removes an Aura credential by name and, in keyring mode, deletes
// its keyring entries. Keyring cleanup failures are written as warnings to
// warnW and do not fail the removal.
func (c *Credentials) RemoveAura(name string, warnW io.Writer) error {
	if err := c.Aura.Remove(name); err != nil {
		return err
	}
	if c.storageMode == StorageModeKeyring {
		if err := c.DeleteKeyringEntries("aura", name); err != nil {
			fmt.Fprintf(warnW, "Warning: %v\n", err) //nolint:errcheck
		}
	}
	return nil
}

// RemoveDbms removes a Dbms credential by name and, in keyring mode, deletes
// its keyring entries. Keyring cleanup failures are written as warnings to
// warnW and do not fail the removal.
func (c *Credentials) RemoveDbms(name string, warnW io.Writer) error {
	if err := c.Dbms.Remove(name); err != nil {
		return err
	}
	if c.storageMode == StorageModeKeyring {
		if err := c.DeleteKeyringEntries("dbms", name); err != nil {
			fmt.Fprintf(warnW, "Warning: %v\n", err) //nolint:errcheck
		}
	}
	return nil
}

// RemoveEmbed removes an Embed credential by name and, in keyring mode, deletes
// its keyring entries. Keyring cleanup failures are written as warnings to
// warnW and do not fail the removal.
func (c *Credentials) RemoveEmbed(name string, warnW io.Writer) error {
	if err := c.Embed.Remove(name); err != nil {
		return err
	}
	if c.storageMode == StorageModeKeyring {
		if err := c.DeleteKeyringEntries("embed", name); err != nil {
			fmt.Fprintf(warnW, "Warning: %v\n", err) //nolint:errcheck
		}
	}
	return nil
}

// loadSensitiveFieldsFromKeyring populates in-memory sensitive fields from the
// OS keyring. It is called by SetStorageMode when switching to keyring mode.
// For each sensitive field:
//   - If found in keyring, the in-memory value is overwritten.
//   - If ErrNotFound but the JSON still holds a value (REQ-F-016, pre-migration
//     state): the JSON value is used silently and an auto-migration is attempted
//     (REQ-F-019): keyring.Set() is called with the JSON value. On success the
//     field is zeroed in the JSON struct (to be persisted by save() after all
//     fields are processed). On failure the scrub is silently skipped for this
//     invocation; retry happens automatically on the next command.
//   - If ErrNotFound and JSON value is also absent, a clierr.UsageError is
//     returned (REQ-NF-004).
//
// Non-ErrNotFound errors are returned as-is.
// save() is called at the end only when at least one field was auto-migrated.
func (c *Credentials) loadSensitiveFieldsFromKeyring() error {
	// anyMigrated tracks whether at least one JSON-resident secret was
	// successfully pushed to the keyring so we only call save() when needed.
	// save() (→ saveWithKeyring) will write in-memory values to keyring
	// (idempotent) and produce a scrubbed JSON snapshot.
	anyMigrated := false

	// tryAutoMigrate attempts to push a JSON-resident value into the keyring.
	// On success it sets anyMigrated so save() is called at the end to scrub
	// the JSON. The in-memory field is intentionally kept populated so the
	// current command can use the secret. Failures are silently ignored
	// (retry on next command); the JSON value continues to serve as fallback.
	tryAutoMigrate := func(val, credType, credName, field string) {
		if val == "" {
			return
		}
		if setErr := defaultKeyring.Set(ServiceName, KeyringKey(credType, credName, field), val); setErr == nil {
			anyMigrated = true
		}
		// keyring.Set failure → skip scrub this run; JSON fallback remains active
	}

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
			// JSON fallback (REQ-F-016) — auto-migrate to keyring (REQ-F-019)
			tryAutoMigrate(cred.ClientSecret, "aura", cred.Name, "client-secret")
		} else {
			cred.ClientSecret = secret
		}

		token, err := defaultKeyring.Get(ServiceName, KeyringKey("aura", cred.Name, "access-token"))
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return fmt.Errorf("keyring get aura/%s/access-token: %w", cred.Name, err)
			}
			// AccessToken is optional — ErrNotFound is fine; keep JSON value if any
			// Still attempt auto-migration if the JSON value is present (REQ-F-019)
			tryAutoMigrate(cred.AccessToken, "aura", cred.Name, "access-token")
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
			// JSON fallback (REQ-F-016) — auto-migrate to keyring (REQ-F-019)
			tryAutoMigrate(cred.Password, "dbms", cred.Name, "password")
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
			// Still attempt auto-migration if the JSON value is present (REQ-F-019)
			tryAutoMigrate(cred.APIKey, "embed", cred.Name, "api-key")
		} else {
			cred.APIKey = key
		}
	}

	// Persist the scrubbed JSON only if at least one field was auto-migrated.
	// saveWithKeyring() writes in-memory values to keyring (idempotent since
	// they were already set by tryAutoMigrate) and produces a scrubbed JSON
	// snapshot, removing the plaintext secrets from credentials.json.
	if anyMigrated {
		_ = c.save() //nolint:errcheck // best-effort JSON scrub; error is non-fatal here
	}

	return nil
}
