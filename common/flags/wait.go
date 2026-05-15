// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"github.com/spf13/cobra"
)

// WaitFlag is the canonical name of the synchronous-wait flag for async commands.
const WaitFlag = "wait"

// AwaitFlagAlias is the deprecated alias retained for one release after CLI-87.
// Removal is tracked in CLI-111.
const AwaitFlagAlias = "await"

// RegisterWait registers --wait on cmd bound to wait, plus the deprecated --await
// alias bound to the same *bool. The alias is hidden from --help and emits cobra's
// standard "Flag --await has been deprecated, use --wait instead" message on use.
//
// Lives in common/flags (not the aura subtree) so non-aura command trees — e.g.
// the neo4j-cli docker subtree — can share the same `--wait`/`--await` semantics
// without crossing the internal-package boundary.
func RegisterWait(cmd *cobra.Command, wait *bool, helpText string) {
	cmd.Flags().BoolVar(wait, WaitFlag, false, helpText)
	cmd.Flags().BoolVar(wait, AwaitFlagAlias, false, helpText)
	if err := cmd.Flags().MarkDeprecated(AwaitFlagAlias, "use --wait instead"); err != nil {
		// MarkDeprecated only fails if the flag is unregistered or the message is empty;
		// both are programmer errors, panic so they surface in tests.
		panic(err)
	}
}
