// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics_test

import (
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/neo4j/cli/internal/analytics"
	amocks "github.com/neo4j/cli/internal/analytics/mocks"
)

// settledGoroutines returns the goroutine count after letting finished ones exit.
func settledGoroutines() int {
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	return runtime.NumGoroutine()
}

func TestNewAnalytics_StartsNoWorkerUntilTheFirstEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl) // no calls expected

	before := settledGoroutines()
	svc := newTestAnalytics(t, mockClient)
	assert.LessOrEqual(t, settledGoroutines(), before, "constructing the service must not start a goroutine")

	svc.Flush() // nothing was emitted: returns immediately, nothing to leak
	assert.LessOrEqual(t, settledGoroutines(), before)
}

func TestDisabledAnalytics_NeverStartsAWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)

	before := settledGoroutines()
	svc := newTestAnalytics(t, mockClient)
	svc.Disable()
	for i := 0; i < 10; i++ {
		svc.EmitEvent("ignored", analytics.TrackEvent{})
	}
	svc.Flush()

	assert.LessOrEqual(t, settledGoroutines(), before, "disabled telemetry costs nothing: no worker, no machine-id lookup")
}

func TestEmitEvent_AfterFlushIsANoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)
	mockClient.EXPECT().Do(gomock.Any()).Return(&http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("1"))}, nil).Times(1)

	svc := newTestAnalytics(t, mockClient)
	svc.EmitEvent("first", analytics.TrackEvent{})
	svc.Flush()
	svc.EmitEvent("late", analytics.TrackEvent{}) // must not panic or send
	svc.Flush()                                   // idempotent
}

// Emitting from many goroutines while another flushes used to be able to send on
// a closed channel; run with -race.
func TestEmitEvent_ConcurrentWithFlushIsSafe(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)
	mockClient.EXPECT().Do(gomock.Any()).
		Return(&http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("1"))}, nil).
		AnyTimes()

	svc := newTestAnalytics(t, mockClient)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.EmitEvent("burst", analytics.TrackEvent{})
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		svc.Flush()
	}()

	assert.NotPanics(t, wg.Wait)
}
