// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFlagSetForTest returns a FlagSet whose viper instance is hermetic
// (no env binding, no config file) so each precedence layer can be exercised
// in isolation. Defaults are intentionally NOT pre-seeded here — viper.IsSet
// returns true only for values explicitly set by Set / env / config / flag,
// which is the semantic Enabled relies on for layer 2 and layer 3.
func newFlagSetForTest(t *testing.T) (*FlagSet, *viper.Viper) {
	t.Helper()
	v := viper.New()
	return &FlagSet{viper: v}, v
}

// registerTestFlag seeds a sentinel flag into Registry for the duration of
// the test and restores the original Registry on cleanup. All test cases that
// require a registered key use "flag.test-sentinel" / "test.legacy-key".
func registerTestFlag(t *testing.T) {
	t.Helper()
	const testKey = "flag.test-sentinel"
	const legacyKey = "test.legacy-key"
	orig := Registry
	Registry = map[string]Flag{
		testKey: {
			Name:             testKey,
			Default:          false,
			Owner:            "test",
			Gates:            "test gate",
			IntroducedIn:     "0.0.0",
			RemovalCondition: "when tests are removed",
			LegacyKey:        legacyKey,
		},
	}
	t.Cleanup(func() { Registry = orig })
}

func TestFlagSet_Enabled_Precedence(t *testing.T) {
	registerTestFlag(t)

	const key = "flag.test-sentinel"
	const legacyKey = "test.legacy-key"

	tests := []struct {
		name  string
		setup func(t *testing.T, fs *FlagSet, v *viper.Viper)
		key   string
		want  bool
	}{
		{
			name:  "default (nothing set) returns spec default false",
			setup: func(t *testing.T, fs *FlagSet, v *viper.Viper) {},
			key:   key,
			want:  false,
		},
		{
			name: "legacy alias only — returns legacy value and is one-shot logged",
			setup: func(t *testing.T, fs *FlagSet, v *viper.Viper) {
				v.Set(legacyKey, true)
			},
			key:  key,
			want: true,
		},
		{
			name: "config file (primary key) takes precedence over legacy alias",
			setup: func(t *testing.T, fs *FlagSet, v *viper.Viper) {
				v.Set(legacyKey, false)
				v.Set(key, true)
			},
			key:  key,
			want: true,
		},
		{
			name: "env-bound IsSet (primary) wins over legacy",
			setup: func(t *testing.T, fs *FlagSet, v *viper.Viper) {
				// Bind a process env var by setting via viper directly —
				// IsSet for that key now returns true.
				v.Set(key, true)
			},
			key:  key,
			want: true,
		},
		{
			name: "SetForTest override beats everything",
			setup: func(t *testing.T, fs *FlagSet, v *viper.Viper) {
				v.Set(key, false)
				v.Set(legacyKey, false)
				fs.SetForTest(key, true)
			},
			key:  key,
			want: true,
		},
		{
			name:  "unknown name returns false",
			setup: func(t *testing.T, fs *FlagSet, v *viper.Viper) {},
			key:   "flag.does-not-exist",
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, v := newFlagSetForTest(t)
			tc.setup(t, fs, v)
			assert.Equal(t, tc.want, fs.Enabled(tc.key))
		})
	}
}

// captureDebugLogs swaps slog.Default() for a handler that writes Debug+
// records into the returned buffer. The previous default is restored on
// t.Cleanup.
func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestFlagSet_LegacyDebugLogIsOneShot(t *testing.T) {
	registerTestFlag(t)
	buf := captureDebugLogs(t)

	fs, v := newFlagSetForTest(t)
	v.Set("test.legacy-key", true)

	// Call Enabled multiple times — the deprecated-key debug log must fire
	// exactly once for this FlagSet instance.
	for i := 0; i < 5; i++ {
		assert.True(t, fs.Enabled("flag.test-sentinel"))
	}

	occurrences := bytes.Count(buf.Bytes(), []byte("feature flag read from deprecated key"))
	assert.Equal(t, 1, occurrences, "deprecated-key debug log should fire exactly once per legacy key per FlagSet")
}

func TestFlagSet_Enabled_UnknownKeyDebugLog(t *testing.T) {
	buf := captureDebugLogs(t)

	fs, _ := newFlagSetForTest(t)
	assert.False(t, fs.Enabled("flag.totally-unknown"))

	assert.Contains(t, buf.String(), "feature-flag lookup for unregistered key")
	assert.Contains(t, buf.String(), "flag.totally-unknown")

	// Sanity: handler runs at debug level.
	ctx := context.Background()
	require.True(t, slog.Default().Enabled(ctx, slog.LevelDebug))
}

func TestFlagSet_SetForTest_PanicsOnUnregisteredKey(t *testing.T) {
	registerTestFlag(t)
	fs, _ := newFlagSetForTest(t)

	assert.NotPanics(t, func() {
		fs.SetForTest("flag.test-sentinel", true)
	})

	assert.PanicsWithValue(t,
		`clicfg.SetForTest: "flag.does-not-exist" is not in Registry — the override would be ignored by Enabled`,
		func() { fs.SetForTest("flag.does-not-exist", true) },
	)
}

func TestFlagNameToEnv(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "docker-command", in: "flag.docker-command", want: "NEO4J_CLI_FLAG_DOCKER_COMMAND"},
		{name: "secrets-os-keystore", in: "flag.secrets-os-keystore", want: "NEO4J_CLI_FLAG_SECRETS_OS_KEYSTORE"},
		{name: "multi-word-feature", in: "flag.multi-word-feature", want: "NEO4J_CLI_FLAG_MULTI_WORD_FEATURE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, FlagNameToEnv(tc.in))
		})
	}
}

func TestFlagSet_SetFromConfigCmd(t *testing.T) {
	registerTestFlag(t)

	const testKey = "flag.test-sentinel"

	tests := []struct {
		name        string
		key         string
		value       string
		wantErr     string
		wantExit    int
		wantWritten string
	}{
		{
			name:        "accepted true writes JSON value",
			key:         testKey,
			value:       "true",
			wantWritten: `{"flag.test-sentinel":true}`,
		},
		{
			name:        "accepted false writes JSON value",
			key:         testKey,
			value:       "false",
			wantWritten: `{"flag.test-sentinel":false}`,
		},
		{
			name:     "unknown key rejected as usage error",
			key:      "flag.does-not-exist",
			value:    "true",
			wantErr:  `invalid config key: "flag.does-not-exist"`,
			wantExit: 2,
		},
		{
			name:     "invalid value rejected as usage error",
			key:      testKey,
			value:    "maybe",
			wantErr:  `invalid value for "flag.test-sentinel": maybe (valid values: true, false)`,
			wantExit: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			memFs := afero.NewMemMapFs()
			configPath := filepath.Join("config", "config.json")
			require.NoError(t, afero.WriteFile(memFs, configPath, []byte(`{}`), 0o600))

			fs := &FlagSet{
				fs:         memFs,
				configPath: configPath,
			}

			err := fs.SetFromConfigCmd(tc.key, tc.value)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				var cliErr *clierr.CLIError
				require.ErrorAs(t, err, &cliErr)
				assert.Equal(t, tc.wantExit, cliErr.Code)
				return
			}

			require.NoError(t, err)
			got, err := afero.ReadFile(memFs, configPath)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantWritten, string(got))
		})
	}
}
