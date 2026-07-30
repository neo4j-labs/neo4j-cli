// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
)

const (
	// ListPageSize is the page_limit the CLI asks for when walking a
	// cursor-paginated collection. The API caps page_limit server-side, so
	// requesting the ceiling keeps the number of round trips low: a 5,000-item
	// collection is a handful of requests rather than dozens at the API's
	// smaller default.
	ListPageSize = 1000

	// MaxListPages bounds a traversal so a cursor that never terminates cannot
	// hang the CLI indefinitely. At ListPageSize this is far above any real
	// collection, so reaching it means something is wrong rather than that the
	// user genuinely has that much data.
	MaxListPages = 100
)

// PagedResult is the outcome of a ListAllPages traversal. The two stop-reason
// flags let the caller tell the user that what they are looking at is not the
// whole collection — a truncated list that looks complete is worse than a slow
// one.
type PagedResult struct {
	// Items is every element gathered across the pages that were walked.
	Items []map[string]any
	// LimitReached reports that traversal stopped because the caller's limit was
	// reached while the API still had more to give.
	LimitReached bool
	// PageCapReached reports that traversal stopped at MaxListPages rather than
	// at the end of the collection.
	PageCapReached bool
}

// ListAllPages walks a cursor-paginated v2beta1 collection from the first page
// to the last, following each response's links.next, and returns the merged
// items.
//
// Following by default is deliberate: the API caps a single response well below
// the size of a large collection, so a single request would quietly return a
// prefix. A `list` command that silently shows a prefix is indistinguishable
// from one that shows everything.
//
// A limit of zero or less means "no limit". A positive limit stops the walk
// once that many items are gathered and trims the final page to match.
func ListAllPages(cfg *clicfg.Config, path string, version AuraApiVersion, limit int) (*PagedResult, error) {
	result := &PagedResult{Items: []map[string]any{}}

	pageToken := ""
	// The cursor is opaque to the CLI, so a server-side bug could hand back a
	// token that does not advance. Tracking the tokens already followed turns
	// that into a clean error instead of an infinite loop.
	followed := map[string]struct{}{}

	for page := 0; page < MaxListPages; page++ {
		queryParams := map[string]string{"page_limit": strconv.Itoa(ListPageSize)}
		if pageToken != "" {
			queryParams["page_token"] = pageToken
		}

		resBody, statusCode, err := MakeRequest(cfg, path, &RequestConfig{
			Method:      http.MethodGet,
			Version:     version,
			QueryParams: queryParams,
		})
		if err != nil {
			return nil, err
		}
		if statusCode != http.StatusOK {
			return result, nil
		}

		result.Items = append(result.Items, ParseBody(resBody).AsArray()...)
		next := NextPageToken(resBody)

		if limit > 0 && len(result.Items) >= limit {
			// More remains if this page overshot the limit, or the API says there
			// is another page after it.
			result.LimitReached = len(result.Items) > limit || next != ""
			result.Items = result.Items[:limit]
			return result, nil
		}

		if next == "" {
			return result, nil
		}
		if _, repeat := followed[next]; repeat {
			return nil, clierr.NewUpstreamError("pagination stopped: the API returned a page cursor it had already returned, so following it would loop")
		}
		followed[next] = struct{}{}
		pageToken = next
	}

	result.PageCapReached = true
	return result, nil
}

// NextPageToken extracts the page_token query parameter from a list response's
// links.next. The API returns links.next either absolute or relative, and
// url.Parse handles both. Returns "" when the body carries no next link (the
// last page reports it as null), or the link carries no page_token.
func NextPageToken(body []byte) string {
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
