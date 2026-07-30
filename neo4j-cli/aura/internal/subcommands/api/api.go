// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package api implements `neo4j-cli aura api`, an escape hatch that issues an
// authenticated request to an arbitrary Aura API endpoint and prints the
// response, so a feature the CLI has no dedicated command for is still
// reachable.
package api

import (
	"net/http"
	"slices"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/flags"
	commonoutput "github.com/neo4j/cli/common/output"
	auraapi "github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	auraflags "github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/spf13/cobra"
)

// NewCmd builds the `aura api` command. It is mounted directly on the aura root,
// which owns only --debug, so it registers the per-group --auth-url/--base-url
// binds, the credential flag, and the org/project flags every other aura subtree
// declares on its own parent.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var reqFlags requestFlags
	var outFlags outputFlags

	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Makes an authenticated request to an arbitrary Aura API endpoint",
		Long: `Makes an authenticated request to an arbitrary Aura API endpoint and prints the response.

The endpoint is the path after the API host, including the API version segment ('v1/instances', 'v2beta1/organizations/{org_id}/projects'), so an endpoint the CLI has no dedicated command for — or an API version it does not know about — is reachable without a CLI release. A leading '/' is accepted and an inline query string is merged with any --field values.

'{org_id}' and '{project_id}' (aliases '{org}' and '{project}') are substituted from --organization-id/--project-id or the default workspace, and only when the endpoint actually uses them.

Credential resolution, the --base-url override, --debug tracing, and the exit-code contract are shared with every other Aura command. Any method other than GET or HEAD requires --rw, and DELETE additionally requires confirmation (--yes --force when non-interactive).

With --format json the response body is written byte-for-byte, so it can be piped into jq.`,
		Example: `# List the databases of an instance (Aura Multi-DB, no dedicated command yet)
neo4j-cli aura api 'v2beta1/organizations/{org_id}/projects/{project_id}/instances/00000000/databases' --format json

# List the projects of the organization in scope, substituting {org_id}
neo4j-cli aura api 'v2beta1/organizations/{org_id}/projects' --format json

# Create a database from a JSON document, inferring POST
neo4j-cli aura api 'v2beta1/organizations/{org_id}/projects/{project_id}/instances/00000000/databases' --input database.json --rw

# Delete a database
neo4j-cli aura api 'v2beta1/organizations/{org_id}/projects/{project_id}/instances/00000000/databases/db-1234' --method DELETE --rw --yes --force

# Pass query parameters and read a single field with jq (--method GET, since a field otherwise infers POST)
neo4j-cli aura api v1/instances --method GET --field tenantId=11111111 --format json | jq -r '.data[].id'`,
		Args: cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))
			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			return runRequest(cmd, cfg, args[0], &reqFlags, &outFlags)
		},
	}

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	registerRequestFlags(cmd, &reqFlags)
	registerOutputFlags(cmd, &outFlags)

	flags.RegisterAuraCredentialFlag(cmd, cfg)
	auraflags.RegisterOrgProjectFlags(cmd)
	confirm.Register(cmd)

	return cmd
}

// readMethods are the methods that read rather than write, and so need no --rw.
var readMethods = []string{http.MethodGet, http.MethodHead}

func runRequest(cmd *cobra.Command, cfg *clicfg.Config, endpoint string, reqFlags *requestFlags, outFlags *outputFlags) error {
	// The gates run before buildRequest reads any payload: a `--field key=@-`
	// would otherwise drain the stdin the confirm prompt reads its answer from.
	method, err := resolveRequestMethod(reqFlags)
	if err != nil {
		return err
	}
	if !slices.Contains(readMethods, method) {
		if err := flags.RequireWriteAccess(cmd); err != nil {
			return err
		}
	}
	// The resource type is passed explicitly: confirm.Require would derive the
	// prompt noun from the parent, which here is the aura root.
	if method == http.MethodDelete {
		if err := confirm.RequireTyped(cmd, "endpoint", endpoint); err != nil {
			return err
		}
	}

	built, err := buildRequest(cmd, cfg, reqFlags, method)
	if err != nil {
		return err
	}

	parsed, err := resolveEndpoint(cmd, cfg, endpoint)
	if err != nil {
		return err
	}

	res, err := auraapi.MakeRawRequest(cfg, &auraapi.RawRequestConfig{
		Method:      method,
		VersionPath: parsed.versionPath,
		Path:        parsed.path,
		Body:        built.body,
		QueryParams: mergeQuery(parsed.query, built.query),
		Headers:     built.headers,
		// Never cmd.OutOrStdout(): a keyring-failure warning on stdout would
		// break the single-document invariant --format json promises.
		WarnW: cmd.ErrOrStderr(),
	})
	// Checked before anything is rendered: MakeRawRequest returns BOTH a
	// response and an auth error on HTTP 401, so rendering first would print a
	// body and swallow the auth failure.
	if err != nil {
		return err
	}
	// Likewise before rendering — RawStatusError folds the upstream body into
	// the error, which clierr.Render writes to stdout as a JSON envelope, so an
	// echoed body would put two documents there.
	if err := auraapi.RawStatusError(res); err != nil {
		return err
	}

	if outFlags.include {
		printResponseMeta(cmd, res)
	}
	if outFlags.silent {
		return nil
	}
	commonoutput.PrintPassthrough(cmd, cfg, res.Body)

	return nil
}
