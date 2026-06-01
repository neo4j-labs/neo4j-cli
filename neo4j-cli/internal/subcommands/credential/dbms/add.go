// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/envfile"
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
		envPath         string
	)

	const (
		nameFlag            = "name"
		usernameFlag        = "username"
		passwordFlag        = "password"
		databaseNameFlag    = "database-name"
		uriFlag             = "uri"
		embedCredentialFlag = "embed-credential"
		envFlag             = "env"

		envURI          = "NEO4J_URI"
		envUsername     = "NEO4J_USERNAME"
		envPassword     = "NEO4J_PASSWORD"
		envDatabase     = "NEO4J_DATABASE"
		envInstanceName = "AURA_INSTANCENAME"
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Adds a dbms credential",
		Long: "Add a Neo4j Bolt connection profile. The first credential added becomes the default. " +
			"Pass `--embed-credential <name>` to link this profile to an existing embed credential — " +
			"`query --credential <name>` will then pick up the embed config automatically. The link can be added later with `credential dbms set-embed`. " +
			"Pass `--env <path>` to import a Neo4j Aura–exported credentials file (recognised keys: NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD, NEO4J_DATABASE, AURA_INSTANCENAME); explicit flags override file values. " +
			"The name `desktop` and any name starting with `desktop-connection:` are reserved by `query --credential` runtime dispatch and cannot be used.",
		Example: `# Add a dbms credential from an Aura-exported credentials file
neo4j-cli credential dbms add --env ./Neo4j-12345-Created-2025-01-01.txt --rw

# Add a dbms credential with explicit flags (becomes the default if it is the first one)
neo4j-cli credential dbms add --name local --uri neo4j://localhost:7687 --username neo4j --password secret --rw

# Add a dbms credential and link it to an existing embed credential
neo4j-cli credential dbms add --name local --uri neo4j://localhost:7687 --username neo4j --password secret --embed-credential openai-small --rw`,
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Credentials.WarnIfEnvMode(cmd.ErrOrStderr())
			// Distinguish file-had-empty-value (error) from file-missing-key (fall through to flag/default).
			var (
				fileVals    = map[string]string{}
				filePresent = map[string]bool{}
			)
			if envPath != "" {
				vals, present, err := envfile.Parse(cfg.Aura.Fs(), envPath)
				if err != nil {
					return err
				}
				fileVals, filePresent = filterAuraEnvKeys(vals, present)
			}

			changed := func(flag string) bool {
				f := cmd.Flag(flag)
				return f != nil && f.Changed
			}

			merge := func(envKey, flagName string, flagVal string) string {
				if changed(flagName) {
					return flagVal
				}
				if filePresent[envKey] {
					return fileVals[envKey]
				}
				return ""
			}

			if !changed(nameFlag) && filePresent[envInstanceName] {
				name = fileVals[envInstanceName]
			}
			username = merge(envUsername, usernameFlag, username)
			password = merge(envPassword, passwordFlag, password)
			uri = merge(envURI, uriFlag, uri)

			// database-name has a non-empty flag default ("neo4j"), so rebuild via Changed-vs-present.
			switch {
			case changed(databaseNameFlag):
			case filePresent[envDatabase]:
				databaseName = fileVals[envDatabase]
			default:
				databaseName = "neo4j"
			}

			for _, c := range []struct {
				envKey   string
				flagName string
			}{
				{envInstanceName, nameFlag},
				{envUsername, usernameFlag},
				{envPassword, passwordFlag},
				{envURI, uriFlag},
				{envDatabase, databaseNameFlag},
			} {
				if filePresent[c.envKey] && fileVals[c.envKey] == "" && !changed(c.flagName) {
					return clierr.NewUsageError("--env %q: %s has an empty value", envPath, c.envKey)
				}
			}

			for _, req := range []struct {
				flag   string
				value  string
				envKey string
			}{
				{nameFlag, name, envInstanceName},
				{usernameFlag, username, envUsername},
				{passwordFlag, password, envPassword},
				{uriFlag, uri, envURI},
			} {
				if req.value == "" {
					return clierr.NewUsageError("--%s is required (provide via --env as %s, or pass --%s)", req.flag, req.envKey, req.flag)
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
	cmd.Flags().StringVar(&envPath, envFlag, "", "Path to a Neo4j Aura–exported credentials file. Recognised keys: NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD, NEO4J_DATABASE, AURA_INSTANCENAME. Explicit flags override file values.")

	return cmd
}

func filterAuraEnvKeys(vals map[string]string, present map[string]bool) (map[string]string, map[string]bool) {
	recognised := map[string]bool{
		"NEO4J_URI":         true,
		"NEO4J_USERNAME":    true,
		"NEO4J_PASSWORD":    true,
		"NEO4J_DATABASE":    true,
		"AURA_INSTANCENAME": true,
	}
	filteredVals := map[string]string{}
	filteredPresent := map[string]bool{}
	for k := range present {
		if !recognised[k] {
			continue
		}
		filteredVals[k] = vals[k]
		filteredPresent[k] = true
	}
	return filteredVals, filteredPresent
}
