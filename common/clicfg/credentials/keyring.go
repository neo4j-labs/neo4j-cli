// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"errors"
	"fmt"
	"io"

	gokeyring "github.com/zalando/go-keyring"
)

// ServiceName is the keyring service name used for all neo4j-cli secrets.
const ServiceName = "neo4j-cli"

// KeyringProvider is the interface used by the credentials package to interact
// with the OS keyring. The three methods mirror the go-keyring package-level
// functions so that tests can swap in a mock without cgo or a real keyring.
type KeyringProvider interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// defaultKeyring is the package-level provider. Tests replace it via
// SetKeyringProviderForTest (exported through export_test.go).
var defaultKeyring KeyringProvider = &goKeyringProvider{}

// goKeyringProvider delegates to the go-keyring package-level functions, which
// route to the OS keyring on each platform (Keychain on macOS, Credential
// Manager on Windows, libsecret/kwallet on Linux).
type goKeyringProvider struct{}

func (p *goKeyringProvider) Get(service, user string) (string, error) {
	return gokeyring.Get(service, user)
}

func (p *goKeyringProvider) Set(service, user, password string) error {
	return gokeyring.Set(service, user, password)
}

func (p *goKeyringProvider) Delete(service, user string) error {
	return gokeyring.Delete(service, user)
}

// ErrNotFound is re-exported from go-keyring so callers in this package and
// its tests can compare errors without importing go-keyring directly.
var ErrNotFound = gokeyring.ErrNotFound

// KeyringKey returns the user-key string used to identify a specific sensitive
// field in the keyring. The format is "<type>/<name>/<field>", for example
// "aura/prod/client-secret". This is stable across platforms and unambiguous
// across credential types and names.
func KeyringKey(credType, name, field string) string {
	return fmt.Sprintf("%s/%s/%s", credType, name, field)
}

// keyringCredential is the common interface satisfied by AuraCredential,
// DbmsCredential, and EmbedCredential. It allows credential-agnostic loops in
// saveWithKeyring, MigrateToKeyring, MigrateToInsecure, and
// loadSensitiveFieldsFromKeyring.
type keyringCredential interface {
	writeToKeyring(provider KeyringProvider, written *[]string) error
	loadFromKeyring(provider KeyringProvider, warnW io.Writer) (migrated bool)
	migrateFromKeyring(provider KeyringProvider, filled *[]migratedField) error
	validateForMigration() error
	zeroSensitiveFields()
	saveSensitiveFields() []string
	restoreSensitiveFields(fields []string)
}

// migratedField records an in-memory field that was populated from the keyring
// during MigrateToInsecure so the caller can zero fields on failure or delete
// keyring entries on success.
type migratedField struct {
	ptr *string
	key string
}

// probeKey is the sentinel user-key used by ProbeKeyringAvailability to test
// whether the OS keyring daemon is reachable without touching real credentials.
const probeKey = "__neo4j-cli-probe__"

// ProbeKeyringAvailability tests whether the OS keyring is reachable. It calls
// Get with a well-known probe key and considers the keyring available if the
// result is nil or ErrNotFound (daemon is running but entry is absent). Any
// other error indicates that the keyring daemon is unavailable (e.g. no D-Bus
// session on headless Linux, locked Keychain on macOS) and is returned as-is.
func ProbeKeyringAvailability() error {
	_, err := defaultKeyring.Get(ServiceName, probeKey)
	if err == nil || errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
