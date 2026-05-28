// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"github.com/spf13/cobra"
)

// WaitFlag is the canonical name of the synchronous-wait flag for async commands.
const WaitFlag = "wait"

// RegisterWait registers --wait on cmd bound to wait.
//
// Lives in common/flags (not the aura subtree) so non-aura command trees — e.g.
// the neo4j-cli docker subtree — can share the same `--wait` semantics without
// crossing the internal-package boundary.
func RegisterWait(cmd *cobra.Command, wait *bool, helpText string) {
	cmd.Flags().BoolVar(wait, WaitFlag, false, helpText)
}
