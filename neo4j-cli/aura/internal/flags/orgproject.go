// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"github.com/spf13/cobra"
)

// OrgIDFlag is the canonical name of the organization ID flag.
const OrgIDFlag = "organization-id"

// ProjectIDFlag is the canonical name of the project ID flag.
const ProjectIDFlag = "project-id"

// TenantIDFlag is the deprecated alias for ProjectIDFlag retained for one release.
const TenantIDFlag = "tenant-id"

// RegisterOrgProjectFlags registers --organization-id and --project-id as persistent
// string flags on cmd, plus the deprecated --tenant-id alias for --project-id.
// The alias is hidden from --help and emits cobra's standard deprecation message on use.
func RegisterOrgProjectFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(OrgIDFlag, "", "ID of the Aura organization")
	cmd.PersistentFlags().String(ProjectIDFlag, "", "ID of the Aura project")
	cmd.PersistentFlags().String(TenantIDFlag, "", "")
	if err := cmd.PersistentFlags().MarkDeprecated(TenantIDFlag, "use --project-id instead"); err != nil {
		// MarkDeprecated only fails if the flag is unregistered or the message is empty;
		// both are programmer errors, panic so they surface in tests.
		panic(err)
	}
}
