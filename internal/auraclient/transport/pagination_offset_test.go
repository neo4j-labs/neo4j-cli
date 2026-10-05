// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package transport_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/auraclient/transport"
)

// offsetListServer serves a two-page offset-paginated collection (a full
// first page, a short second page) and records the page/page_size query of
// every request, plus any instance filter passed through.
type offsetListServer struct {
	pagesSeen     []string
	pageSizesSeen []string
	filtersSeen   []string
}

func newOffsetListServer(t *testing.T) (*httptest.Server, *offsetListServer) {
	t.Helper()
	s := &offsetListServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		s.pagesSeen = append(s.pagesSeen, page)
		s.pageSizesSeen = append(s.pageSizesSeen, r.URL.Query().Get("page_size"))
		s.filtersSeen = append(s.filtersSeen, r.URL.Query().Get("instanceId"))

		var items []string
		switch page {
		case "1":
			for i := 0; i < transport.ListPageSize; i++ {
				items = append(items, fmt.Sprintf(`{"id":"item-%04d"}`, i))
			}
		case "2":
			items = append(items, `{"id":"item-tail"}`)
		default:
			t.Errorf("unexpected page requested: %q", page)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[` + strings.Join(items, ",") + `]}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv, s
}

func TestListAllOffsetPages_WalkerKeysWinACollision(t *testing.T) {
	srv, s := newOffsetListServer(t)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)

	res, err := transport.ListAllOffsetPages(context.Background(), cfg, "/x", transport.AuraApiVersion2, map[string]string{
		"instanceId": "inst-1",
		// A caller-supplied page/page_size must not pin or shrink the walk.
		"page":      "1",
		"page_size": "1",
	})

	require.NoError(t, err)
	assert.False(t, res.PageCapReached)
	assert.Len(t, res.Items, transport.ListPageSize+1)
	assert.Equal(t, []string{"1", "2"}, s.pagesSeen, "the walker's advancing page keys override a colliding caller key")
	wantPageSize := fmt.Sprint(transport.ListPageSize)
	for _, got := range s.pageSizesSeen {
		assert.Equal(t, wantPageSize, got)
	}
	assert.Equal(t, []string{"inst-1", "inst-1"}, s.filtersSeen, "non-colliding caller filters still pass through on every page")
}
