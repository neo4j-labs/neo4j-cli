// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

// sessionListServer serves the offset-paginated sessions endpoint: page 1 is
// a full page (transport.ListPageSize items) and page 2 holds tailItems (or
// none), so the walker must follow a full page and stop on a short or empty
// one. It records the page/page_size query of every request.
type sessionListServer struct {
	srv       *httptest.Server
	pagesSeen []string
}

func newSessionListServer(t *testing.T, tailItems int) *sessionListServer {
	t.Helper()
	s := &sessionListServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v2beta1/organizations/org-1/projects/proj-1/graph-analytics/sessions", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		assert.Equal(t, strconv.Itoa(transport.ListPageSize), r.URL.Query().Get("page_size"))
		s.pagesSeen = append(s.pagesSeen, page)

		var items []string
		switch page {
		case "1":
			for i := 0; i < transport.ListPageSize; i++ {
				items = append(items, fmt.Sprintf(`{"id":"sess-%04d","name":"s%d","status":"ready"}`, i, i))
			}
		case "2":
			for i := 0; i < tailItems; i++ {
				items = append(items, fmt.Sprintf(`{"id":"sess-tail-%d","name":"t%d","status":"ready"}`, i, i))
			}
		default:
			t.Errorf("unexpected page requested: %q", page)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[` + strings.Join(items, ",") + `]}`)) //nolint:errcheck
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sessionListServer) client(t *testing.T) Client {
	t.Helper()
	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, s.srv.URL, s.srv.URL)
	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)
	return New(clicfg.NewConfig(fs, "test"))
}

func TestSessionsList_WalksAllOffsetPages(t *testing.T) {
	t.Run("merges a full first page and a short last page, in order", func(t *testing.T) {
		s := newSessionListServer(t, 2)
		c := s.client(t)

		sessions, err := c.Sessions().List(context.Background(), Scope{OrgID: "org-1", ProjectID: "proj-1"}, "")

		require.NoError(t, err)
		require.Len(t, sessions, transport.ListPageSize+2)
		assert.Equal(t, "sess-0000", sessions[0].ID)
		assert.Equal(t, fmt.Sprintf("sess-%04d", transport.ListPageSize-1), sessions[transport.ListPageSize-1].ID)
		assert.Equal(t, "sess-tail-0", sessions[transport.ListPageSize].ID)
		assert.Equal(t, "sess-tail-1", sessions[transport.ListPageSize+1].ID)
		assert.Equal(t, []string{"1", "2"}, s.pagesSeen, "a short last page terminates the walk")
	})

	t.Run("an empty second page terminates the walk", func(t *testing.T) {
		s := newSessionListServer(t, 0)
		c := s.client(t)

		sessions, err := c.Sessions().List(context.Background(), Scope{OrgID: "org-1", ProjectID: "proj-1"}, "")

		require.NoError(t, err)
		assert.Len(t, sessions, transport.ListPageSize)
		assert.Equal(t, []string{"1", "2"}, s.pagesSeen)
	})
}
