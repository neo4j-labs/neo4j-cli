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
	// markerExecCredential identifies a failed exec credential plugin
	// (`getting credentials: exec: executable <name> failed …`). kubectl
	// prefixes it with "Unable to connect to the server", so it must be
	// classified before the connection markers. The text was recorded on
	// GKE, where an expired session also prints `Reauthentication failed.
	// cannot prompt during non-interactive execution`. kubectl does not
	// promise this wording, so a future release may need a new marker.
	markerExecCredential = "getting credentials: exec"
)

// Helm has no machine-readable not-found reason. `helm uninstall` of a
// missing release prints `Error: uninstall: Release not loaded: <name>:
// release: not found`, and `helm upgrade` of one prints `Error: UPGRADE
// FAILED: "<name>" has no deployed releases`. Both were recorded against
// helm 4.1.1 and are not a contract.
var helmReleaseNotFoundMarkers = []string{
	"release: not found",
	"Release not loaded",
	"has no deployed releases",
}

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

// IsNotFound reports whether err is the engine's not-found classification,
// so a caller can tell "the object is absent" from a real failure.
func IsNotFound(err error) bool {
	var cliErr *clierr.CLIError
	return errors.As(err, &cliErr) && cliErr.CodeName == "not_found"
}

// IsCloudLoginFailure reports whether err came from a failed exec credential
// plugin. Callers use it to tell "sign in to your cloud provider again" from
// an ordinary auth or upstream failure. A plain `(Unauthorized)` from the API
// server is not a cloud-login failure.
func IsCloudLoginFailure(err error) bool {
	var cliErr *clierr.CLIError
	return errors.As(err, &cliErr) && cliErr.CodeName == "auth_error" &&
		strings.Contains(cliErr.Message, markerExecCredential)
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
	case strings.Contains(raw, markerExecCredential):
		// The plugin cannot prompt because stdin is not attached, so an
		// expired session fails even though the cluster itself is healthy.
		return clierr.NewAuthError("%s: %s", binary, msg).
			WithSuggestion("Sign in to your cloud provider again (for example gcloud auth login, aws sso login or az login), then retry")
	}
	for _, marker := range helmReleaseNotFoundMarkers {
		if strings.Contains(raw, marker) {
			return clierr.NewNotFoundError("%s: %s", binary, msg)
		}
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
