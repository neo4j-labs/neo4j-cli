// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/neo4j/cli/common/skill/catalog"
)

// ErrNotSelfSkill signals that a `<skill-name>` positional did not resolve
// to the embedded self-skill. Callers (install/remove/print leaves) should
// fall through to `catalog.Lookup` after seeing this sentinel — the
// resolver intentionally never touches the catalog itself so the two
// surfaces stay testable in isolation.
var ErrNotSelfSkill = errors.New("skill: not the self-skill")

// ResolveSelf maps a positional skill name to the embedded self-skill
// Source. Both `self` (canonical, REQ-F-020) and `binaryName` (back-compat
// alias, REQ-F-021) resolve to the same Source. Any other name returns
// ErrNotSelfSkill so the caller can consult the curated catalog.
//
// The reserved-name set is owned by `catalog.IsReserved` — the catalog
// rejects upstream entries matching either name (fail-closed collision
// guard, task-003), and this resolver honours the same set so both
// surfaces share one source of truth.
func ResolveSelf(bundle fs.FS, version, binaryName, name string) (Source, error) {
	if bundle == nil {
		return Source{}, errors.New("skill: nil bundle FS")
	}
	if !catalog.IsReserved(name, binaryName) {
		return Source{}, fmt.Errorf("%w: %q", ErrNotSelfSkill, name)
	}
	return Source{FS: bundle, Version: version}, nil
}
