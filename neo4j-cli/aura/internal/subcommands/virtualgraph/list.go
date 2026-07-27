// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		pageLimit int
		pageToken string
	)

	const (
		pageLimitFlag = "page-limit"
		pageTokenFlag = "page-token"
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of virtual graphs",
		Long: `This subcommand returns a list containing a summary of each virtual graph in the specified project, newest first. To find out more about a specific virtual graph, retrieve the details using the get subcommand.

Results are cursor-paginated. When more results are available the next page's cursor is printed to stderr; pass it back with --page-token to fetch the next page.

Use --organization-id and --project-id to specify which project's virtual graphs to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List virtual graphs in a project (using flags)
neo4j-cli aura virtual-graph list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List virtual graphs using a configured default workspace
neo4j-cli aura virtual-graph list

# Fetch a bounded page and emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura virtual-graph list --page-limit 10 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			queryParams := map[string]string{}
			if pageLimit > 0 {
				queryParams["page_limit"] = strconv.Itoa(pageLimit)
			}
			if pageToken != "" {
				queryParams["page_token"] = pageToken
			}
			if len(queryParams) == 0 {
				queryParams = nil
			}

			path := api.ScopedVirtualGraphsPath(orgID, projectID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:      http.MethodGet,
				Version:     api.AuraApiVersion2,
				QueryParams: queryParams,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, summaryFields)

				// links.next is dropped by the {"data": ...} envelope the CLI renders,
				// so surface the cursor as stderr narration rather than silently
				// truncating the result set at one page.
				if next := nextPageToken(resBody); next != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "More results available. Re-run with --page-token %s\n", next) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&pageLimit, pageLimitFlag, 0, "The maximum number of virtual graphs to return in one page. Omit to use the API default.")
	cmd.Flags().StringVar(&pageToken, pageTokenFlag, "", "The cursor printed by a previous page's next-page hint. Omit to fetch the first page.")

	return cmd
}
