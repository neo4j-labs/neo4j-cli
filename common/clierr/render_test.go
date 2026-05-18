// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clierr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// TestBuildEnvelope_PureProjection asserts that BuildEnvelope produces the
// documented shape across every CodeName, including the omitempty behaviour
// for optional hint fields.
func TestBuildEnvelope_PureProjection(t *testing.T) {
	tests := []struct {
		name         string
		err          *CLIError
		wantCode     string
		wantExit     int
		wantRetry    bool
		wantMessage  string
		wantResType  string
		wantResID    string
		wantSuggest  string
		wantJSONHas  []string
		wantJSONOmit []string
	}{
		{
			name:        "fatal",
			err:         NewFatalError("boom"),
			wantCode:    "fatal_error",
			wantExit:    1,
			wantRetry:   false,
			wantMessage: "boom",
			wantJSONHas: []string{`"code":"fatal_error"`, `"exit_code":1`, `"retryable":false`},
			wantJSONOmit: []string{
				`"resource_type"`, `"resource_id"`, `"suggestion"`,
			},
		},
		{
			name:        "usage",
			err:         NewUsageError("bad flag"),
			wantCode:    "usage_error",
			wantExit:    2,
			wantRetry:   false,
			wantMessage: "bad flag",
		},
		{
			name:        "not_found_with_resource",
			err:         NewNotFoundError("missing").WithResource("instance", "abc-123"),
			wantCode:    "not_found",
			wantExit:    3,
			wantRetry:   false,
			wantMessage: "missing",
			wantResType: "instance",
			wantResID:   "abc-123",
			wantJSONHas: []string{`"resource_type":"instance"`, `"resource_id":"abc-123"`},
		},
		{
			name:        "auth",
			err:         NewAuthError("denied"),
			wantCode:    "auth_error",
			wantExit:    4,
			wantRetry:   false,
			wantMessage: "denied",
		},
		{
			name:        "conflict",
			err:         NewConflictError("conflict"),
			wantCode:    "conflict",
			wantExit:    5,
			wantRetry:   false,
			wantMessage: "conflict",
		},
		{
			name:        "validation_with_suggestion",
			err:         NewValidationError("bad body").WithSuggestion("see docs"),
			wantCode:    "validation_error",
			wantExit:    6,
			wantRetry:   false,
			wantMessage: "bad body",
			wantSuggest: "see docs",
			wantJSONHas: []string{`"suggestion":"see docs"`},
		},
		{
			name:        "rate_limited",
			err:         NewRateLimitError("30", "rate limited"),
			wantCode:    "rate_limited",
			wantExit:    7,
			wantRetry:   true,
			wantMessage: "rate limited",
			wantJSONHas: []string{`"retryable":true`},
		},
		{
			name:        "upstream",
			err:         NewUpstreamError("502"),
			wantCode:    "upstream_error",
			wantExit:    8,
			wantRetry:   true,
			wantMessage: "502",
			wantJSONHas: []string{`"retryable":true`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.err.BuildEnvelope()
			if env.Error.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", env.Error.Code, tc.wantCode)
			}
			if env.Error.ExitCode != tc.wantExit {
				t.Errorf("ExitCode = %d, want %d", env.Error.ExitCode, tc.wantExit)
			}
			if env.Error.Retryable != tc.wantRetry {
				t.Errorf("Retryable = %v, want %v", env.Error.Retryable, tc.wantRetry)
			}
			if env.Error.Message != tc.wantMessage {
				t.Errorf("Message = %q, want %q", env.Error.Message, tc.wantMessage)
			}
			if env.Error.ResourceType != tc.wantResType {
				t.Errorf("ResourceType = %q, want %q", env.Error.ResourceType, tc.wantResType)
			}
			if env.Error.ResourceID != tc.wantResID {
				t.Errorf("ResourceID = %q, want %q", env.Error.ResourceID, tc.wantResID)
			}
			if env.Error.Suggestion != tc.wantSuggest {
				t.Errorf("Suggestion = %q, want %q", env.Error.Suggestion, tc.wantSuggest)
			}

			buf, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			out := string(buf)
			for _, want := range tc.wantJSONHas {
				if !bytes.Contains(buf, []byte(want)) {
					t.Errorf("JSON output missing %q\nfull: %s", want, out)
				}
			}
			for _, omit := range tc.wantJSONOmit {
				if bytes.Contains(buf, []byte(omit)) {
					t.Errorf("JSON output should omit %q\nfull: %s", omit, out)
				}
			}
		})
	}
}

// TestRender_JSONMode covers every CodeName in JSON mode, asserting the
// stdout envelope is valid JSON with the documented fields and stderr
// contains the one-line summary including the exit code.
func TestRender_JSONMode(t *testing.T) {
	tests := []struct {
		name     string
		err      *CLIError
		wantCode string
		wantExit int
	}{
		{"fatal", NewFatalError("boom"), "fatal_error", 1},
		{"usage", NewUsageError("bad flag"), "usage_error", 2},
		{"not_found", NewNotFoundError("missing"), "not_found", 3},
		{"auth", NewAuthError("denied"), "auth_error", 4},
		{"conflict", NewConflictError("conflict"), "conflict", 5},
		{"validation", NewValidationError("bad body"), "validation_error", 6},
		{"rate_limited", NewRateLimitError("30", "rate limited"), "rate_limited", 7},
		{"upstream", NewUpstreamError("502"), "upstream_error", 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			Render(error(tc.err), &stdout, &stderr, "json")

			// stdout: valid JSON envelope with the right code/exit_code.
			var env Envelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not valid JSON: %v\ngot: %s", err, stdout.String())
			}
			if env.Error.Code != tc.wantCode {
				t.Errorf("envelope.code = %q, want %q", env.Error.Code, tc.wantCode)
			}
			if env.Error.ExitCode != tc.wantExit {
				t.Errorf("envelope.exit_code = %d, want %d", env.Error.ExitCode, tc.wantExit)
			}
			if !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
				t.Errorf("stdout should end with a newline; got %q", stdout.String())
			}

			// stderr: one-line `Error: <msg> (exit <N>)` summary.
			wantStderr := fmt.Sprintf("Error: %s (exit %d)\n", tc.err.Message, tc.wantExit)
			if stderr.String() != wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
			}
		})
	}
}

// TestRender_PlaintextMode covers every format other than "json"/"toon" —
// they all behave identically (one-line summary on stderr; optional
// suggestion on a second stderr line; stdout untouched).
func TestRender_PlaintextMode(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"empty", ""},
		{"default", "default"},
		{"table", "table"},
		{"unknown", "yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := NewUsageError("bad flag")
			var stdout, stderr bytes.Buffer
			Render(error(err), &stdout, &stderr, tc.format)

			if stdout.Len() != 0 {
				t.Errorf("stdout should be empty in plaintext mode; got %q", stdout.String())
			}
			want := "Error: bad flag (exit 2)\n"
			if stderr.String() != want {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
		})
	}
}

func TestRender_PlaintextMode_WithSuggestion(t *testing.T) {
	err := NewUsageError("bad flag").WithSuggestion("try --help")
	var stdout, stderr bytes.Buffer
	Render(error(err), &stdout, &stderr, "")

	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty; got %q", stdout.String())
	}
	want := "Error: bad flag (exit 2)\ntry --help\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRender_JSONMode_WithSuggestion_StderrSummaryHasNoSuggestion(t *testing.T) {
	// In JSON mode the suggestion belongs in the envelope on stdout — the
	// stderr summary stays a single line so scripts grepping stderr aren't
	// surprised by a second line that's actually present in stdout.
	err := NewUsageError("bad flag").WithSuggestion("try --help")
	var stdout, stderr bytes.Buffer
	Render(error(err), &stdout, &stderr, "json")

	wantStderr := "Error: bad flag (exit 2)\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}

	var env Envelope
	if e := json.Unmarshal(stdout.Bytes(), &env); e != nil {
		t.Fatalf("stdout invalid JSON: %v", e)
	}
	if env.Error.Suggestion != "try --help" {
		t.Errorf("envelope.suggestion = %q, want %q", env.Error.Suggestion, "try --help")
	}
}

func TestRender_UntypedError_WrappedAsFatal_JSON(t *testing.T) {
	plain := errors.New("kaboom")
	var stdout, stderr bytes.Buffer
	Render(plain, &stdout, &stderr, "json")

	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\ngot: %s", err, stdout.String())
	}
	if env.Error.Code != "fatal_error" {
		t.Errorf("envelope.code = %q, want fatal_error", env.Error.Code)
	}
	if env.Error.ExitCode != 1 {
		t.Errorf("envelope.exit_code = %d, want 1", env.Error.ExitCode)
	}
	if env.Error.Message != "kaboom" {
		t.Errorf("envelope.message = %q, want kaboom", env.Error.Message)
	}
	if env.Error.Retryable {
		t.Errorf("fatal_error should not be retryable")
	}

	wantStderr := "Error: kaboom (exit 1)\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}
}

func TestRender_UntypedError_WrappedAsFatal_Plaintext(t *testing.T) {
	plain := errors.New("kaboom")
	var stdout, stderr bytes.Buffer
	Render(plain, &stdout, &stderr, "")

	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty; got %q", stdout.String())
	}
	want := "Error: kaboom (exit 1)\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRender_NilError_IsNoop(t *testing.T) {
	var stdout, stderr bytes.Buffer
	Render(nil, &stdout, &stderr, "json")
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("nil err should produce no output; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	Render(nil, &stdout, &stderr, "")
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("nil err (plaintext) should produce no output; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// TestRender_ErrorsAsExtraction asserts Render finds a *CLIError wrapped via
// %w through fmt.Errorf — the chain semantics promised by the constructors.
func TestRender_ErrorsAsExtraction(t *testing.T) {
	inner := NewNotFoundError("missing %q", "abc").WithResource("instance", "abc")
	wrapped := fmt.Errorf("aura call: %w", error(inner))

	var stdout, stderr bytes.Buffer
	Render(wrapped, &stdout, &stderr, "json")

	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
	if env.Error.Code != "not_found" {
		t.Errorf("envelope.code = %q, want not_found", env.Error.Code)
	}
	if env.Error.ResourceType != "instance" {
		t.Errorf("envelope.resource_type = %q, want instance", env.Error.ResourceType)
	}
	if env.Error.ResourceID != "abc" {
		t.Errorf("envelope.resource_id = %q, want abc", env.Error.ResourceID)
	}
}

// TestRender_ToonMode covers --format=toon. The stdout payload must be the
// envelope encoded as TOON (non-empty, distinct from the JSON encoding, and
// containing the expected code/exit_code substrings); the stderr payload
// must be the same one-line summary the JSON path emits.
func TestRender_ToonMode(t *testing.T) {
	tests := []struct {
		name     string
		err      *CLIError
		wantCode string
		wantExit int
	}{
		{"usage", NewUsageError("bad flag"), "usage_error", 2},
		{"not_found", NewNotFoundError("missing").WithResource("instance", "abc-123"), "not_found", 3},
		{"rate_limited", NewRateLimitError("30", "rate limited"), "rate_limited", 7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			Render(error(tc.err), &stdout, &stderr, "toon")

			if stdout.Len() == 0 {
				t.Fatalf("stdout should be non-empty in toon mode")
			}
			if !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
				t.Errorf("stdout should end with a newline; got %q", stdout.String())
			}

			// TOON output should NOT be byte-identical to the JSON form —
			// confirms we're going down the toon-marshaller branch, not
			// accidentally re-using json.Marshal.
			jsonBuf, err := json.Marshal(tc.err.BuildEnvelope())
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if bytes.Equal(bytes.TrimRight(stdout.Bytes(), "\n"), jsonBuf) {
				t.Errorf("toon stdout matches JSON encoding byte-for-byte; want distinct encoding")
			}

			// TOON is human-readable and includes the field names + scalar
			// values literally — substring assertions are robust to layout
			// changes in the encoder.
			out := stdout.String()
			if !bytes.Contains(stdout.Bytes(), []byte(tc.wantCode)) {
				t.Errorf("toon stdout missing code %q\nfull: %s", tc.wantCode, out)
			}
			if !bytes.Contains(stdout.Bytes(), []byte(fmt.Sprintf("%d", tc.wantExit))) {
				t.Errorf("toon stdout missing exit_code %d\nfull: %s", tc.wantExit, out)
			}

			// stderr: one-line `Error: <msg> (exit <N>)` summary.
			wantStderr := fmt.Sprintf("Error: %s (exit %d)\n", tc.err.Message, tc.wantExit)
			if stderr.String() != wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
			}
		})
	}
}

// TestRender_UntypedError_WrappedAsFatal_Toon asserts the untyped-error
// fallback fires in toon mode too — wraps to fatal_error / exit 1.
func TestRender_UntypedError_WrappedAsFatal_Toon(t *testing.T) {
	plain := errors.New("kaboom")
	var stdout, stderr bytes.Buffer
	Render(plain, &stdout, &stderr, "toon")

	if stdout.Len() == 0 {
		t.Fatalf("stdout should be non-empty in toon mode for untyped err")
	}
	if !bytes.Contains(stdout.Bytes(), []byte("fatal_error")) {
		t.Errorf("toon stdout missing fatal_error\nfull: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("kaboom")) {
		t.Errorf("toon stdout missing message\nfull: %s", stdout.String())
	}

	wantStderr := "Error: kaboom (exit 1)\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}
}

// TestRender_ToonMode_WithSuggestion_StderrSummaryHasNoSuggestion asserts
// that in toon mode (like JSON mode) the suggestion is carried in the
// stdout envelope, not duplicated on stderr.
func TestRender_ToonMode_WithSuggestion_StderrSummaryHasNoSuggestion(t *testing.T) {
	err := NewUsageError("bad flag").WithSuggestion("try --help")
	var stdout, stderr bytes.Buffer
	Render(error(err), &stdout, &stderr, "toon")

	wantStderr := "Error: bad flag (exit 2)\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("try --help")) {
		t.Errorf("toon stdout missing suggestion\nfull: %s", stdout.String())
	}
}

// TestBuildEnvelope_NoIO documents that BuildEnvelope is a pure projection —
// it's exercised here without any writer to prove the contract.
func TestBuildEnvelope_NoIO(t *testing.T) {
	err := NewUpstreamError("boom").WithResource("instance", "id-1").WithSuggestion("retry")
	env := err.BuildEnvelope()
	if env.Error.Code != "upstream_error" {
		t.Errorf("Code = %q, want upstream_error", env.Error.Code)
	}
	if env.Error.ResourceType != "instance" || env.Error.ResourceID != "id-1" {
		t.Errorf("resource fields lost: %+v", env)
	}
	if env.Error.Suggestion != "retry" {
		t.Errorf("Suggestion = %q, want retry", env.Error.Suggestion)
	}
	if !env.Error.Retryable {
		t.Errorf("upstream should be retryable")
	}
}
