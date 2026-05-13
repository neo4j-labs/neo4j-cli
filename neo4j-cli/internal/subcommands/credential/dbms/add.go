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
		Use:   "add",
		Short: "Adds a dbms credential",
		Long: "Add a Neo4j Bolt connection profile. The first credential added becomes the default. " +
			"Pass `--embed-credential <name>` to link this profile to an existing embed credential — " +
			"`query --credential <name>` will then pick up the embed config automatically. The link can be added later with `credential dbms set-embed`.",
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, req := range []struct {
				flag   string
				value  string
				envKey string
			}{
				{nameFlag, name, "AURA_INSTANCENAME"},
				{usernameFlag, username, "NEO4J_USERNAME"},
				{passwordFlag, password, "NEO4J_PASSWORD"},
				{uriFlag, uri, "NEO4J_URI"},
			} {
				if req.value == "" {
					return clierr.NewUsageError("--%s is required (provide via --file as %s, or pass --%s)", req.flag, req.envKey, req.flag)
				}
			}
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
	cmd.Flags().StringVar(&username, usernameFlag, "", "(required) Username")
	cmd.Flags().StringVar(&password, passwordFlag, "", "(required) Password")
	cmd.Flags().StringVar(&uri, uriFlag, "", "(required) URI")
	cmd.Flags().StringVar(&databaseName, databaseNameFlag, "neo4j", "Database name")
	cmd.Flags().StringVar(&embedCredential, embedCredentialFlag, "", "Name of an embed credential to link (must already exist; see `credential embed list`)")

	return cmd
}
