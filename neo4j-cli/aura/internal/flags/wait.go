// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"github.com/spf13/cobra"

	commonflags "github.com/neo4j/cli/common/flags"
)

// WaitFlag is the canonical name of the synchronous-wait flag for async commands.
// Re-exported from common/flags so call sites that already imported the aura
// flags package keep building without churn.
const WaitFlag = commonflags.WaitFlag

// RegisterWait registers --wait on cmd bound to wait. Delegates to
// common/flags.RegisterWait so the docker subtree (outside aura/internal) and
// the aura subtree share one canonical implementation.
func RegisterWait(cmd *cobra.Command, wait *bool, helpText string) {
	commonflags.RegisterWait(cmd, wait, helpText)
}
