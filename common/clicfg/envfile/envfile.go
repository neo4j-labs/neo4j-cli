// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package envfile parses dotenv-style credential files for CLI commands that
// accept a `--file` flag. The helper is intentionally domain-neutral: it
// returns the raw key/value map and a per-key presence map; key filtering
// (recognised keys, required keys) is left to the caller.
//
// The presence map exists so callers can distinguish `KEY=` (present, empty)
// from a missing key — both surface as `""` in the values map, but they have
// different semantics: an empty-in-file value is a usage error, a missing key
// falls through to flag/default.
package envfile

import (
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/subosito/gotenv"
)

// Parse reads the dotenv-style file at path via fs and returns:
//
//   - vals:    the parsed key/value map (after gotenv expansion)
//   - present: per-key presence map; true when the file contained the key,
//     regardless of whether the value was empty
//   - err:     a clierr.UsageError wrapping any open failure, in the shape
//     `--file %q: %s` so the user-facing message points at the offending path
//
// Unrecognised keys are returned as-is — filtering is the caller's job.
func Parse(fs afero.Fs, path string) (vals map[string]string, present map[string]bool, err error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, nil, clierr.NewUsageError("--file %q: %s", path, err.Error())
	}
	defer f.Close() //nolint:errcheck // read-only close error is not actionable in a defer

	// gotenv.Parse keeps `KEY=` entries with an empty value AND skips missing
	// keys entirely, so its returned map directly encodes presence: any key
	// listed in the map was in the file.
	parsed := gotenv.Parse(f)

	vals = map[string]string{}
	present = map[string]bool{}
	for k, v := range parsed {
		vals[k] = v
		present[k] = true
	}
	return vals, present, nil
}
