// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func newAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name            string
		username        string
		password        string
		databaseName    string
		uri             string
		embedCredential string
	)

	const (
		nameFlag            = "name"
		usernameFlag        = "username"
		passwordFlag        = "password"
		databaseNameFlag    = "database-name"
		uriFlag             = "uri"
		embedCredentialFlag = "embed-credential"
	)

	cmd := &cobra.Command{
		Use:         "add",
		Short:       "Adds a dbms credential",
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if embedCredential != "" {
				if _, err := cfg.Credentials.Embed.Get(embedCredential); err != nil {
					return clierr.NewUsageError("invalid --embed-credential %q: no such embed credential (run `credential embed list` to see available)", embedCredential)
				}
			}
			if err := cfg.Credentials.Dbms.Add(name, username, password, databaseName, uri); err != nil {
				return err
			}
			if embedCredential != "" {
				return cfg.Credentials.Dbms.SetEmbed(name, embedCredential)
			}
			return nil
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
	cmd.Flags().StringVar(&embedCredential, embedCredentialFlag, "", "Name of an embed credential to link (must already exist; see `credential embed list`)")

	return cmd
}
