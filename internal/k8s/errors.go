// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// stderr reason markers. kubectl exits 1 for almost every failure, so the
// classification reads the reason kubectl prints in parentheses, such as
// `Error from server (NotFound)`. Server-side apply conflicts carry no
// parentheses: kubectl prints `Apply failed with 1 conflict: conflict with
// "<manager>": <field>`. All markers were recorded against kubectl 1.35 on
// a kind cluster. They are not a contract. Where the distinction matters to
// behaviour, the caller must probe with a get first rather than trust
// stderr text.
var (
	markerNotFound      = "(NotFound)"
	markerAlreadyExists = "(AlreadyExists)"
	markerConflict      = "(Conflict)"
	markerSSAConflict   = "Apply failed with"
	markerForbidden     = "(Forbidden)"
	markerUnauthorized  = "(Unauthorized)"
)

// connectionMarkers identify a child failure that means "the API server did
// not answer". These become NewUpstreamError so the exit code and the
// envelope say "upstream unavailable", not "CLI bug".
var connectionMarkers = []string{
	"connection refused",
	"was refused",
	"connection to the server",
	"no such host",
	"dial tcp",
	"i/o timeout",
	"unable to connect to the server",
	"context deadline exceeded",
}

// classifyRunError maps one child-process failure to a *clierr.CLIError. A
// cancelled context returns the context error unchanged, so a Ctrl-C maps
// to the interrupt exit code instead of an upstream error.
func classifyRunError(ctx context.Context, binary string, args []string, res runResult) error {
	if res.err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var exitErr *exec.ExitError
	if !errors.As(res.err, &exitErr) {
		// The process never ran (bad path, permission). Report it as an
		// upstream failure with the raw error.
		return clierr.NewUpstreamError("%s %s: %s", binary, strings.Join(args, " "), res.err.Error())
	}

	// Classify on the raw stderr, but only ever embed the scrubbed text: an
	// exec credential plugin can echo a token into kubectl's error.
	raw := strings.TrimSpace(res.stderr)
	lower := strings.ToLower(raw)
	msg := scrub(raw)
	switch {
	case strings.Contains(raw, markerNotFound):
		return clierr.NewNotFoundError("%s: %s", binary, msg)
	case strings.Contains(raw, markerAlreadyExists), strings.Contains(raw, markerConflict), strings.Contains(raw, markerSSAConflict):
		return clierr.NewConflictError("%s: %s", binary, msg)
	case strings.Contains(raw, markerForbidden), strings.Contains(raw, markerUnauthorized):
		return clierr.NewAuthError("%s: %s", binary, msg)
	}
	for _, marker := range connectionMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return clierr.NewUpstreamError("%s: %s", binary, msg)
		}
	}

	// Unknown shape: keep the child's own stderr, scrubbed, so the operator
	// sees the real cause. The argv echo is scrubbed too, for the same
	// reason as the docker engine (a third-party wrapper can echo argv).
	return clierr.NewFatalError("%s %s: %s", binary, scrub(strings.Join(args, " ")), msg)
}
