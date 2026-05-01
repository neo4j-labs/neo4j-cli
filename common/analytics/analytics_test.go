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
	svc.EmitEvent(analytics.TrackEvent{Event: "should_not_be_sent"})
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
	svc.EmitEvent(analytics.TrackEvent{Event: "test_event"})
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
			svc.EmitEvent(analytics.TrackEvent{Event: "url_test"})
			svc.Flush()
		})
	}
}

func TestEmitEvent_CorrectBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)

	event := analytics.TrackEvent{
		Event:      "body_test",
		Properties: map[string]interface{}{"key": "value"},
	}

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
			if events[0].Event != "body_test" {
				t.Errorf("event name: got %s, want body_test", events[0].Event)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("1")),
			}, nil
		})

	svc := newTestAnalytics(t, mockClient)
	svc.EmitEvent(event)
	svc.Flush()
}

// ---- Enable / Disable / IsEnabled ----------------------------------------

func TestEnableDisable(t *testing.T) {
	svc := newTestAnalytics(t, nil)

	if !svc.IsEnabled() {
		t.Error("should be enabled by default")
	}

	svc.Disable()
	if svc.IsEnabled() {
		t.Error("should be disabled after Disable()")
	}

	svc.Enable()
	if !svc.IsEnabled() {
		t.Error("should be enabled after Enable()")
	}
}

// ---- Event constructors --------------------------------------------------

func TestEmitStartupEvent_SendsEvent(t *testing.T) {
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
			if !strings.HasSuffix(events[0].Event, "STARTUP") {
				t.Errorf("expected STARTUP event, got %q", events[0].Event)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("1")),
			}, nil
		})

	svc := newTestAnalytics(t, mockClient)
	svc.EmitStartupEvent()
	svc.Flush()
}

// TestEmitStartupEvent_Disabled verifies that a disabled analytics instance
// does not send the startup event.
func TestEmitStartupEvent_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)
	// No Post calls expected.

	svc := newTestAnalytics(t, mockClient)
	svc.Disable()
	svc.EmitStartupEvent()
}

func TestNewStartupEvent(t *testing.T) {
	svc := newTestAnalytics(t, nil)
	event := svc.NewStartupEvent()

	if !strings.HasSuffix(event.Event, "STARTUP") {
		t.Errorf("unexpected event name: %s", event.Event)
	}
	assertBaseProperties(t, event.Properties)
}

func TestNewToolEvent(t *testing.T) {
	svc := newTestAnalytics(t, nil)

	t.Run("success", func(t *testing.T) {
		event := svc.NewToolEvent("echo", true)
		if !strings.HasSuffix(event.Event, "TOOL_USED") {
			t.Errorf("unexpected event name: %s", event.Event)
		}
		props := assertBaseProperties(t, event.Properties)
		if props["tool_name"] != "echo" {
			t.Errorf("tool_name: got %v, want echo", props["tool_name"])
		}
		if props["success"] != true {
			t.Errorf("success: got %v, want true", props["success"])
		}
	})

	t.Run("failure", func(t *testing.T) {
		event := svc.NewToolEvent("query", false)
		props := assertBaseProperties(t, event.Properties)
		if props["success"] != false {
			t.Errorf("success: got %v, want false", props["success"])
		}
	})
}

// TestEmitToolEvent_SendsFullyPopulatedEvent verifies that EmitToolEvent
// produces an event with the correct name and tool properties — not a bare
// TrackEvent{Event: "tool_used"} with no properties.
func TestEmitToolEvent_SendsFullyPopulatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := amocks.NewMockHTTPClient(ctrl)

	var capturedBody []byte
	mockClient.EXPECT().
		Post(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_, _ string, body io.Reader) (*http.Response, error) {
			var err error
			capturedBody, err = io.ReadAll(body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("1")),
			}, nil
		})

	svc := newTestAnalytics(t, mockClient)
	svc.EmitToolEvent("query", true)
	svc.Flush()

	// Decode the raw Mixpanel payload and verify event properties.
	var events []analytics.TrackEvent
	if err := json.Unmarshal(capturedBody, &events); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if !strings.HasSuffix(event.Event, "TOOL_USED") {
		t.Errorf("event name should end with TOOL_USED, got %q", event.Event)
	}

	props := decodeProperties(t, event.Properties)
	if props["tool_name"] != "query" {
		t.Errorf("tool_name: got %v, want query", props["tool_name"])
	}
	if props["success"] != true {
		t.Errorf("success: got %v, want true", props["success"])
	}
	// Verify base properties are present.
	if _, ok := props["distinct_id"]; !ok {
		t.Error("event should contain distinct_id")
	}
	if _, ok := props["$os"]; !ok {
		t.Error("event should contain $os")
	}
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
