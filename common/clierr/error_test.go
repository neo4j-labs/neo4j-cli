// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clierr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestConstructors_ExitCodesAndMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{"fatal", NewFatalError("boom %s", "fatal"), 1, "boom fatal"},
		{"usage", NewUsageError("bad %s", "flag"), 2, "bad flag"},
		{"not_found", NewNotFoundError("no %s", "instance"), 3, "no instance"},
		{"auth", NewAuthError("auth %d", 401), 4, "auth 401"},
		{"conflict", NewConflictError("conflict %s", "x"), 5, "conflict x"},
		{"validation", NewValidationError("bad %s", "body"), 6, "bad body"},
		{"rate_limited", NewRateLimitError("30", "limited %s", "wait"), 7, "limited wait"},
		{"upstream", NewUpstreamError("upstream %d", 503), 8, "upstream 503"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ce *CLIError
			if !errors.As(tc.err, &ce) {
				t.Fatalf("errors.As did not extract *CLIError from %T", tc.err)
			}
			if ce.Code != tc.wantCode {
				t.Errorf("Code = %d, want %d", ce.Code, tc.wantCode)
			}
			if ce.Error() != tc.wantMsg {
				t.Errorf("Error() = %q, want %q", ce.Error(), tc.wantMsg)
			}
		})
	}
}

func TestRateLimitError_RetryAfterFieldAndMessage(t *testing.T) {
	err := NewRateLimitError("30", "rate limited, retry after %s seconds", "30")
	var ce *CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("errors.As did not extract *CLIError")
	}
	if ce.RetryAfter != "30" {
		t.Errorf("RetryAfter = %q, want %q", ce.RetryAfter, "30")
	}
	if !strings.Contains(ce.Error(), "30") {
		t.Errorf("message body should mention 30; got %q", ce.Error())
	}
}

func TestErrorsAs_ThroughWrapping(t *testing.T) {
	inner := NewNotFoundError("no instance %q", "abc")

	wrappedOnce := fmt.Errorf("get instance: %w", inner)
	wrappedTwice := fmt.Errorf("aura call failed: %w", wrappedOnce)

	var ce *CLIError
	if !errors.As(wrappedTwice, &ce) {
		t.Fatalf("errors.As should find *CLIError through two layers of %%w")
	}
	if ce.Code != 3 {
		t.Errorf("Code = %d, want 3", ce.Code)
	}
}

func TestUnwrap_PreservesWrappedChainFromConstructor(t *testing.T) {
	sentinel := errors.New("sentinel")
	err := NewUpstreamError("call failed: %w", sentinel)

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is should reach sentinel through CLIError.Unwrap")
	}

	var ce *CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("errors.As did not extract *CLIError")
	}
	if ce.Unwrap() == nil {
		t.Errorf("Unwrap() should return the wrapped sentinel, got nil")
	}
}
