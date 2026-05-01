// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	mixpanel "github.com/mixpanel/mixpanel-go"
)

// httpClientTransport adapts our HTTPClient interface into an http.RoundTripper,
// allowing the Mixpanel SDK to use our injectable client (including mocks in tests).
// The endpoint is stored here so we can rewrite the URL on every request —
// the SDK resolves its own internal URL before hitting the transport, which
// would otherwise bypass our configured proxy endpoint.
type httpClientTransport struct {
	client   HTTPClient
	endpoint string
}

func (t *httpClientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	path := strings.TrimLeft(req.URL.Path, "/")
	url := t.endpoint + "/" + path
	if req.URL.RawQuery != "" {
		url += "?" + req.URL.RawQuery
	}
	return t.client.Post(url, req.Header.Get("Content-Type"), req.Body)
}

type analyticsConfig struct {
	distinctID  string
	machineID   string
	cliVersion  string
	token       string
	startupTime int64
	appName     string
	mp          *mixpanel.ApiClient
}

// eventBufferSize is the capacity of the internal event channel.
// If the buffer fills up (e.g. extended network outage) EmitEvent drops
// the event and logs a warning rather than blocking the caller.
const eventBufferSize = 64

type Analytics struct {
	disabled bool
	cfg      analyticsConfig
	log      *slog.Logger

	// eventCh carries events to the single background worker.
	// Closed by Flush() to signal the worker to drain and exit.
	eventCh chan TrackEvent

	// closed is set to 1 by Flush() before closing eventCh.
	// EmitEvent checks this to avoid a send-on-closed-channel panic.
	closed atomic.Bool

	// wg tracks the single worker goroutine so Flush() can wait for it.
	wg sync.WaitGroup
}

// NewAnalytics creates an Analytics instance using the default http.Client.
// mixpanelToken , the token for mixPanel
// mixpanelEndpoint, the mixpanel endpoint to use
// appName is used as the prefix for events
// version of the application using analytics
// log allows for passing in custom instance of slog
func NewAnalytics(mixPanelToken string, mixpanelEndpoint string, appName string, version string, log *slog.Logger) *Analytics {
	return NewAnalyticsWithClient(mixPanelToken, mixpanelEndpoint, &http.Client{Timeout: 10 * time.Second}, appName, version, log)
}

// NewAnalyticsWithClient creates an Analytics instance with an injectable HTTPClient,
// allowing tests to intercept outbound Mixpanel calls via a mock.
// log may be nil; in that case analytics logs nothing on the injected logger.
func NewAnalyticsWithClient(mixPanelToken string, mixpanelEndpoint string, client HTTPClient, appName string, version string, log *slog.Logger) *Analytics {
	endpoint := strings.TrimRight(mixpanelEndpoint, "/")

	var mpClient *mixpanel.ApiClient

	if client != nil {
		httpClient := &http.Client{Transport: &httpClientTransport{client: client, endpoint: endpoint}}
		mpClient = mixpanel.NewApiClient(mixPanelToken,
			mixpanel.HttpClient(httpClient),
		)
	} else {
		mpClient = mixpanel.NewApiClient(mixPanelToken,
			mixpanel.ProxyApiLocation(endpoint),
		)
	}

	a := &Analytics{
		log:     log,
		eventCh: make(chan TrackEvent, eventBufferSize),
		cfg: analyticsConfig{
			// Use the stable, OS-derived machine ID as the distinct ID so that
			// Mixpanel can correlate events across sessions for the same user.
			distinctID:  GetMachineID(appName),
			machineID:   GetMachineID(appName),
			cliVersion:  version,
			token:       mixPanelToken,
			startupTime: time.Now().Unix(),
			mp:          mpClient,
			appName:     appName,
		},
	}

	// Start the single background worker that serialises all Mixpanel calls.
	a.wg.Add(1)
	go a.worker()

	return a
}

// EmitStartupEvent queues a startup event with all standard base properties.
func (a *Analytics) EmitStartupEvent() {
	a.logInfo("Startup event sent")
	a.EmitEvent(a.NewStartupEvent())
}

// EmitCommandEvent queues a command invocation event with the full command
// path, success flag, and active persistent flags.
func (a *Analytics) EmitCommandEvent(command string, success bool, flags ActiveFlags) {
	a.EmitEvent(a.NewCommandEvent(command, success, flags))
}

// EmitToolEvent records a tool invocation outcome with all standard
// base properties. It is the preferred way to emit tool events from the
// shell because it ensures the correct event name and property set.
func (a *Analytics) EmitToolEvent(toolName string, success bool) {
	a.EmitEvent(a.NewToolEvent(toolName, success))
}

// EmitEvent queues an analytics event for the background worker.
// It never blocks: if the internal buffer is full the event is dropped
// and a warning is logged. Safe to call after Flush() — it is a no-op.
func (a *Analytics) EmitEvent(event TrackEvent) {
	if a.disabled || a.closed.Load() {
		return
	}
	select {
	case a.eventCh <- event:
		a.logDebug("queued analytics event", "event", slog.StringValue(event.Event))
		a.logInfo("queued analytics event", "event", slog.StringValue(event.Event))
	default:
		a.logWarn("analytics buffer full — dropping event", "event", slog.StringValue(event.Event), "buffer_size", slog.Int64Value(eventBufferSize))

	}
}

// Flush closes the event channel and blocks until the worker has sent every
// queued event. Call it once during application shutdown.
// After Flush returns, EmitEvent is a safe no-op.
func (a *Analytics) Flush() {
	// Mark as closed before closing the channel so EmitEvent's guard fires
	// first and we never race a send against a close.
	if a.closed.CompareAndSwap(false, true) {
		close(a.eventCh)
	}
	a.wg.Wait()
}

// worker is the single goroutine that drains eventCh and forwards events to
// Mixpanel. It exits when the channel is closed and fully drained (i.e. after
// Flush() is called).
func (a *Analytics) worker() {
	defer a.wg.Done()
	for event := range a.eventCh {
		if err := a.sendTrackEvent([]TrackEvent{event}); err != nil {
			a.logError("error sending analytics event", "event", slog.StringValue(event.Event), "error", err.Error())
		}
		a.logInfo("Event worker sent event", "event", slog.AnyValue(event))
	}
}

func (a *Analytics) Enable()         { a.disabled = false }
func (a *Analytics) Disable()        { a.disabled = true }
func (a *Analytics) IsEnabled() bool { return !a.disabled }

func (a *Analytics) sendTrackEvent(events []TrackEvent) error {
	sdkEvents := make([]*mixpanel.Event, 0, len(events))
	for _, e := range events {
		props, err := toPropertiesMap(e.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties for event %q: %w", e.Event, err)
		}
		sdkEvents = append(sdkEvents, a.cfg.mp.NewEvent(e.Event, a.cfg.distinctID, props))
	}

	if err := a.cfg.mp.Track(context.Background(), sdkEvents); err != nil {
		return fmt.Errorf("mixpanel track error: %w", err)
	}
	a.logDebug("sent event to Mixpanel", "event", slog.StringValue(sdkEvents[0].Name))
	return nil
}

// logDebug logs at debug level if a logger has been injected.
// Use for internal pipeline messages that are only relevant when diagnosing issues.
func (a *Analytics) logDebug(msg string, fields ...any) {
	if a.log != nil {
		a.log.Debug(msg, fields...)
	}
}

// logWarn logs at warn level if a logger has been injected.
func (a *Analytics) logWarn(msg string, fields ...any) {
	if a.log != nil {
		a.log.Warn(msg, fields...)
	}
}

// logWarn logs at info level if a logger has been injected.
func (a *Analytics) logInfo(msg string, fields ...any) {
	if a.log != nil {
		a.log.Info(msg, fields...)
	}
}

// logError logs at error level if a logger has been injected.
func (a *Analytics) logError(msg string, fields ...any) {
	if a.log != nil {
		a.log.Error(msg, fields...)
	}
}

// toPropertiesMap converts any properties struct to map[string]any via JSON
// so it's compatible with the SDK without duplicating field mappings.
func toPropertiesMap(props any) (map[string]any, error) {
	b, err := json.Marshal(props)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetMachineID returns a stable, privacy-safe machine identifier using the OS-provided
// hardware UUID, HMAC-hashed with the app name so the raw system UUID is never exposed.
// Returns an empty string on failure (e.g. insufficient permissions on some Linux configs).
func GetMachineID(appName string) string {
	id, err := machineid.ProtectedID(appName)
	if err != nil {
		slog.Warn("Could not retrieve machine ID for analytics", "error", err)
		return ""
	}
	return id
}

func GetDistinctID() string {
	id, err := uuid.NewV6()
	if err != nil {
		slog.Error("Error generating distinct ID for analytics", "error", err.Error())
		return ""
	}
	return id.String()
}
