// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package login

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Aura using the device authorization flow",
		Long: "Authenticate with Aura using the OAuth 2.0 Device Authorization Grant (RFC 8628).\n" +
			"On success, the access token is printed to stdout.\n\n" +
			"The following environment variables must be set before running:\n" +
			"  NEO4J_AURA_LOGIN_DEVICE_ENDPOINT  Device authorization endpoint URL\n" +
			"  NEO4J_AURA_LOGIN_TOKEN_ENDPOINT    Token endpoint URL\n" +
			"  NEO4J_AURA_LOGIN_CLIENT_ID         Public OAuth client ID\n" +
			"  NEO4J_AURA_LOGIN_AUDIENCE          OAuth audience",
		Example: `# Log in interactively; the command prints a URL to open in your browser
neo4j-cli aura login

# Source the example env file first, then log in
source .env.aura-login-spike && neo4j-cli aura login

# Capture the access token into a shell variable for use in subsequent calls
TOKEN=$(neo4j-cli aura login)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	return cmd
}
