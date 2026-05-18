// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clierr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestConstructors_CodeNameAndRetryable(t *testing.T) {
	tests := []struct {
		name          string
		err           *CLIError
		wantCode      int
		wantCodeName  string
		wantRetryable bool
		wantMsg       string
	}{
		{"fatal", NewFatalError("boom %s", "fatal"), 1, "fatal_error", false, "boom fatal"},
		{"usage", NewUsageError("bad %s", "flag"), 2, "usage_error", false, "bad flag"},
		{"not_found", NewNotFoundError("no %s", "instance"), 3, "not_found", false, "no instance"},
		{"auth", NewAuthError("auth %d", 401), 4, "auth_error", false, "auth 401"},
		{"conflict", NewConflictError("conflict %s", "x"), 5, "conflict", false, "conflict x"},
		{"validation", NewValidationError("bad %s", "body"), 6, "validation_error", false, "bad body"},
		{"rate_limited", NewRateLimitError("30", "limited %s", "wait"), 7, "rate_limited", true, "limited wait"},
		{"upstream", NewUpstreamError("upstream %d", 503), 8, "upstream_error", true, "upstream 503"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ce *CLIError
			// Round-trip through errors.As to verify the constructor return
			// is still assignable to error via interface implicitness.
			var asError error = tc.err
			if !errors.As(asError, &ce) {
				t.Fatalf("errors.As did not extract *CLIError from %T", asError)
			}
			if ce.Code != tc.wantCode {
				t.Errorf("Code = %d, want %d", ce.Code, tc.wantCode)
			}
			if ce.CodeName != tc.wantCodeName {
				t.Errorf("CodeName = %q, want %q", ce.CodeName, tc.wantCodeName)
			}
			if ce.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, want %v", ce.Retryable, tc.wantRetryable)
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
	if !errors.As(error(err), &ce) {
		t.Fatalf("errors.As did not extract *CLIError")
	}
	if ce.RetryAfter != "30" {
		t.Errorf("RetryAfter = %q, want %q", ce.RetryAfter, "30")
	}
	if !ce.Retryable {
		t.Errorf("Retryable = false, want true for rate_limited")
	}
	if !strings.Contains(ce.Error(), "30") {
		t.Errorf("message body should mention 30; got %q", ce.Error())
	}
}

func TestErrorsAs_ThroughWrapping(t *testing.T) {
	inner := NewNotFoundError("no instance %q", "abc")

	wrappedOnce := fmt.Errorf("get instance: %w", error(inner))
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

	if !errors.Is(error(err), sentinel) {
		t.Errorf("errors.Is should reach sentinel through CLIError.Unwrap")
	}

	var ce *CLIError
	if !errors.As(error(err), &ce) {
		t.Fatalf("errors.As did not extract *CLIError")
	}
	if ce.Unwrap() == nil {
		t.Errorf("Unwrap() should return the wrapped sentinel, got nil")
	}
}

func TestWithResource_MutatesAndReturnsReceiver(t *testing.T) {
	err := NewNotFoundError("missing")
	got := err.WithResource("instance", "abc-123")
	if got != err {
		t.Errorf("WithResource should return the same receiver pointer")
	}
	if err.ResourceType != "instance" {
		t.Errorf("ResourceType = %q, want %q", err.ResourceType, "instance")
	}
	if err.ResourceID != "abc-123" {
		t.Errorf("ResourceID = %q, want %q", err.ResourceID, "abc-123")
	}

	// Clearing semantics: pass empty strings to wipe.
	_ = err.WithResource("", "")
	if err.ResourceType != "" || err.ResourceID != "" {
		t.Errorf("WithResource('','') should clear; got %q/%q", err.ResourceType, err.ResourceID)
	}
}

func TestWithSuggestion_MutatesAndReturnsReceiver(t *testing.T) {
	err := NewUsageError("bad flag")
	got := err.WithSuggestion("try --help")
	if got != err {
		t.Errorf("WithSuggestion should return the same receiver pointer")
	}
	if err.Suggestion != "try --help" {
		t.Errorf("Suggestion = %q, want %q", err.Suggestion, "try --help")
	}
}

func TestChain_WithResource_WithSuggestion(t *testing.T) {
	err := NewNotFoundError("missing %s", "thing").
		WithResource("instance", "abc").
		WithSuggestion("run `aura instance list`")
	if err.Code != 3 || err.CodeName != "not_found" {
		t.Errorf("chain dropped constructor fields: %+v", err)
	}
	if err.ResourceType != "instance" || err.ResourceID != "abc" {
		t.Errorf("chain dropped resource fields: %+v", err)
	}
	if err.Suggestion != "run `aura instance list`" {
		t.Errorf("chain dropped suggestion: %+v", err)
	}
}

func TestCodes_HasAllEightEntries(t *testing.T) {
	wantNames := map[int]string{
		1: "fatal_error",
		2: "usage_error",
		3: "not_found",
		4: "auth_error",
		5: "conflict",
		6: "validation_error",
		7: "rate_limited",
		8: "upstream_error",
	}
	if len(Codes) != len(wantNames) {
		t.Fatalf("Codes has %d entries, want %d", len(Codes), len(wantNames))
	}
	for code, wantName := range wantNames {
		meta, ok := Codes[code]
		if !ok {
			t.Errorf("Codes[%d] missing", code)
			continue
		}
		if meta.Name != wantName {
			t.Errorf("Codes[%d].Name = %q, want %q", code, meta.Name, wantName)
		}
		if meta.Description == "" {
			t.Errorf("Codes[%d].Description is empty", code)
		}
		if CodeNames[code] != meta.Name {
			t.Errorf("CodeNames[%d] = %q, want %q", code, CodeNames[code], meta.Name)
		}
		if CodeDescriptions[code] != meta.Description {
			t.Errorf("CodeDescriptions[%d] mismatch", code)
		}
	}

	// Retryable invariant: only 7 and 8 are retryable.
	for code, meta := range Codes {
		wantRetryable := code == 7 || code == 8
		if meta.Retryable != wantRetryable {
			t.Errorf("Codes[%d].Retryable = %v, want %v", code, meta.Retryable, wantRetryable)
		}
	}
}
