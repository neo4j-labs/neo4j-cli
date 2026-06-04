// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
)

// roleExecFn is the package-level test seam. It is set by NewCmd in production
// and replaced by tests to inject fake results without a real Bolt connection.
var roleExecFn adminutil.ExecFn
