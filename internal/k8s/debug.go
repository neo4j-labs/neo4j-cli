// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/neo4j/cli/internal/clievents"
	"github.com/neo4j/cli/internal/output"
)

// debugW is the destination for --debug diagnostics from this package. It
// defaults to os.Stderr. Tests redirect it through SetDebugWriterForTest,
// mirroring the aura and docker engines.
var debugW io.Writer = os.Stderr

// DebugReqPrefix and DebugRespPrefix start every --debug trace line, so
// tests and log scrapers can recognise them.
const (
	DebugReqPrefix  = "[k8s-debug] > "
	DebugRespPrefix = "[k8s-debug] < "
)

// debugInvocation emits the binary name and the redacted argv about to run.
// Manifests travel on stdin, so argv holds no secrets, but the trace is
// scrubbed anyway to match the other engines.
func debugInvocation(binary string, args []string) {
	_, _ = fmt.Fprintf(debugW, "%s%s %s\n", DebugReqPrefix, binary, scrub(strings.Join(args, " ")))
}

// debugResult emits the exit code, the elapsed time and the child's
// scrubbed stderr. The engine captures each child's stderr, so a child
// never writes to the operator's stderr directly. Stdout is never echoed:
// a read can return a whole CRD manifest.
func debugResult(binary string, res runResult, elapsed time.Duration) {
	code := -1
	var exitErr *exec.ExitError
	if errors.As(res.err, &exitErr) {
		code = exitErr.ExitCode()
	} else if res.err == nil {
		code = 0
	}
	_, _ = fmt.Fprintf(debugW, "%sexit %d elapsed %s\n", DebugRespPrefix, code, elapsed)
	if res.stderr != "" {
		_, _ = fmt.Fprintf(debugW, "%sstderr %s\n", DebugRespPrefix, scrub(strings.TrimSpace(res.stderr)))
	}
}

// scrub redacts secrets and then strips control bytes, the same order the
// aura and docker engines use.
func scrub(s string) string {
	return output.StripControl(clievents.RedactText(s))
}
