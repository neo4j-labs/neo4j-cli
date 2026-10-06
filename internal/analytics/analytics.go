// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/denisbrodbeck/machineid"
	mixpanel "github.com/mixpanel/mixpanel-go"

	"github.com/tklauser/ps"
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
	rawURL := t.endpoint + "/" + path
	if req.URL.RawQuery != "" {
		rawURL += "?" + req.URL.RawQuery
	}
	inner, err := http.NewRequestWithContext(req.Context(), http.MethodPost, rawURL, req.Body)
	if err != nil {
		return nil, err
	}
	inner.Header.Set("Content-Type", req.Header.Get("Content-Type"))
	return t.client.Do(inner)
}

type analyticsConfig struct {
	pprocessPath string
	distinctID   string
	cliVersion   string
	token        string
	startupTime  int64
	appName      string
	mp           *mixpanel.ApiClient
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

	// closed is set by Flush() before closing eventCh.
	// EmitEvent checks this to avoid a send-on-closed-channel panic.
	closed atomic.Bool

	// mu serialises starting the worker, sending an event and closing the
	// channel, so Flush can never close eventCh under a concurrent send. The
	// send is non-blocking, so holding mu across it is cheap.
	mu sync.Mutex

	// started reports whether the worker goroutine exists. The worker (and the
	// machine-ID lookup it needs) start lazily on the first event, so a Config
	// that never emits — and every Config when telemetry is disabled — costs
	// nothing and has nothing to Flush.
	started bool

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
		httpClient := &http.Client{Transport: &httpClientTransport{client: client, endpoint: endpoint}, Timeout: 2 * time.Second}
		mpClient = mixpanel.NewApiClient(mixPanelToken,
			mixpanel.HttpClient(httpClient),
		)
	} else {
		httpClient := &http.Client{Timeout: 2 * time.Second}
		mpClient = mixpanel.NewApiClient(mixPanelToken,
			mixpanel.ProxyApiLocation(endpoint),
			mixpanel.HttpClient(httpClient),
		)
	}

	parentProcessPath := GetParentProcessPath()

	a := &Analytics{
		log:     log,
		eventCh: make(chan TrackEvent, eventBufferSize),
		cfg: analyticsConfig{
			// distinctID is the stable, OS-derived machine ID so Mixpanel can
			// correlate events across sessions for the same user. It is resolved
			// when the worker starts (see EmitEvent), not here: the lookup can
			// shell out, and most Configs never emit an event.
			cliVersion:   version,
			token:        mixPanelToken,
			startupTime:  time.Now().Unix(),
			mp:           mpClient,
			appName:      appName,
			pprocessPath: parentProcessPath,
		},
	}

	return a
}

// EmitEvent queues an analytics event for the background worker.
// The full event name is constructed as "<appName>_<eventSuffix>".
// It never blocks: if the internal buffer is full the event is dropped
// and a warning is logged. Safe to call after Flush() — it is a no-op.
func (a *Analytics) EmitEvent(eventSuffix string, event TrackEvent) {
	if a.disabled {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed.Load() {
		return
	}
	if !a.started {
		// Start the single background worker that serialises all Mixpanel calls.
		a.started = true
		a.cfg.distinctID = GetMachineID(a.cfg.appName)
		a.wg.Add(1)
		go a.worker()
	}
	event.Event = strings.Join([]string{a.cfg.appName, eventSuffix}, "_")
	select {
	case a.eventCh <- event:
		a.logDebug("queued analytics event", "event", slog.StringValue(event.Event))
	default:
		a.logWarn("analytics buffer full — dropping event", "event", slog.StringValue(event.Event), "buffer_size", slog.Int64Value(eventBufferSize))

	}
}

// Flush closes the event channel and blocks until the worker (if one was ever
// started) has sent every queued event. Call it once during application
// shutdown. After Flush returns, EmitEvent is a safe no-op. Flushing a service
// that never emitted returns immediately.
func (a *Analytics) Flush() {
	a.mu.Lock()
	// Mark as closed before closing the channel so EmitEvent's guard fires
	// first and we never race a send against a close.
	if a.closed.CompareAndSwap(false, true) {
		close(a.eventCh)
	}
	a.mu.Unlock()
	a.wg.Wait()
}

// worker is the single goroutine that drains eventCh and forwards events to
// Mixpanel. It exits when the channel is closed and fully drained (i.e. after
// Flush() is called).
func (a *Analytics) worker() {
	defer a.wg.Done()
	for event := range a.eventCh {
		if err := a.sendTrackEvent([]TrackEvent{event}); err != nil {
			a.logDebug("error sending analytics event", "event", slog.StringValue(event.Event), "error", err.Error())
		}
	}
}

func (a *Analytics) Disable() { a.disabled = true }

func (a *Analytics) sendTrackEvent(events []TrackEvent) error {
	// Build the base property map once per batch — timestamps and uptime are
	// captured here so they reflect actual send time rather than construction time.
	baseProps, err := toPropertiesMap(a.getBaseProperties())
	if err != nil {
		return fmt.Errorf("marshal base properties: %w", err)
	}

	sdkEvents := make([]*mixpanel.Event, 0, len(events))
	for _, e := range events {
		// Start with base properties, then let event-specific fields override.
		merged := make(map[string]any, len(baseProps))
		for k, v := range baseProps {
			merged[k] = v
		}
		if e.Properties != nil {
			eventProps, err := toPropertiesMap(e.Properties)
			if err != nil {
				return fmt.Errorf("marshal properties for event %q: %w", e.Event, err)
			}
			for k, v := range eventProps {
				merged[k] = v
			}
		}
		sdkEvents = append(sdkEvents, a.cfg.mp.NewEvent(e.Event, a.cfg.distinctID, merged))
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

//nolint:unused
func (a *Analytics) logInfo(msg string, fields ...any) {
	if a.log != nil {
		a.log.Info(msg, fields...)
	}
}

//nolint:unused
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

// GetParentProcessPath returns the file name of the binary of the process that
// called this application (e.g. "zsh", "Code Helper"), without its directory,
// so no home directory or username reaches analytics. It returns "" when the
// parent cannot be determined.
func GetParentProcessPath() string {
	p, err := ps.FindProcess(os.Getppid())
	if err != nil {
		slog.Error("Failed to obtain the parent process", "error", err)
		return ""
	}

	path := p.ExecutablePath()
	if path == "" {
		return ""
	}
	name := filepath.Base(path)

	slog.Debug("Parent process binary", "name", name)

	return name
}
