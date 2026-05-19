// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package configmigrate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// configPath is where seedTestFs seeds config.json.
//
// We do NOT import common/clicfg here (would create an import cycle since
// clicfg imports configmigrate) nor test/utils/testfs (transitively imports
// clicfg). Hard-code the path; it only needs to be consistent within the
// in-memory fs used by these tests.
func configPath() string {
	return filepath.Join("neo4j", "cli", "config.json")
}

// seedTestFs returns an in-memory fs seeded with the given config.json body.
// An empty body means no file is created (used to exercise the missing-file
// path). Mirrors the surface of testfs.GetTestFs(config, "{}") without the
// clicfg import cycle.
func seedTestFs(t *testing.T, config string) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	if config == "" {
		return fs
	}
	assert.Nil(t, fs.MkdirAll(filepath.Dir(configPath()), 0o755))
	f, err := fs.OpenFile(configPath(), os.O_WRONLY|os.O_CREATE, 0o600)
	assert.Nil(t, err)
	_, err = f.Write([]byte(config))
	assert.Nil(t, err)
	assert.Nil(t, f.Close())
	return fs
}

// readConfig reads the seeded config.json from the in-memory fs.
func readConfig(t *testing.T, fs afero.Fs) []byte {
	t.Helper()
	b, err := afero.ReadFile(fs, configPath())
	assert.Nil(t, err)
	return b
}

// addOne returns a Migration that sets/overwrites top-level key `key` to value
// `value` via sjson. Used as a deterministic, side-effect-only fixture.
func addOne(version int, description, key, value string) Migration {
	return Migration{
		Version:     version,
		Description: description,
		Apply: func(b []byte) ([]byte, error) {
			// Inline sjson.Set to keep the fixture trivial; the real engine
			// stamps _schema_version separately after Apply.
			return setJSONString(b, key, value)
		},
	}
}

// failing returns a Migration whose Apply always errors.
func failing(version int, description string, err error) Migration {
	return Migration{
		Version:     version,
		Description: description,
		Apply: func(b []byte) ([]byte, error) {
			return nil, err
		},
	}
}

// TestRunWith covers the engine's seven runtime scenarios as table-driven cases.
func TestRunWith(t *testing.T) {
	type tc struct {
		name           string
		seedConfig     string // "" => no seeded file
		migrations     []Migration
		wantMutated    bool
		wantSchemaVer  int64
		wantStderr     string // exact stderr; "" means must be empty
		wantFileExists bool
		wantKey        string // key to assert in resulting config
		wantKeyValue   string
		assertNoMutate bool // when true, verify config.json bytes equal seedConfig
	}

	cases := []tc{
		{
			name:       "fresh install — no _schema_version, applies all",
			seedConfig: `{}`,
			migrations: []Migration{
				addOne(1, "set-a", "a", "1"),
				addOne(2, "set-b", "b", "2"),
			},
			wantMutated:    true,
			wantSchemaVer:  2,
			wantStderr:     "",
			wantFileExists: true,
			wantKey:        "a",
			wantKeyValue:   "1",
		},
		{
			name:       "partial — _schema_version:3 with registry v1..v5 applies only v4+v5",
			seedConfig: `{"_schema_version":3}`,
			migrations: []Migration{
				addOne(1, "v1", "k1", "x"),
				addOne(2, "v2", "k2", "x"),
				addOne(3, "v3", "k3", "x"),
				addOne(4, "v4", "k4", "v4val"),
				addOne(5, "v5", "k5", "v5val"),
			},
			wantMutated:    true,
			wantSchemaVer:  5,
			wantStderr:     "",
			wantFileExists: true,
			wantKey:        "k4",
			wantKeyValue:   "v4val",
		},
		{
			name:       "up-to-date — _schema_version equals max applies nothing, no mutation",
			seedConfig: `{"_schema_version":2,"keep":"me"}`,
			migrations: []Migration{
				addOne(1, "v1", "should", "not-apply"),
				addOne(2, "v2", "should", "not-apply"),
			},
			wantMutated:    false,
			wantStderr:     "",
			wantFileExists: true,
			assertNoMutate: true,
		},
		{
			name:       "per-migration error — exact warning, no write, returns (false,nil)",
			seedConfig: `{"_schema_version":0,"keep":"me"}`,
			migrations: []Migration{
				addOne(1, "v1", "k1", "ok"),
				failing(2, "v2-boom", errors.New("boom")),
				addOne(3, "v3", "k3", "never"),
			},
			wantMutated:    false,
			wantStderr:     "Warning: config migration v2 (v2-boom) failed: boom; continuing with un-migrated config\n",
			wantFileExists: true,
			assertNoMutate: true,
		},
		{
			name:           "empty registry — no read, no write, returns (false,nil)",
			seedConfig:     `{"keep":"me"}`,
			migrations:     []Migration{},
			wantMutated:    false,
			wantStderr:     "",
			wantFileExists: true,
			assertNoMutate: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := seedTestFs(t, c.seedConfig)

			seedBytes := []byte(c.seedConfig)

			var stderr bytes.Buffer
			mutated, runErr := runWith(fs, configPath(), &stderr, c.migrations)
			assert.Nil(t, runErr)
			assert.Equal(t, c.wantMutated, mutated)
			assert.Equal(t, c.wantStderr, stderr.String())

			if c.wantFileExists {
				got := readConfig(t, fs)
				if c.assertNoMutate {
					assert.Equal(t, string(seedBytes), string(got), "file must be byte-identical when no migrations applied")
				} else {
					v := gjson.GetBytes(got, "_schema_version")
					assert.True(t, v.Exists(), "_schema_version must be stamped")
					assert.Equal(t, c.wantSchemaVer, v.Int())
					if c.wantKey != "" {
						assert.Equal(t, c.wantKeyValue, gjson.GetBytes(got, c.wantKey).String())
					}
				}
			}
		})
	}
}

// TestRunWith_MissingFile verifies runWith silently no-ops when config.json is
// absent. testfs seeds a config; we remove it so the read path hits
// os.ErrNotExist.
func TestRunWith_MissingFile(t *testing.T) {
	fs := seedTestFs(t, `{}`)
	assert.Nil(t, fs.Remove(configPath()))

	var stderr bytes.Buffer
	mutated, runErr := runWith(fs, configPath(), &stderr, []Migration{
		addOne(1, "v1", "k", "v"),
	})
	assert.Nil(t, runErr)
	assert.False(t, mutated)
	assert.Equal(t, "", stderr.String())

	// File must remain absent.
	exists, err := afero.Exists(fs, configPath())
	assert.Nil(t, err)
	assert.False(t, exists)
}

// TestRunWith_Idempotency: running runWith twice on the same fixture yields a
// byte-identical file the second time.
func TestRunWith_Idempotency(t *testing.T) {
	fs := seedTestFs(t, `{}`)

	ms := []Migration{
		addOne(1, "v1", "a", "1"),
		addOne(2, "v2", "b", "2"),
	}

	var stderr bytes.Buffer
	mutated, runErr := runWith(fs, configPath(), &stderr, ms)
	assert.Nil(t, runErr)
	assert.True(t, mutated)
	first := readConfig(t, fs)

	mutated2, runErr2 := runWith(fs, configPath(), &stderr, ms)
	assert.Nil(t, runErr2)
	assert.False(t, mutated2, "second run must be a no-op when _schema_version already at max")
	second := readConfig(t, fs)

	assert.Equal(t, string(first), string(second), "config must be byte-identical across runs")
}

// TestValidateMigrations covers the init() validator extracted as
// validateMigrations. We call it directly to avoid process-level panics.
func TestValidateMigrations(t *testing.T) {
	cases := []struct {
		name    string
		ms      []Migration
		wantErr bool
		errSub  string
	}{
		{
			name:    "empty slice is valid",
			ms:      []Migration{},
			wantErr: false,
		},
		{
			name: "contiguous ascending from 1 is valid",
			ms: []Migration{
				addOne(1, "v1", "a", "1"),
				addOne(2, "v2", "b", "2"),
				addOne(3, "v3", "c", "3"),
			},
			wantErr: false,
		},
		{
			name: "gap [v1,v3] is invalid",
			ms: []Migration{
				addOne(1, "v1", "a", "1"),
				addOne(3, "v3", "c", "3"),
			},
			wantErr: true,
			errSub:  "index 1 has Version=3, want 2",
		},
		{
			name: "duplicate [v1,v1] is invalid",
			ms: []Migration{
				addOne(1, "v1a", "a", "1"),
				addOne(1, "v1b", "b", "1"),
			},
			wantErr: true,
			errSub:  "index 1 has Version=1, want 2",
		},
		{
			name: "non-1 start [v2] is invalid",
			ms: []Migration{
				addOne(2, "v2", "a", "1"),
			},
			wantErr: true,
			errSub:  "index 0 has Version=2, want 1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateMigrations(c.ms)
			if c.wantErr {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), c.errSub)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// setJSONString is a tiny helper that wraps sjson.SetBytes for the test
// fixtures.
func setJSONString(data []byte, key, value string) ([]byte, error) {
	return sjson.SetBytes(data, key, value)
}
