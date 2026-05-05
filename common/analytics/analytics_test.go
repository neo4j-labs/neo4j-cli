// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics_test

import (
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/analytics"
	amocks "github.com/neo4j/cli/common/analytics/mocks"

	"go.uber.org/mock/gomock"
)

// newTestAnalytics creates an Analytics instance wired to a mock HTTP client.
// It registers t.Cleanup(a.Flush) so the background worker goroutine is
// always stopped at the end of each test, regardless of whether the test
// calls Flush explicitly.
func newTestAnalytics(t *testing.T, client analytics.HTTPClient) *analytics.Analytics {
	t.Helper()
	a := analytics.NewAnalyticsWithClient("test-token", "http://localhost", client, "bolt://localhost:7687", "1.2.3", nil)
	t.Cleanup(a.Flush)
	return a
}

// decodeProperties marshals props through JSON and returns a flat map so tests
// can assert individual field values without caring about the concrete struct type.
func decodeProperties(t *testing.T, props interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal properties: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal properties to map: %v", err)
	}
	return m
}

func assertBaseProperties(t *testing.T, props interface{}) map[string]interface{} {
	t.Helper()
	m := decodeProperties(t, props)
	if m["token"] != "test-token" {
		t.Errorf("token: got %v, want test-token", m["token"])
	}
	if _, ok := m["time"].(float64); !ok {
		t.Error("time is not a number")
	}
	if _, ok := m["distinct_id"].(string); !ok {
		t.Error("distinct_id is not a string")
	}
	if _, ok := m["$insert_id"].(string); !ok {
		t.Error("$insert_id is not a string")
	}
	if _, ok := m["uptime"].(float64); !ok {
		t.Error("uptime is not a number")
	}
	if m["$os"] != runtime.GOOS {
		t.Errorf("$os: got %v, want %v", m["$os"], runtime.GOOS)
	}
	if m["os_arch"] != runtime.GOARCH {
		t.Errorf("os_arch: got %v, want %v", m["os_arch"], runtime.GOARCH)
	}
	if m["cli_version"] != "1.2.3" {
		t.Errorf("cli_version: got %v, want 1.2.3", m["cli_version"])
	}
	return m
}

// ---- Emit behaviour -------------------------------------------------------

func TestEmitEvent_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)
	// No Post calls expected — the mock will fail the test if Post is called.

	svc := newTestAnalytics(t, mockClient)
	svc.Disable()
	svc.EmitEvent("should_not_be_sent", analytics.TrackEvent{})
}

func TestEmitEvent_Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)

	mockClient.EXPECT().
		Post(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("1")),
		}, nil)

	svc := newTestAnalytics(t, mockClient)
	svc.EmitEvent("test_event", analytics.TrackEvent{})
	svc.Flush()
}

func TestEmitEvent_CorrectURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantURL  string
	}{
		{"trailing slash", "http://localhost/", "http://localhost/track?verbose=1"},
		{"no trailing slash", "http://localhost", "http://localhost/track?verbose=1"},
		{"double trailing slash", "http://localhost//", "http://localhost/track?verbose=1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := amocks.NewMockHTTPClient(ctrl)

			mockClient.EXPECT().
				Post(tc.wantURL, gomock.Any(), gomock.Any()).
				Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("1")),
				}, nil)

			svc := analytics.NewAnalyticsWithClient("test-token", tc.endpoint, mockClient, "", "1.2.3", nil)
			t.Cleanup(svc.Flush)
			svc.EmitEvent("url_test", analytics.TrackEvent{})
			svc.Flush()
		})
	}
}

func TestEmitEvent_CorrectBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)

	mockClient.EXPECT().
		Post(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_, _ string, body io.Reader) (*http.Response, error) {
			b, _ := io.ReadAll(body)
			var events []analytics.TrackEvent
			if err := json.Unmarshal(b, &events); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if !strings.HasSuffix(events[0].Event, "body_test") {
				t.Errorf("event name: got %s, want suffix body_test", events[0].Event)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("1")),
			}, nil
		})

	svc := newTestAnalytics(t, mockClient)
	svc.EmitEvent("body_test", analytics.TrackEvent{Properties: map[string]interface{}{"key": "value"}})
	svc.Flush()
}

// ---- Disable ------------------------------------------------------------

func TestDisable_SuppressesEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)
	// No Post calls expected — mock will fail the test if Post is called.

	svc := newTestAnalytics(t, mockClient)
	svc.Disable()
	svc.EmitEvent("should_not_be_sent", analytics.TrackEvent{})
	svc.Flush()
}

// ---- Event constructors --------------------------------------------------

// TestNewStartupEvent has been removed — NewStartupEvent no longer exists.
// Event naming is now handled by EmitEvent via the eventSuffix parameter.
// The STARTUP suffix is verified via TestEmitStartupEvent_SendsEvent.

// TestEmitEvent_IncludesBaseProperties verifies that base properties are always
// merged into the outgoing payload by sendTrackEvent, even when EmitEvent is
// called with a TrackEvent that carries no Properties of its own.
func TestEmitEvent_IncludesBaseProperties(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)

	mockClient.EXPECT().
		Post(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_, _ string, body io.Reader) (*http.Response, error) {
			b, _ := io.ReadAll(body)
			// The Mixpanel SDK wraps events in a JSON array.
			var payload []struct {
				Event      string                 `json:"event"`
				Properties map[string]interface{} `json:"properties"`
			}
			if err := json.Unmarshal(b, &payload); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			if len(payload) != 1 {
				t.Fatalf("expected 1 event in payload, got %d", len(payload))
			}
			props := payload[0].Properties
			assertBaseProperties(t, props)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("1")),
			}, nil
		})

	svc := newTestAnalytics(t, mockClient)
	// Emit with nil Properties — base props must still appear in the payload.
	svc.EmitEvent("base_props_test", analytics.TrackEvent{})
	svc.Flush()
}

// TestIsAura verifies the Aura URI detection (exercises the package-level
// compiled regex that replaced the per-call regexp.MustCompile).
// isAura is unexported, so we test it via the exported IsAura helper.
func TestIsAura(t *testing.T) {
	tests := []struct {
		uri  string
		want bool
	}{
		{"bolt+s://abc123.databases.neo4j.io", true},
		{"neo4j+s://xyz.instances.neo4j.io", true},
		{"bolt://mydb.databases.neo4j.io:7687", true},
		{"bolt://localhost:7687", false},
		{"bolt://192.168.1.1:7687", false},
		{"bolt://myprivate.neo4j.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := analytics.IsAuraURI(tt.uri)
		if got != tt.want {
			t.Errorf("IsAuraURI(%q) = %v, want %v", tt.uri, got, tt.want)
		}
	}
}
