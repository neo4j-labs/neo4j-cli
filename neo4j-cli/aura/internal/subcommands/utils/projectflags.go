// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

const (
	ORGANIZATION_ID_FLAG = "organization-id"
	PROJECT_ID_FLAG      = "project-id"
)

// defaultOrgAndProject parses the aura.default-workspace slug ("{orgId}/{projectId}")
// and returns the org and project portions. Returns empty strings when the workspace
// is not set or does not contain a '/'.
func defaultOrgAndProject(cfg *clicfg.Config) (orgID, projectID string) {
	ctx := cfg.Aura.DefaultWorkspace()
	if ctx == "" {
		return "", ""
	}
	idx := strings.LastIndex(ctx, "/")
	if idx < 0 {
		return "", ""
	}
	return ctx[:idx], ctx[idx+1:]
}

// SetProjectFlagsAsRequired is meant to run in the PreRun of V2 commands to ensure
// that the flags are marked as required if no values have been set via aura.default-workspace.
func SetProjectFlagsAsRequired(cfg *clicfg.Config, cmd *cobra.Command) error {
	orgID, projectID := defaultOrgAndProject(cfg)

	if orgID == "" {
		if err := cmd.MarkFlagRequired(ORGANIZATION_ID_FLAG); err != nil {
			return err
		}
	}

	if projectID == "" {
		if err := cmd.MarkFlagRequired(PROJECT_ID_FLAG); err != nil {
			return err
		}
	}

	return nil
}

// SetProjetDefaults is meant to run in the RunE of V2 commands to fill in org/project
// from aura.default-workspace when the flags were not provided on the command line.
func SetProjetDefaults(cfg *clicfg.Config, organizationId string, projectId string) (string, string, error) {
	defaultOrg, defaultProject := defaultOrgAndProject(cfg)

	if organizationId == "" {
		organizationId = defaultOrg
	}
	if projectId == "" {
		projectId = defaultProject
	}
	return organizationId, projectId, nil
}
