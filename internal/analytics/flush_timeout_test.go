// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics_test

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neo4j/cli/internal/analytics"
	amocks "github.com/neo4j/cli/internal/analytics/mocks"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// blockingClient simulates a slow or unreachable analytics endpoint: every Do
// blocks until release is closed. calls signals that the worker has picked up
// an event and is inside Do.
type blockingClient struct {
	calls   chan struct{}
	release chan struct{}
}

func (c *blockingClient) Do(*http.Request) (*http.Response, error) {
	c.calls <- struct{}{}
	<-c.release
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("1")),
	}, nil
}

func TestFlushTimeout_ReturnsBeforeSlowSendCompletes(t *testing.T) {
	client := &blockingClient{calls: make(chan struct{}, 1), release: make(chan struct{})}
	// Always release the blocked send exactly once so newTestAnalytics's
	// cleanup Flush can drain and stop the worker, no matter how the
	// assertions go (a plain defer would run too late for the explicit
	// Flush below, and a double close would panic).
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(client.release) }) }
	t.Cleanup(release)

	svc := newTestAnalytics(t, client)
	svc.EmitEvent("slow_event", analytics.TrackEvent{})
	<-client.calls // the worker is blocked inside Do

	start := time.Now()
	svc.FlushTimeout(50 * time.Millisecond)
	elapsed := time.Since(start)

	// The send is still blocked, so an unbounded Flush would wait out the
	// event's HTTP timeout here. FlushTimeout must return at its deadline.
	assert.Less(t, elapsed, 400*time.Millisecond,
		"FlushTimeout must return at its deadline while a send is still in flight, took %v", elapsed)

	// The worker keeps draining in the background: once released, a plain
	// Flush completes.
	release()
	svc.Flush()
}

func TestFlushTimeout_DrainsFastSends(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)
	mockClient.EXPECT().
		Do(gomock.Any()).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("1")),
		}, nil)

	svc := newTestAnalytics(t, mockClient)
	svc.EmitEvent("fast_event", analytics.TrackEvent{})
	svc.FlushTimeout(analytics.DefaultShutdownFlushTimeout)

	// gomock verifies the Do expectation when the controller finishes, so
	// reaching here with the expectation unfulfilled would fail the test:
	// the event must have been sent within the budget.
}

func TestFlushTimeout_ZeroTimeoutIsUnbounded(t *testing.T) {
	client := &blockingClient{calls: make(chan struct{}, 1), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(client.release) }) }
	t.Cleanup(release)

	svc := newTestAnalytics(t, client)
	svc.EmitEvent("slow_event", analytics.TrackEvent{})
	<-client.calls

	done := make(chan struct{})
	go func() {
		svc.FlushTimeout(0)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("FlushTimeout(0) returned while the send was still blocked")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FlushTimeout(0) did not return after the send completed")
	}
}
