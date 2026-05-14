// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/envfile"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func NewAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name             string
		clientId         string
		clientSecret     string
		organizationId   string
		organizationName string
		envPath          string
	)

	const (
		nameFlag         = "name"
		clientIdFlag     = "client-id"
		clientSecretFlag = "client-secret"
		orgIdFlag        = "organization-id"
		orgNameFlag      = "organization-name"
		envFlag          = "env"

		envClientId     = "CLIENT_ID"
		envClientSecret = "CLIENT_SECRET"
		envClientName   = "CLIENT_NAME"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "add",
		Short:       "Adds a credential",
		Long: "Add an Aura API client credential. " +
			"Pass `--env <path>` to import an Aura console–exported aura-client credentials file " +
			"(recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME); explicit flags override file values.",
		Example: `# Add an Aura Console API credential (becomes the default if it is the first one)
neo4j-cli aura credential add --name my-creds --client-id <client-id> --client-secret <client-secret> --rw

# Import an Aura console–exported aura-client credentials file
neo4j-cli aura credential add --name my-creds --env ~/Downloads/aura-client-creds.txt --rw

# Add a credential and emit the response as JSON
neo4j-cli aura credential add --name my-creds --client-id <client-id> --client-secret <client-secret> --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse the optional --env first so we can distinguish "file had
			// the key but with empty value" (error) from "file didn't have
			// the key" (fall through to flag).
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

			// Merge file → flag for each recognised field.
			if !changed(nameFlag) && filePresent[envClientName] {
				name = fileVals[envClientName]
			}
			if !changed(clientIdFlag) && filePresent[envClientId] {
				clientId = fileVals[envClientId]
			}
			if !changed(clientSecretFlag) && filePresent[envClientSecret] {
				clientSecret = fileVals[envClientSecret]
			}

			// REQ-F-008: if any recognised file key was present with an empty
			// value AND no flag override won, it's a usage error.
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

			// REQ-F-009: surface the first missing required field.
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

			return cfg.Credentials.Aura.Add(name, clientId, clientSecret, organizationId, organizationName)
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.Flags().StringVar(&clientId, clientIdFlag, "", "(required) Client ID")
	cmd.Flags().StringVar(&clientSecret, clientSecretFlag, "", "(required) Client secret")
	cmd.Flags().StringVar(&organizationId, orgIdFlag, "", "Optional default organization ID — stored and used by 'project list' when --organization-id is not passed explicitly")
	cmd.Flags().StringVar(&organizationName, orgNameFlag, "", "Human-readable organization name (display only; not validated against the API)")
	cmd.Flags().StringVar(&envPath, envFlag, "", "Path to an Aura console–exported aura-client credentials file. Recognised keys: CLIENT_ID, CLIENT_SECRET, CLIENT_NAME. Explicit flags override file values.")

	return cmd
}

// filterAuraClientEnvKeys narrows envfile.Parse's domain-neutral maps to the
// keys the aura-client add command recognises. Unrecognised keys are silently
// discarded.
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
