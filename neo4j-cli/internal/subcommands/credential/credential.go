// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/envfile"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential/dbms"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential/embed"
	"github.com/spf13/cobra"
)

func NewCredentialCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage and view credential values",
		Long: "Manage stored credentials. Three subtrees are available: " +
			"`aura-client` for Aura Console API client credentials, " +
			"`dbms` for Neo4j Bolt connection profiles consumed by `query`, " +
			"and `embed` for embedding-provider credentials consumed by `query --param NAME:embed=...` and `query :embed`. " +
			"Note: `query --credential desktop` and `query --credential desktop-connection:<uuid>` are runtime-resolved against the running Neo4j Desktop 2 instance and are NOT stored here — Desktop owns those credential lifecycles. " +
			"See `neo4j-cli desktop list` to discover saved Desktop connections.",
	}

	cmd.AddCommand(NewAuraClientCredentialCmd(cfg))
	cmd.AddCommand(dbms.NewCmd(cfg))
	cmd.AddCommand(embed.NewCmd(cfg))

	return cmd
}

func NewAuraClientCredentialCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aura-client",
		Short: "Manage and view aura-client credential values",
		Long: "Manage Aura Console API client credentials (client ID + client secret). " +
			"These credentials are required by every `aura ...` subcommand that calls " +
			"the Aura Console API. The first credential added is set as default.",
	}

	cmd.AddCommand(newCredentialAddCmd(cfg))
	cmd.AddCommand(newCredentialListCmd(cfg))
	cmd.AddCommand(newCredentialRemoveCmd(cfg))
	cmd.AddCommand(newCredentialUseCmd(cfg))

	return cmd
}

func newCredentialAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name         string
		clientId     string
		clientSecret string
		envPath      string
	)

	const (
		nameFlag         = "name"
		clientIdFlag     = "client-id"
		clientSecretFlag = "client-secret"
		envFlag          = "env"

		envClientId     = "CLIENT_ID"
		envClientSecret = "CLIENT_SECRET"
		envClientName   = "CLIENT_NAME"
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Adds an aura-client credential",
		Long: "Add an Aura Console API client credential (client ID + secret). " +
			"The first credential added becomes the default; switch later with " +
			"`credential aura-client use <name>`. " +
			"Pass `--env <path>` to import an Aura console–exported aura-client credentials file " +
			"(recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME); explicit flags override file values.",
		Example: `# Add the first aura-client credential (becomes the default)
neo4j-cli credential aura-client add --name work --client-id <id> --client-secret <secret> --rw

# Import an Aura console–exported aura-client credentials file
neo4j-cli credential aura-client add --name work --env ~/Downloads/aura-client-creds.txt --rw

# Switch the default after adding a second credential
neo4j-cli credential aura-client use personal --rw`,
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Credentials.WarnIfEnvMode(cmd.ErrOrStderr())
			// Distinguish file-had-empty-value (error) from file-missing-key (fall through to flag).
			var (
				fileVals    = map[string]string{}
				filePresent = map[string]bool{}
			)
			if envPath != "" {
				vals, present, err := envfile.Parse(cfg.Aura.Fs(), envPath)
				if err != nil {
					return err
				}
				fileVals, filePresent = filterAuraClientEnvKeys(vals, present)
			}

			changed := func(flag string) bool {
				f := cmd.Flag(flag)
				return f != nil && f.Changed
			}

			if !changed(nameFlag) && filePresent[envClientName] {
				name = fileVals[envClientName]
			}
			if !changed(clientIdFlag) && filePresent[envClientId] {
				clientId = fileVals[envClientId]
			}
			if !changed(clientSecretFlag) && filePresent[envClientSecret] {
				clientSecret = fileVals[envClientSecret]
			}

			for _, c := range []struct {
				envKey   string
				flagName string
			}{
				{envClientName, nameFlag},
				{envClientId, clientIdFlag},
				{envClientSecret, clientSecretFlag},
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
				{nameFlag, name, envClientName},
				{clientIdFlag, clientId, envClientId},
				{clientSecretFlag, clientSecret, envClientSecret},
			} {
				if req.value == "" {
					return clierr.NewUsageError("--%s is required (provide via --env as %s, or pass --%s)", req.flag, req.envKey, req.flag)
				}
			}

			return cfg.Credentials.Aura.Add(name, clientId, clientSecret)
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.Flags().StringVar(&clientId, clientIdFlag, "", "(required) Client ID")
	cmd.Flags().StringVar(&clientSecret, clientSecretFlag, "", "(required) Client secret")
	cmd.Flags().StringVar(&envPath, envFlag, "", "Path to an Aura console–exported aura-client credentials file. Recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME. Explicit flags override file values.")

	return cmd
}

func filterAuraClientEnvKeys(vals map[string]string, present map[string]bool) (map[string]string, map[string]bool) {
	recognised := map[string]bool{
		"CLIENT_ID":     true,
		"CLIENT_SECRET": true,
		"CLIENT_NAME":   true,
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

func newCredentialListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List aura-client credentials",
		Long:  "List stored Aura Console API client credentials. The `default` column flags the credential used by `aura ...` commands when no other selector is set.",
		Example: `# List all aura-client credentials as a table
neo4j-cli credential aura-client list

# List as JSON for scripting / agent consumption
neo4j-cli credential aura-client list --format json

# List as toon (compact, agent-friendly)
neo4j-cli credential aura-client list --format toon`,
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Credentials.Aura.Printable(), []string{"name", "type", "identifier", "default"})
			return nil
		},
	}
}

func newCredentialRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Removes an aura-client credential",
		Long: `Remove a stored Aura Console API client credential by name.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Remove an aura-client credential by name
neo4j-cli credential aura-client remove work --rw --yes --force

# Remove the personal credential
neo4j-cli credential aura-client remove personal --rw --yes --force

# Remove a stale credential that no longer authenticates
neo4j-cli credential aura-client remove old-tenant --rw --yes --force`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := confirm.Require(cmd, args[0]); err != nil {
				return err
			}
			return cfg.Credentials.RemoveAura(args[0], cmd.ErrOrStderr())
		},
	}

	confirm.Register(cmd)

	return cmd
}

func newCredentialUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "use",
		Short: "Sets the default aura-client credential to be used",
		Long:  "Set the named aura-client credential as the default consumed by `aura ...` commands.",
		Example: `# Switch the default to the personal credential
neo4j-cli credential aura-client use personal --rw

# Switch the default to the work credential
neo4j-cli credential aura-client use work --rw

# Switch the default after adding a new credential
neo4j-cli credential aura-client use new-tenant --rw`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Aura.SetDefault(args[0])
		},
	}
}
