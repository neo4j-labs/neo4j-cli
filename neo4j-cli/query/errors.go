// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"errors"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"

	"github.com/neo4j/cli/common/clierr"
)

// categorizeBoltError wraps a Bolt-driver error into a typed *CLIError so the
// top-level main can map it to a process exit code via errors.As. Cypher
// ClientError-class failures (Neo.ClientError.*) surface as validation errors
// (exit 6); transport / TransientError / DatabaseError failures surface as
// upstream errors (exit 8). The original error is preserved on CLIError.Err so
// errors.As / errors.Is continue to find inner sentinels.
//
// One transport-shaped failure is reclassified as a usage error: when the
// driver reports `server responded HTTP` (the user pointed Bolt at the HTTP
// port, e.g. 7474 or a `:443` ingress), the error is user-input
// misconfiguration, not a transient blip. Surface it as a UsageError with a
// hint pointing at the Bolt port (7687) and the `https://` rewrite path.
//
// The function inspects the error in two passes: first via errors.As on the
// concrete *neo4j.Neo4jError type (real driver path), then by string-prefix
// match on the error message (tests inject plain `errors.New("Neo.ClientError…")`
// without constructing a real Neo4jError). A nil error is returned unchanged.
func categorizeBoltError(err error) error {
	if err == nil {
		return nil
	}

	// Already typed — leave it alone so callers can wrap once at the boundary
	// without double-classifying.
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		return err
	}

	// Wrong-port-family detection: the driver emits a stable
	// `server responded HTTP` message when it talks Bolt at an HTTP endpoint
	// (port 7474, 443, or any other HTTP listener). This is user-input
	// misconfiguration, so reclassify as a UsageError with an actionable hint
	// before the generic Neo4jError / transport branches below run.
	if strings.Contains(err.Error(), "server responded HTTP") {
		return clierr.NewUsageError(
			"server responded HTTP at the Bolt endpoint: the URI points at an HTTP listener, "+
				"not a Bolt one. Use the Bolt port (default 7687) on the same host, or pass an "+
				"https://... URI so it is rewritten to neo4j+s://<host>:7687 automatically. "+
				"(driver: %w)", err)
	}

	// Real driver error path. Neo4jError.Classification() returns
	// "ClientError" / "TransientError" / "DatabaseError" derived from the
	// dotted code; ClientError is a validation rejection, the rest are
	// upstream/transport failures the user can retry.
	var ne *neo4j.Neo4jError
	if errors.As(err, &ne) {
		if ne.Classification() == "ClientError" {
			return validationFrom(err)
		}
		return upstreamFrom(err)
	}

	// Plain-text path (tests + any future error that just embeds the code in
	// its message). Inspect the prefix to decide; everything else is treated
	// as an upstream/transport failure.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "Neo.ClientError."):
		return validationFrom(err)
	case strings.Contains(msg, "Neo.TransientError."),
		strings.Contains(msg, "Neo.DatabaseError."):
		return upstreamFrom(err)
	}

	return upstreamFrom(err)
}

// validationFrom returns a NewValidationError that preserves the underlying
// error chain via %w so errors.As still reaches the original driver error.
func validationFrom(err error) error {
	return clierr.NewValidationError("%w", err)
}

// upstreamFrom returns a NewUpstreamError that preserves the underlying error
// chain via %w so errors.As still reaches the original driver error.
func upstreamFrom(err error) error {
	return clierr.NewUpstreamError("%w", err)
}
