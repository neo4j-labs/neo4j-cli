// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
)

// privilegeExecFn is the package-level test seam. It is set by NewCmd in
// production and replaced by tests to inject fake results without a real Bolt
// connection.
var privilegeExecFn adminutil.ExecFn
