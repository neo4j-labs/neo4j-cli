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

func (c *Credentials) allCredentials() []keyringCredential {
	result := make([]keyringCredential, 0, len(c.Aura.Credentials)+len(c.Dbms.Credentials)+len(c.Embed.Credentials))
	for _, cred := range c.Aura.Credentials {
		result = append(result, cred)
	}
	for _, cred := range c.Dbms.Credentials {
		result = append(result, cred)
	}
	for _, cred := range c.Embed.Credentials {
		result = append(result, cred)
	}
	return result
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
// sensitive fields from the OS keyring. Warning messages are written to warnW.
func (c *Credentials) SetStorageMode(mode string, warnW io.Writer) error {
	c.storageMode = mode
	if mode == StorageModeKeyring {
		return c.loadSensitiveFieldsFromKeyring(warnW)
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
	c.saveToJSON()
	return nil
}

func (c *Credentials) saveToJSON() {
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
// can continue using them. Returns an error if a keyring.Set call fails (e.g.
// the OS keyring daemon is unavailable); file I/O and JSON marshal errors
// still panic, consistent with the insecure-mode save() path.
func (c *Credentials) saveWithKeyring() error {
	all := c.allCredentials()

	for _, cred := range all {
		if err := writeCredToKeyring(cred, defaultKeyring, nil); err != nil {
			return err
		}
	}

	// Save sensitive field values, zero them in-memory, write scrubbed JSON,
	// then restore. saveToJSON() panics on marshal/write errors so no error
	// return path requires the restore — it is unconditional.
	saved := make([][]string, len(all))
	for i, cred := range all {
		saved[i] = saveCredSensitiveFields(cred)
		zeroCredSensitiveFields(cred)
	}
	c.saveToJSON()
	for i, cred := range all {
		restoreCredSensitiveFields(cred, saved[i])
	}
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
// On full success: sensitive fields are zeroed in the in-memory structs,
// save() is called to scrub them from credentials.json, and storageMode is
// set to StorageModeKeyring. The caller is responsible for persisting the
// "credential-storage" config key.
func (c *Credentials) MigrateToKeyring() error {
	// Probe keyring availability before writing any entries.
	if err := ProbeKeyringAvailability(); err != nil {
		return clierr.NewUsageError(
			"keyring is unavailable (%v)\n%s",
			err, KeyringSetupHint(),
		)
	}

	all := c.allCredentials()

	// writtenKeys tracks every keyring key written so far so we can roll back
	// on partial failure.
	var writtenKeys []string

	rollback := func() {
		for _, key := range writtenKeys {
			_ = defaultKeyring.Delete(ServiceName, key)
		}
	}

	// Validate all credentials before writing any keyring entries.
	for _, cred := range all {
		if err := validateCredForMigration(cred); err != nil {
			return err
		}
	}

	// Write sensitive fields to the keyring, tracking each written key for rollback.
	for _, cred := range all {
		if err := writeCredToKeyring(cred, defaultKeyring, &writtenKeys); err != nil {
			rollback()
			return err
		}
	}

	// Zero sensitive in-memory fields and persist scrubbed JSON.
	for _, cred := range all {
		zeroCredSensitiveFields(cred)
	}
	if err := c.save(); err != nil {
		return err
	}

	c.storageMode = StorageModeKeyring
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
// On full success: storageMode is set to StorageModeInsecure, saveToJSON() is
// called to persist the secrets to JSON, then keyring.Delete() is called for
// all non-empty entries (best-effort: errors are ignored).
//
// The caller is responsible for persisting the "credential-storage" config key.
func (c *Credentials) MigrateToInsecure() error {
	// Phase 1: read all sensitive fields from the keyring into in-memory structs.
	// If any required field is missing (ErrNotFound) or any Get returns a
	// non-ErrNotFound error, abort immediately.

	// Track which in-memory fields we have populated so we can zero them on
	// failure or delete keyring entries on success.
	var filled []sensitiveField

	zero := func() {
		for _, f := range filled {
			*f.ptr = ""
		}
	}

	for _, cred := range c.allCredentials() {
		if err := migrateCredFromKeyring(cred, defaultKeyring, &filled); err != nil {
			zero()
			return err
		}
	}

	// Phase 2: persist secrets to JSON and own the mode transition.
	c.storageMode = StorageModeInsecure
	c.saveToJSON()

	// Phase 3: delete keyring entries for all fields we successfully read
	// (best-effort — errors are ignored).
	for _, f := range filled {
		_ = defaultKeyring.Delete(ServiceName, f.key)
	}

	return nil
}

// RemoveAura removes an Aura credential by name and, in keyring mode, deletes
// its keyring entries. Keyring cleanup failures are written as warnings to
// warnW and do not fail the removal.
func (c *Credentials) RemoveAura(name string, warnW io.Writer) error {
	cred, err := c.Aura.Get(name)
	if err != nil {
		return err
	}
	if err := c.Aura.Remove(name); err != nil {
		return err
	}
	if c.storageMode == StorageModeKeyring {
		if err := cred.deleteFromKeyring(defaultKeyring); err != nil {
			fmt.Fprintf(warnW, "Warning: %v\n", err) //nolint:errcheck
		}
	}
	return nil
}

// RemoveDbms removes a Dbms credential by name and, in keyring mode, deletes
// its keyring entries. Keyring cleanup failures are written as warnings to
// warnW and do not fail the removal.
func (c *Credentials) RemoveDbms(name string, warnW io.Writer) error {
	cred, err := c.Dbms.Get(name)
	if err != nil {
		return err
	}
	if err := c.Dbms.Remove(name); err != nil {
		return err
	}
	if c.storageMode == StorageModeKeyring {
		if err := cred.deleteFromKeyring(defaultKeyring); err != nil {
			fmt.Fprintf(warnW, "Warning: %v\n", err) //nolint:errcheck
		}
	}
	return nil
}

// RemoveEmbed removes an Embed credential by name and, in keyring mode, deletes
// its keyring entries. Keyring cleanup failures are written as warnings to
// warnW and do not fail the removal.
func (c *Credentials) RemoveEmbed(name string, warnW io.Writer) error {
	cred, err := c.Embed.Get(name)
	if err != nil {
		return err
	}
	if err := c.Embed.Remove(name); err != nil {
		return err
	}
	if c.storageMode == StorageModeKeyring {
		if err := cred.deleteFromKeyring(defaultKeyring); err != nil {
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
//   - If ErrNotFound and JSON value is also absent, a warning is written to
//     stderr and the field is left empty; the command continues (error only
//     surfaces if the credential is actually used).
//
// Non-ErrNotFound errors are returned as-is.
// save() is called at the end only when at least one field was auto-migrated.
func (c *Credentials) loadSensitiveFieldsFromKeyring(warnW io.Writer) error {
	// anyMigrated tracks whether at least one JSON-resident secret was
	// successfully pushed to the keyring so we only call save() when needed.
	// save() (→ saveWithKeyring) will write in-memory values to keyring
	// (idempotent) and produce a scrubbed JSON snapshot.
	anyMigrated := false

	for _, cred := range c.allCredentials() {
		if loadCredFromKeyring(cred, defaultKeyring, warnW) {
			anyMigrated = true
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

// writeCredToKeyring writes non-empty sensitive fields to the keyring.
// If written is non-nil, each successfully written key is appended to it.
func writeCredToKeyring(cred keyringCredential, provider KeyringProvider, written *[]string) error {
	for _, f := range cred.sensitiveFields() {
		if *f.ptr == "" {
			continue
		}
		if err := provider.Set(ServiceName, f.key, *f.ptr); err != nil {
			return fmt.Errorf("keyring set %s: %w", f.key, err)
		}
		if written != nil {
			*written = append(*written, f.key)
		}
	}
	return nil
}

func zeroCredSensitiveFields(cred keyringCredential) {
	for _, f := range cred.sensitiveFields() {
		*f.ptr = ""
	}
}

func saveCredSensitiveFields(cred keyringCredential) []string {
	fields := cred.sensitiveFields()
	vals := make([]string, len(fields))
	for i, f := range fields {
		vals[i] = *f.ptr
	}
	return vals
}

func restoreCredSensitiveFields(cred keyringCredential, vals []string) {
	for i, f := range cred.sensitiveFields() {
		*f.ptr = vals[i]
	}
}

func validateCredForMigration(cred keyringCredential) error {
	for _, f := range cred.sensitiveFields() {
		if f.required && *f.ptr == "" {
			return clierr.NewUsageError(
				"cannot migrate credential: %s is empty; remove and re-add the credential",
				f.key,
			)
		}
	}
	return nil
}

// loadCredFromKeyring populates sensitive fields from the keyring (startup/SetStorageMode path).
// For each field: if found, overwrite in-memory. If ErrNotFound + in-memory non-empty, auto-migrate
// to keyring (returns migrated=true on success). If ErrNotFound + required + empty, warn to warnW.
// On any non-ErrNotFound error, stop processing further fields and return migrated as-is.
func loadCredFromKeyring(cred keyringCredential, provider KeyringProvider, warnW io.Writer) (migrated bool) {
	for _, f := range cred.sensitiveFields() {
		val, err := provider.Get(ServiceName, f.key)
		if err == nil {
			*f.ptr = val
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			return migrated
		}
		if *f.ptr != "" {
			if setErr := provider.Set(ServiceName, f.key, *f.ptr); setErr == nil {
				migrated = true
			}
		} else if f.required {
			fmt.Fprintf(warnW, "Warning: keyring entry missing for credential (see key %s); remove and re-add the credential\n", f.key) //nolint:errcheck
		}
	}
	return migrated
}

// migrateCredFromKeyring reads sensitive fields from the keyring (MigrateToInsecure path).
// Required fields: ErrNotFound + in-memory non-empty → no-op (REQ-F-018); ErrNotFound + empty → error.
// Optional fields: ErrNotFound silently skips.
// Successfully populated fields are appended to filled for cleanup on success or failure.
func migrateCredFromKeyring(cred keyringCredential, provider KeyringProvider, filled *[]sensitiveField) error {
	for _, f := range cred.sensitiveFields() {
		val, err := provider.Get(ServiceName, f.key)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return fmt.Errorf("keyring get %s: %w", f.key, err)
			}
			if f.required && *f.ptr == "" {
				return clierr.NewUsageError(
					"cannot migrate credential: %s not found in keyring; remove and re-add the credential",
					f.key,
				)
			}
			continue
		}
		*f.ptr = val
		*filled = append(*filled, f)
	}
	return nil
}
