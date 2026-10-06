// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection

import (
	"github.com/google/uuid"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newUpdateCmd builds the `desktop connection update <id>` leaf. Positional
// `<id>` is UUID-only (no get-by-id route — find via `desktop list`). At
// least one mutating flag must be supplied. PATCH body contains ONLY the keys
// the caller actually set so empty-string remains distinct from "not provided".
// `--password ""` triggers the same no-echo TTY prompt `create` uses.
func newUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name        string
		uri         string
		username    string
		password    string
		description string
		projects    []string
		tags        []string
	)

	const (
		nameFlag        = "name"
		uriFlag         = "uri"
		usernameFlag    = "username"
		passwordFlag    = "password"
		descriptionFlag = "description"
		projectFlag     = "project"
		tagsFlag        = "tags"
	)

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a saved remote DB connection registered with Neo4j Desktop 2",
		Long: "Update a saved remote DB connection profile by id. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"At least one of `--name --uri --username --password --description --project --tags` must be supplied; the PATCH body contains ONLY the keys you set, so empty-string is a legitimate update for `--description`. " +
			"`--password` with an empty value prompts interactively (no echo) on a TTY and fails with a usage error on a non-TTY, mirroring `desktop connection create`. " +
			"`--project`/`--tags` have REPLACE semantics: each element is a project/tag name or UUID resolved against the Desktop catalog, and the resolved arrays become the connection's entire project/tag assignments (pass an empty value to clear them). " +
			"Find connection ids with `neo4j-cli desktop list`.",
		Example: `# Rename a saved connection
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --name aura-prod-renamed --rw

# Rotate the password and update the URI in one PATCH
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --uri neo4j+s://new-host.databases.neo4j.io --password new-secret --rw

# Clear the description by sending an empty string and emit the updated Connection as JSON
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --description "" --format json --rw

# Reassign the connection to a project and replace its tags (names or UUIDs)
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --project Prod --tags backend,oncall --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			id := args[0]
			if _, err := uuid.Parse(id); err != nil {
				return clierr.NewUsageError(
					"connection id must be a UUID; got %q. "+
						"Run 'neo4j-cli desktop list' to see connection ids.", id)
			}

			// Without this gate we'd send an empty PATCH body which relate
			// accepts silently — surfaces as a confusing no-op.
			nameSet := cmd.Flag(nameFlag).Changed
			uriSet := cmd.Flag(uriFlag).Changed
			usernameSet := cmd.Flag(usernameFlag).Changed
			passwordSet := cmd.Flag(passwordFlag).Changed
			descriptionSet := cmd.Flag(descriptionFlag).Changed
			projectSet := cmd.Flag(projectFlag).Changed
			tagsSet := cmd.Flag(tagsFlag).Changed
			if !nameSet && !uriSet && !usernameSet && !passwordSet && !descriptionSet && !projectSet && !tagsSet {
				return clierr.NewUsageError(
					"at least one of --name, --uri, --username, --password, --description, --project, --tags must be supplied")
			}

			ctx := cmd.Context()
			fs := cfg.Fs()
			port, _ := cmd.Flags().GetInt("port")

			// `--password ""` is the "prompt me" form — relate does not accept
			// an empty password, so there is no path to set one. Prompt BEFORE
			// the client is built so non-TTY callers fail fast.
			if passwordSet && password == "" {
				pw, err := promptPassword(cmd)
				if err != nil {
					return err
				}
				password = pw
			}

			client, err := desktopclient.Connect(ctx, fs, port)
			if err != nil {
				return err
			}

			if passwordSet && password != "" {
				clievents.RegisterSecretValue(password)
			}

			updateArgs := desktopclient.ConnectionUpdateArgs{}
			if nameSet {
				updateArgs.Name = &name
			}
			if uriSet {
				updateArgs.ConnectionURI = &uri
			}
			if usernameSet {
				updateArgs.Username = &username
			}
			if passwordSet {
				updateArgs.Password = &password
			}
			if descriptionSet {
				updateArgs.Description = &description
			}
			if projectSet {
				// Replace semantics: the resolved IDs become the connection's
				// ENTIRE project assignment. Resolution errors are already
				// usage errors — propagate as-is.
				resolved, err := client.ResolveProjectIDs(ctx, desktopclient.NonEmptyStrings(projects))
				if err != nil {
					return err
				}
				updateArgs.Projects = &resolved
			}
			if tagsSet {
				resolved, err := client.ResolveTagIDs(ctx, desktopclient.NonEmptyStrings(tags))
				if err != nil {
					return err
				}
				updateArgs.Tags = &resolved
			}

			updated, err := client.UpdateConnection(ctx, id, updateArgs)
			if err != nil {
				return err
			}

			output.PrintBodyMap(cmd, cfg, connectionResult{Item: updated}, connectionCreateFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "New human-readable name for the saved connection")
	cmd.Flags().StringVar(&uri, uriFlag, "", "New Bolt URI for the remote DB (e.g. neo4j+s://abc.databases.neo4j.io)")
	cmd.Flags().StringVar(&username, usernameFlag, "", "New username used to authenticate against the remote DB")
	cmd.Flags().StringVar(&password, passwordFlag, "", "New password for the remote DB. Pass an empty value on a TTY to be prompted (no echo); fails on non-TTY")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "New description for the saved connection. Pass an empty string to clear the existing description")
	cmd.Flags().StringSliceVar(&projects, projectFlag, nil, "Replace the connection's project assignments; each element is a project name or UUID (repeatable/comma-separated). Pass an empty value to clear all project assignments")
	cmd.Flags().StringSliceVar(&tags, tagsFlag, nil, "Replace the connection's tag assignments; each element is a tag name or UUID (repeatable/comma-separated). Pass an empty value to clear all tag assignments")

	return cmd
}
