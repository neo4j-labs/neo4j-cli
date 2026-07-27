// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"encoding/json"
	"net/url"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

// resourceName is the singular resource label used in validation errors. It
// matches the singular form api.parseResourceFromRequest derives from the
// `virtual-graphs` path segment, so CLI-side and API-side 404s agree.
const resourceName = "virtual-graph"

// detailFields is the single-resource column projection shared by create, get
// and update. It mirrors the public API's VirtualGraph schema field order,
// leading with the fields a user identifies an instance by.
var detailFields = []string{
	"id",
	"name",
	"status",
	"cloud_provider",
	"region",
	"memory",
	"bolt_url",
	"data_source_id",
	"data_source_type",
	"error_detail",
	"created_at",
}

// summaryFields is the list-view column projection: the identifying and
// lifecycle fields, without the per-instance connection and data-source detail
// that would make a multi-row table unreadable.
var summaryFields = []string{
	"id",
	"name",
	"status",
	"cloud_provider",
	"region",
	"memory",
	"created_at",
}

// detailFieldsFor returns detailFields for a single virtual graph, appending
// maximum_bytes_billed only when the API returned it (the field is BigQuery-only
// and omitted for other data-source types, so an unconditional column would
// render empty for most instances). Any extra fields are appended last.
func detailFieldsFor(virtualGraph map[string]any, extra ...string) []string {
	fields := append([]string{}, detailFields...)
	if _, ok := virtualGraph["maximum_bytes_billed"]; ok {
		fields = append(fields, "maximum_bytes_billed")
	}
	return append(fields, extra...)
}

// printVirtualGraph renders a single-resource response body with the shared
// detail projection, widened by extra.
func printVirtualGraph(cmd *cobra.Command, cfg *clicfg.Config, resBody []byte, extra ...string) error {
	responseData := api.ParseBody(resBody)
	virtualGraph, err := responseData.GetSingleOrError()
	if err != nil {
		return err
	}
	output.PrintBodyMap(cmd, cfg, responseData, detailFieldsFor(virtualGraph, extra...))
	return nil
}

// nextPageToken extracts the page_token query parameter from a list response's
// links.next URL. The public API returns links.next as an absolute URL (null on
// the last page); the CLI surfaces just the cursor so the value can be fed
// straight back in via --page-token. Returns "" when the body carries no next
// link, or the link is not a URL carrying a page_token.
func nextPageToken(body []byte) string {
	var envelope struct {
		Links struct {
			Next *string `json:"next"`
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Links.Next == nil {
		return ""
	}
	next, err := url.Parse(*envelope.Links.Next)
	if err != nil {
		return ""
	}
	return next.Query().Get("page_token")
}
