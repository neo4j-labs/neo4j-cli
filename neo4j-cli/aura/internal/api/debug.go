// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/neo4j/cli/common/debug"
	"github.com/neo4j/cli/common/output"
)

// scrub delegates to debug.Scrub (StripControl(RedactText)).
//
// security: redaction here is best-effort. clievents.RedactText is shape-based
// (Authorization Bearer/Basic headers, URI userinfo passwords, and JSON/kv keys
// whose name contains a secretWords substring, plus literal RegisterSecretValue
// values). The --debug dump emits the FULL raw request/response bodies, which is
// broader than the PrintBody-filtered fields a command normally shows, so a
// secret nested under a custom/opaque key with no secretWords substring (e.g.
// `agent create --tools` JSON carrying {"headers":{"X-Auth":"..."}}) can surface
// here in the clear. This is the user's own secret in their own terminal; the
// residual risk is mainly pasting --debug output into a bug report.
func scrub(s string) string {
	return debug.Scrub(s)
}

// debugW is the destination for --debug diagnostics from the api package
// (MakeRequest/getToken/Poll). It defaults to os.Stderr and is overridable in
// tests, following the existing var-seam pattern (e.g. driverOpener).
var debugW io.Writer = os.Stderr

const (
	debugReqPrefix  = "[aura-debug] > "
	debugRespPrefix = "[aura-debug] < "
	debugInfoPrefix = "[aura-debug] "
)

// debugRequest emits the method, URL, headers, and body of an outgoing request.
func debugRequest(method, url string, header http.Header, body []byte) {
	_, _ = fmt.Fprintf(debugW, "%s%s %s\n", debugReqPrefix, method, scrub(url))
	writeHeaders(debugReqPrefix, header)
	if len(body) > 0 {
		_, _ = fmt.Fprintf(debugW, "%s%s\n", debugReqPrefix, scrub(string(body)))
	}
}

// debugResponse emits the status, headers, body, and elapsed duration of a
// response.
func debugResponse(statusCode int, header http.Header, body []byte, elapsed time.Duration) {
	_, _ = fmt.Fprintf(debugW, "%s%d %s\n", debugRespPrefix, statusCode, http.StatusText(statusCode))
	writeHeaders(debugRespPrefix, header)
	if len(body) > 0 {
		_, _ = fmt.Fprintf(debugW, "%s%s\n", debugRespPrefix, scrub(string(body)))
	}
	_, _ = fmt.Fprintf(debugW, "%selapsed %s\n", debugInfoPrefix, elapsed)
}

// debugInfo emits a single redacted [aura-debug] line for loop/credential-level
// context (token acquisition, poll attempts).
func debugInfo(format string, args ...any) {
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
