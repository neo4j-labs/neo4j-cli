// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package transport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clierr"
)

// hangingServer answers the token endpoint normally and blocks every other
// request until the client goes away, so a cancelled context is the only way
// a request can finish.
func hangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMakeRequest_AbandonsABlockedRequestWhenContextIsCancelled(t *testing.T) {
	srv := hangingServer(t)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	_, _, err := transport.MakeRequest(ctx, cfg, "/instances", &transport.RequestConfig{Method: http.MethodGet})

	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "cancellation must not wait out the 60s client timeout")
	assert.ErrorIs(t, err, context.Canceled, "the cancellation stays discoverable through the error chain")
}

func TestPoll_IsInterruptedDuringTheIntervalSleep(t *testing.T) {
	srv := hangingServer(t)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)
	cfg.AuraRuntime.SetPollingConfig(5, 3600) // an hour between attempts

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	_, err := transport.PollInstance(ctx, cfg, "org-1", "proj-1", "inst-1", "creating")

	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 5*time.Second, "an hour-long poll interval must not delay cancellation")
}

func TestPoll_AlreadyCancelledContextMakesNoRequest(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Write([]byte(`{"data":{"id":"x","status":"ready"}}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)
	cfg.AuraRuntime.SetPollingConfig(3, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transport.PollInstance(ctx, cfg, "org-1", "proj-1", "inst-1", "creating")

	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, hits)
}

// An unreachable authentication endpoint used to panic out of the token mint
// (and, via the top-level recover, exit 0). It is now an ordinary upstream
// error.
func TestMakeRequest_UnreachableAuthServerIsAnErrorNotAPanic(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	cfg := buildTestConfig(t, srv.URL, noTokenCredJSON)
	srv.Close() // nothing is listening any more

	var err error
	require.NotPanics(t, func() {
		_, _, err = transport.MakeRequest(context.Background(), cfg, "/instances", &transport.RequestConfig{Method: http.MethodGet})
	})

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "want a typed error, got %T: %v", err, err)
	assert.Equal(t, 8, ce.Code, "a transport failure is a retryable upstream error")
	assert.Contains(t, ce.Message, "can't retrieve authentication token")
}
