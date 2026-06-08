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

	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/output"
)

// scrub redacts secrets (RedactText) then neutralises terminal control/ANSI
// escapes (StripControl) before any string is written to the operator's
// terminal. Order matters: redact first, strip on the result.
func scrub(s string) string {
	return output.StripControl(clievents.RedactText(s))
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
