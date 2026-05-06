// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential/database"
	"github.com/spf13/cobra"
)

func NewCredentialCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage and view credential values",
	}

	cmd.AddCommand(NewAuraClientCredentialCmd(cfg))
	cmd.AddCommand(database.NewCmd(cfg))

	return cmd
}

func NewAuraClientCredentialCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aura-client",
		Short: "Manage and view aura-client credential values",
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
		Short: "Adds a credential",
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
		Short: "List credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Credentials.Aura.Printable(), []string{"name", "type", "identifier", "default"})
			return nil
		},
	}
}

func newCredentialRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Removes a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Aura.Remove(args[0])
		},
	}
}

func newCredentialUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "use",
		Short: "Sets the default credential to be used",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Aura.SetDefault(args[0])
		},
	}
}
