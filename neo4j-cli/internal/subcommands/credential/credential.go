// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
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
			"and `embed` for embedding-provider credentials consumed by `query --param NAME:embed=...` and `query :embed`.",
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
	)

	const (
		nameFlag         = "name"
		clientIdFlag     = "client-id"
		clientSecretFlag = "client-secret"
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Adds an aura-client credential",
		Long: "Add an Aura Console API client credential (client ID + secret). " +
			"The first credential added becomes the default; switch later with " +
			"`credential aura-client use <name>`.",
		Example: `# Add the first aura-client credential (becomes the default)
neo4j-cli credential aura-client add --name work --client-id <id> --client-secret <secret> --rw

# Add an additional aura-client credential (default stays unchanged)
neo4j-cli credential aura-client add --name personal --client-id <id> --client-secret <secret> --rw

# Switch the default after adding a second credential
neo4j-cli credential aura-client use personal --rw`,
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Aura.Add(name, clientId, clientSecret)
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&clientId, clientIdFlag, "", "(required) Client ID")
	cmd.MarkFlagRequired(clientIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&clientSecret, clientSecretFlag, "", "(required) Client secret")
	cmd.MarkFlagRequired(clientSecretFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
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
	return &cobra.Command{
		Use:   "remove",
		Short: "Removes an aura-client credential",
		Long:  "Remove a stored Aura Console API client credential by name.",
		Example: `# Remove an aura-client credential by name
neo4j-cli credential aura-client remove work --rw

# Remove the personal credential
neo4j-cli credential aura-client remove personal --rw

# Remove a stale credential that no longer authenticates
neo4j-cli credential aura-client remove old-tenant --rw`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Aura.Remove(args[0])
		},
	}
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
