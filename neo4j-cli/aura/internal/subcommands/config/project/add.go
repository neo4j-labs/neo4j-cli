// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name           string
		organizationId string
		projectId      string
	)

	const (
		nameFlag           = "name"
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Adds a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Aura.Projects.Add(name, organizationId, projectId)
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Oragnization ID")
	cmd.MarkFlagRequired(organizationIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project ID")
	cmd.MarkFlagRequired(projectIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
