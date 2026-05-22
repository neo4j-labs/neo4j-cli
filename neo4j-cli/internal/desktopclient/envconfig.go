// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

var userConfigDirFn = os.UserConfigDir

func SetUserConfigDirFnForTest(fn func() (string, error)) func() {
	prev := userConfigDirFn
	userConfigDirFn = fn
	return func() { userConfigDirFn = prev }
}

// EnvConfigDir returns the directory holding relate env JSONs. Desktop-2's
// electron override intentionally omits the `config` field so env JSONs fall
// back to the deprecated `env-paths.ts` locations:
//
//   - macOS:   <userConfigDir>/com.Neo4j.Relate/Config/environments
//   - Windows: <userConfigDir>/Neo4j/Relate/Config/environments
//   - Linux:   <userConfigDir>/neo4j-relate/environments
func EnvConfigDir() (string, error) {
	base, err := userConfigDirFn()
	if err != nil {
		return "", err
	}
	switch goosFn() {
	case "darwin":
		return filepath.Join(base, "com.Neo4j.Relate", "Config", "environments"), nil
	case "windows":
		return filepath.Join(base, "Neo4j", "Relate", "Config", "environments"), nil
	default:
		// linux + everything else
		return filepath.Join(base, "neo4j-relate", "environments"), nil
	}
}

// LoadEnvs reads every `*.json` file under EnvConfigDir() and returns them
// in directory-listing order. Files that fail to parse or have unexpected
// shapes are skipped silently; a corrupt env metadata file should never
// take the CLI down. Returns an empty slice when the directory does not
// exist — Desktop may not be installed at all.
func LoadEnvs(fs afero.Fs) ([]EnvJSON, error) {
	dir, err := EnvConfigDir()
	if err != nil {
		return nil, err
	}

	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []EnvJSON{}, nil
		}
		return nil, err
	}

	envs := make([]EnvJSON, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		full := filepath.Join(dir, name)
		env, ok := loadEnvFile(fs, full)
		if !ok {
			continue
		}
		envs = append(envs, env)
	}

	return envs, nil
}

// ActiveEnv returns the env with `active: true`, or the env whose `name`
// matches `nameOverride` when non-empty. Returns `nil` when no env matches —
// even when `nameOverride` is set but doesn't match, so the CLI doesn't crash
// on a misspelling.
func ActiveEnv(envs []EnvJSON, nameOverride string) *EnvJSON {
	if nameOverride != "" {
		for i := range envs {
			if envs[i].Name == nameOverride {
				return &envs[i]
			}
		}
		return nil
	}
	for i := range envs {
		if envs[i].Active {
			return &envs[i]
		}
	}
	return nil
}

func loadEnvFile(fs afero.Fs, path string) (EnvJSON, bool) {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return EnvJSON{}, false
	}
	var env EnvJSON
	if err := json.Unmarshal(data, &env); err != nil {
		return EnvJSON{}, false
	}
	env.Path = path
	return env, true
}
