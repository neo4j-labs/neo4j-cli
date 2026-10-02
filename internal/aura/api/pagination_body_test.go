// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/clierr"
)

func TestListAllPages_BadBodyIsAnErrorNotAPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html>gateway welcome page</html>")) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)

	var err error
	require.NotPanics(t, func() {
		_, err = api.ListAllPages(context.Background(), cfg, "/organizations/o/projects/p/virtual-graphs", api.AuraApiVersion2, 0)
	})

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "want a typed error, got %T: %v", err, err)
	assert.Equal(t, 8, ce.Code)
	assert.Contains(t, ce.Message, "unreadable list response")
}

func TestListAllPages_AcceptsAnArrayOrASingleRecord(t *testing.T) {
	for name, body := range map[string]string{
		"array":         `{"data":[{"id":"a"},{"id":"b"}],"links":{"next":null}}`,
		"single record": `{"data":{"id":"a"},"links":{"next":null}}`,
		"empty array":   `{"data":[],"links":{"next":null}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) })) //nolint:errcheck
			t.Cleanup(srv.Close)
			cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)

			res, err := api.ListAllPages(context.Background(), cfg, "/x", api.AuraApiVersion2, 0)

			require.NoError(t, err)
			switch name {
			case "array":
				assert.Len(t, res.Items, 2)
			case "single record":
				assert.Len(t, res.Items, 1)
			default:
				assert.Empty(t, res.Items)
			}
		})
	}
}
