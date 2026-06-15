// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

// ExportedUserExecFn exposes the package-level userExecFn seam for use by
// the external test package (package user_test). Only compiled during tests.
var ExportedUserExecFn = &userExecFn

// NewListCmdForTest exposes the private newListCmd constructor to the
// external test package (package user_test). Only compiled during tests.
func NewListCmdForTest(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return newListCmd(cfg, conn)
}

// NewGetCmdForTest exposes the private newGetCmd constructor to the
// external test package (package user_test). Only compiled during tests.
func NewGetCmdForTest(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return newGetCmd(cfg, conn)
}
