// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package transport_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/auraclient/transport"
)

// cursorListServer serves a two-page cursor-paginated collection and records
// the query of every request.
func newCursorListServer(t *testing.T) (*httptest.Server, *[]url.Values) {
	t.Helper()
	var queries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query())
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("page_token") == "" {
			w.Write([]byte(`{"data":[{"id":"a"}],"links":{"next":"/x?page_token=tok-2"}}`)) //nolint:errcheck
			return
		}
		w.Write([]byte(`{"data":[{"id":"b"}],"links":{"next":null}}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

func TestListAllPagesWithQuery_ExtraQueryRidesEveryPage(t *testing.T) {
	srv, queries := newCursorListServer(t)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)

	res, err := transport.ListAllPagesWithQuery(context.Background(), cfg, "/x", transport.AuraApiVersion2, 0, map[string]string{
		"agent_id": "agent-1",
		// The walker's own keys must win a collision: a caller-supplied
		// page_limit/page_token can neither shrink the walk nor inject a cursor.
		"page_limit": "1",
		"page_token": "hostile",
	})

	require.NoError(t, err)
	require.Len(t, res.Items, 2)
	require.Len(t, *queries, 2)
	for i, q := range *queries {
		assert.Equal(t, "agent-1", q.Get("agent_id"), "page %d must carry the caller filter", i+1)
		assert.Equal(t, fmt.Sprint(transport.ListPageSize), q.Get("page_limit"), "page %d", i+1)
	}
	assert.Empty(t, (*queries)[0].Get("page_token"), "a caller-supplied cursor must not start the walk")
	assert.Equal(t, "tok-2", (*queries)[1].Get("page_token"))
}

func TestListAllPagesWithQuery_LimitStopsTheWalk(t *testing.T) {
	srv, queries := newCursorListServer(t)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)

	res, err := transport.ListAllPagesWithQuery(context.Background(), cfg, "/x", transport.AuraApiVersion2, 1, map[string]string{
		"agent_id": "agent-1",
	})

	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.True(t, res.LimitReached, "one item was returned while the API had more")
	assert.False(t, res.PageCapReached)
	assert.Len(t, *queries, 1, "the walk stops once the limit is reached")
	assert.Equal(t, "agent-1", (*queries)[0].Get("agent_id"))
}
