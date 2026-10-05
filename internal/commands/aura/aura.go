// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"os"

	"github.com/neo4j/cli/internal/commands/aura/graphanalytics"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/commands/aura/agent"
	apicmd "github.com/neo4j/cli/internal/commands/aura/api"
	"github.com/neo4j/cli/internal/commands/aura/customermanagedkey"
	"github.com/neo4j/cli/internal/commands/aura/graphql"
	"github.com/neo4j/cli/internal/commands/aura/instance"
	"github.com/neo4j/cli/internal/commands/aura/organization"
	"github.com/neo4j/cli/internal/commands/aura/project"
	"github.com/neo4j/cli/internal/commands/aura/virtualgraph"
	"github.com/neo4j/cli/internal/commands/aura/workspace"
	"github.com/neo4j/cli/internal/debug"
)

// NewCmd returns the aura command tree, mounted as `neo4j-cli aura`.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "aura",
		Short:   "Allows you to programmatically provision and manage your Aura resources",
		Long:    "Allows you to programmatically provision and manage your Aura resources. Write operations require --rw.",
		Version: cfg.Version,
	}

	cmd.PersistentFlags().Bool("debug", false, "Route Aura API activity (HTTP request/response wire, token acquisition, polling) to stderr; stdout is unaffected. Output may include the (best-effort-redacted) request/response bodies [env: NEO4J_DEBUG (set to 1 to enable)]")

	// Resolve --debug once at startup and carry it on cfg so the api package
	// (MakeRequest/getToken/Poll, which take *clicfg.Config not *cobra.Command)
	// can read it. cobra.EnableTraverseRunHooks (set on the neo4j-cli root) runs
	// every PersistentPreRunE up the ancestry, so this fires alongside the root
	// hook on the mounted `neo4j-cli aura ...` surface.
	prev := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		cfg.AuraRuntime.SetDebug(debug.Resolve(cmd))
		if err := applyEnvCredential(cfg); err != nil {
			return err
		}
		return nil
	}

	cmd.AddCommand(apicmd.NewCmd(cfg))
	cmd.AddCommand(workspace.NewCmd(cfg))
	cmd.AddCommand(customermanagedkey.NewCmd(cfg))
	cmd.AddCommand(instance.NewCmd(cfg))
	cmd.AddCommand(organization.NewCmd(cfg))
	cmd.AddCommand(project.NewCmd(cfg))
	cmd.AddCommand(graphanalytics.NewCmd(cfg))
	cmd.AddCommand(virtualgraph.NewCmd(cfg))
	cmd.AddCommand(agent.NewCmd(cfg))
	cmd.AddCommand(graphql.NewCmd(cfg))

	return cmd
}

// applyEnvCredential synthesizes an ephemeral Aura credential from
// NEO4J_AURA_CLIENT_ID/NEO4J_AURA_CLIENT_SECRET when accept-env-vars is enabled.
// The credential lives only in memory (cfg.AuraRuntime.SetActiveCredential) and is never
// persisted to disk or keyring. A partial pair is a usage error naming the missing
// variable. An explicit --credential flag runs afterwards (its hook is registered
// on the subcommand) and takes precedence.
func applyEnvCredential(cfg *clicfg.Config) error {
	if !cfg.Global.AcceptEnvVars() {
		return nil
	}
	if err := credentials.ValidateEnvCredentialSet(credentials.AuraEnvSpec, os.Getenv); err != nil {
		return err
	}
	clientID := os.Getenv(credentials.EnvAuraClientID)
	clientSecret := os.Getenv(credentials.EnvAuraClientSecret)
	if clientID == "" || clientSecret == "" {
		return nil
	}
	cfg.AuraRuntime.SetActiveCredential(&credentials.AuraCredential{
		Name:         "env",
		ClientId:     clientID,
		ClientSecret: clientSecret,
		Ephemeral:    true,
	})
	return nil
}
