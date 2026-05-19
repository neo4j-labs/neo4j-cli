// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package configmigrate

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/spf13/afero"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Run applies every pending forward-only migration in the package registry to
// the config.json at configPath. It returns mutated=true when at least one
// migration ran and the file was rewritten.
//
// Run is intentionally tolerant: a missing config file is a silent no-op
// (returns (false, nil)), a non-ErrNotExist read error emits a single-line
// warning to stderr and returns (false, nil), and a per-migration Apply error
// emits a warning, stops the loop, leaves the file untouched, and returns
// (false, nil). Callers do not need to check the error value; it is kept on
// the signature for future expansion.
func Run(fs afero.Fs, configPath string, stderr io.Writer) (mutated bool, err error) {
	return runWith(fs, configPath, stderr, migrations)
}

// runWith is the test seam: it accepts an explicit migrations slice so unit
// tests can drive the engine without touching the package-level registry.
func runWith(fs afero.Fs, configPath string, stderr io.Writer, ms []Migration) (bool, error) {
	if len(ms) == 0 {
		return false, nil
	}

	data, err := afero.ReadFile(fs, configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		fmt.Fprintf(stderr, "Warning: config migration could not read %s: %v; continuing with un-migrated config\n", configPath, err) //nolint:errcheck // warning to stderr; write errors are not actionable
		return false, nil
	}

	current := 0
	if r := gjson.GetBytes(data, "_schema_version"); r.Exists() && r.Type == gjson.Number {
		current = int(r.Int())
	}

	applied := false
	for _, m := range ms {
		if m.Version <= current {
			continue
		}

		next, applyErr := m.Apply(data)
		if applyErr != nil {
			fmt.Fprintf(stderr, "Warning: config migration v%d (%s) failed: %v; continuing with un-migrated config\n", m.Version, m.Description, applyErr) //nolint:errcheck // warning to stderr; write errors are not actionable
			return false, nil
		}

		stamped, setErr := sjson.SetBytes(next, "_schema_version", m.Version)
		if setErr != nil {
			fmt.Fprintf(stderr, "Warning: config migration v%d (%s) failed: %v; continuing with un-migrated config\n", m.Version, m.Description, setErr) //nolint:errcheck // warning to stderr; write errors are not actionable
			return false, nil
		}

		data = stamped
		applied = true
	}

	if !applied {
		return false, nil
	}

	fileutils.WriteFile(fs, configPath, data)
	return true, nil
}
