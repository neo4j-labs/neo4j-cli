// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package testfs

import (
	"io"
	"os"
	"path/filepath"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
)

func GetDefaultTestFs() (afero.Fs, error) {
	return GetTestFs("{}", "{}")
}

func GetTestFs(config string, credentials string) (afero.Fs, error) {
	fs := afero.NewMemMapFs()

	if config == "" {
		return fs, nil
	}

	configPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json")
	credentialsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")

	if err := fs.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return nil, err
	}

	f, err := fs.OpenFile(configPath, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	if _, err = f.Write([]byte(config)); err != nil {
		return nil, err
	}

	credentialsFile, err := fs.OpenFile(credentialsPath, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	defer credentialsFile.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	if _, err = credentialsFile.Write([]byte(credentials)); err != nil {
		return nil, err
	}

	return fs, nil
}

func GetTestConfig(fs afero.Fs) (string, error) {
	configPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json")

	file, err := fs.Open(configPath)
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	if err := file.Close(); err != nil {
		return "", err
	}

	return string(b), nil
}
