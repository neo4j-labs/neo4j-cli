// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	commondebug "github.com/neo4j/cli/common/debug"
	"github.com/neo4j/cli/common/output"
)

// scrub delegates to debug.Scrub (StripControl(RedactText)).
//
// security: redaction here is best-effort. clievents.RedactText is shape-based
// (Authorization Bearer/Basic headers, URI userinfo passwords, and JSON/kv keys
// whose name contains a secretWords substring, plus literal RegisterSecretValue
// values). The --debug dump emits the FULL raw request/response bodies, which is
// broader than the fields a command normally shows, so a secret nested under a
// custom/opaque key with no secretWords substring can surface here in the clear.
// This is the user's own secret in their own terminal; the residual risk is
// mainly pasting --debug output into a bug report.
func scrub(s string) string {
	return commondebug.Scrub(s)
}

// debugW is the destination for --debug diagnostics from the desktopclient
// package. It defaults to os.Stderr and is overridable in tests, following the
// existing var-seam pattern.
var debugW io.Writer = os.Stderr

// debugEnabled gates every emit helper. Unlike the aura api package (which
// gates at the call site via cfg.Aura.Debug()), the gate lives inside the
// helpers here so call sites in client.go/discovery.go can call them
// unconditionally. Toggle via SetDebug.
var debugEnabled bool

// SetDebug enables or disables --debug diagnostics for the desktopclient
// package. It is wired from the desktop command's PersistentPreRunE.
func SetDebug(enabled bool) {
	debugEnabled = enabled
}

// DebugEnabled reports whether --debug diagnostics are currently enabled. It
// lets callers (and tests asserting the desktop-root PersistentPreRunE
// resolution) read the package-global gate that SetDebug toggles.
func DebugEnabled() bool {
	return debugEnabled
}

// SetDebugWriterForTest overrides the package-level debug seam (debugW) for the
// duration of the test, restoring the previous value via t.Cleanup. It is
// exported (not export_test.go) so the external desktop command tests can
// capture this package's --debug diagnostics. The package-global debugEnabled
// gate is a process-global; tests sharing it must reset between cases.
func SetDebugWriterForTest(t *testing.T, w io.Writer) {
	t.Helper()
	prev := debugW
	debugW = w
	t.Cleanup(func() { debugW = prev })
}

// SetDebugForTest toggles the package-level debugEnabled gate for the duration
// of the test, restoring the previous value via t.Cleanup.
func SetDebugForTest(t *testing.T, enabled bool) {
	t.Helper()
	prev := debugEnabled
	debugEnabled = enabled
	t.Cleanup(func() { debugEnabled = prev })
}

const (
	debugReqPrefix  = "[desktop-debug] > "
	debugRespPrefix = "[desktop-debug] < "
	debugInfoPrefix = "[desktop-debug] "
)

// debugRequest emits the method, URL, headers, and body of an outgoing request.
func debugRequest(method, url string, header http.Header, body []byte) {
	if !debugEnabled {
		return
	}
	_, _ = fmt.Fprintf(debugW, "%s%s %s\n", debugReqPrefix, method, scrub(url))
	writeHeaders(debugReqPrefix, header)
	if len(body) > 0 {
		_, _ = fmt.Fprintf(debugW, "%s%s\n", debugReqPrefix, scrub(string(body)))
	}
}

// debugResponse emits the status, headers, body, and elapsed duration of a
// response.
func debugResponse(statusCode int, header http.Header, body []byte, elapsed time.Duration) {
	if !debugEnabled {
		return
	}
	_, _ = fmt.Fprintf(debugW, "%s%d %s\n", debugRespPrefix, statusCode, http.StatusText(statusCode))
	writeHeaders(debugRespPrefix, header)
	if len(body) > 0 {
		_, _ = fmt.Fprintf(debugW, "%s%s\n", debugRespPrefix, scrub(string(body)))
	}
	_, _ = fmt.Fprintf(debugW, "%selapsed %s\n", debugInfoPrefix, elapsed)
}

// debugInfo emits a single redacted [desktop-debug] line for context that is
// not a full request/response (discovery probes, mDNS outcomes, transport
// errors).
func debugInfo(format string, args ...any) {
	if !debugEnabled {
		return
	}
	_, _ = fmt.Fprintf(debugW, "%s%s\n", debugInfoPrefix, scrub(fmt.Sprintf(format, args...)))
}

func writeHeaders(prefix string, header http.Header) {
	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range header[k] {
			_, _ = fmt.Fprintf(debugW, "%s%s: %s\n", prefix, output.StripControl(k), scrub(v))
		}
	}
}
