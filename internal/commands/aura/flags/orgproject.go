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

// RegisterOrgProjectFlags registers --organization-id and --project-id as persistent
// string flags on cmd.
func RegisterOrgProjectFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(OrgIDFlag, "", "ID of the Aura organization")
	cmd.PersistentFlags().String(ProjectIDFlag, "", "ID of the Aura project")
}
