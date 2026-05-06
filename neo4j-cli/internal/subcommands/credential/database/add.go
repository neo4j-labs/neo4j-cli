// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name         string
		username     string
		password     string
		databaseName string
		uri          string
		insecure     bool
	)

	const (
		nameFlag         = "name"
		usernameFlag     = "username"
		passwordFlag     = "password"
		databaseNameFlag = "database-name"
		uriFlag          = "uri"
		insecureFlag     = "insecure"
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Adds a database credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Database.Add(name, username, password, databaseName, uri, insecure)
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&username, usernameFlag, "", "(required) Username")
	cmd.MarkFlagRequired(usernameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&password, passwordFlag, "", "(required) Password")
	cmd.MarkFlagRequired(passwordFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&uri, uriFlag, "", "(required) URI")
	cmd.MarkFlagRequired(uriFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&databaseName, databaseNameFlag, "neo4j", "Database name")
	cmd.Flags().BoolVar(&insecure, insecureFlag, false, "Disable TLS verification")

	return cmd
}
