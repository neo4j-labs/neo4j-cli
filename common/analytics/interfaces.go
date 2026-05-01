// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

//go:generate mockgen -destination=mocks/mock_analytics.go -package=analytics_mocks -typed github.com/neo4j/cli/common/analytics/mocks Service,HTTPClient

import (
	"io"
	"net/http"
)

// HTTPClient is the subset of *http.Client used by Analytics, allowing injection of a mock in tests.
type HTTPClient interface {
	Post(url, contentType string, body io.Reader) (*http.Response, error)
}

type Service interface {
	Disable()
	Enable()
	IsEnabled() bool
	// EmitStartupEvent records that the CLI has started with all standard
	// base properties (OS, machine ID, version, etc.).
	EmitStartupEvent()
	// EmitCommandEvent records a CLI command invocation with the full command
	// path (e.g. "cloud instances list"), success, and active flags.
	EmitCommandEvent(command string, success bool, flags ActiveFlags)
	// EmitEvent queues a pre-built event. Prefer EmitToolEvent for tool
	// invocations — it attaches all standard properties automatically.
	EmitEvent(event TrackEvent)
	// Flush blocks until all in-flight async EmitEvent goroutines have completed.
	// Call it during shutdown to avoid dropping events.
	Flush()
}
