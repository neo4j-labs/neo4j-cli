// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package depflag is the shared --deployment-id flag of the deployment
// sub-resource groups (token, database, server).
package depflag

import "github.com/spf13/cobra"

const Name = "deployment-id"

// Register adds a required persistent --deployment-id flag to a resource group.
func Register(cmd *cobra.Command) {
	cmd.PersistentFlags().String(Name, "", "(required) Fleet Manager deployment ID")
	cmd.MarkPersistentFlagRequired(Name) //nolint:errcheck // MarkPersistentFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
}

// Get returns the --deployment-id value of a leaf under a group that called Register.
func Get(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString(Name)
	return v
}
