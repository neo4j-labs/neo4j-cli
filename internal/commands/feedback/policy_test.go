// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type corpusCase struct {
	ID       string `json:"id"`
	Field    string `json:"field"`
	Text     string `json:"text"`
	Expect   string `json:"expect"`
	Detector string `json:"detector"`
	Category string `json:"category"`
}

func loadCorpus(t *testing.T, name string) []corpusCase {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var out []corpusCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c corpusCase
		require.NoError(t, json.Unmarshal(sc.Bytes(), &c))
		out = append(out, c)
	}
	require.NoError(t, sc.Err())
	return out
}

const (
	baseExpected = "The command should explain what went wrong and how to fix it."
	baseObserved = "The command failed with a message that did not name a next step."
)

func baseInput() Input {
	return Input{Category: "unexpected_behavior", Expected: baseExpected, Observed: baseObserved, CLIVersion: "test"}
}

func inputFor(c corpusCase) Input {
	in := baseInput()
	if c.Category != "" {
		in.Category = c.Category
	}
	switch c.Field {
	case "expected":
		in.Expected = c.Text
	case "observed":
		in.Observed = c.Text
	}
	return in
}

func detectors(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Detector)
	}
	return out
}

func TestValidate_BaseInputPasses(t *testing.T) {
	_, vs, dropped := Validate(baseInput(), testRoot())
	assert.Empty(t, vs)
	assert.Empty(t, dropped)
}

func TestValidate_ProhibitedCorpus(t *testing.T) {
	cases := loadCorpus(t, "prohibited_corpus.jsonl")
	require.Len(t, cases, 38)
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			_, vs, _ := Validate(inputFor(c), testRoot())
			require.NotEmpty(t, vs, "expected rejection for %q", c.Text)
			assert.Contains(t, detectors(vs), c.Detector, "violations: %v", vs)
			for _, v := range vs {
				assert.Equal(t, c.Field, v.Field)
			}
		})
	}
}

func TestValidate_AllowedCorpus(t *testing.T) {
	cases := loadCorpus(t, "allowed_corpus.jsonl")
	require.Len(t, cases, 29)
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			_, vs, _ := Validate(inputFor(c), testRoot())
			assert.Empty(t, vs, "text %q rejected: %v", c.Text, vs)
		})
	}
}

func TestValidate_FieldRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Input)
		want   Violation
	}{
		{"bad category", func(in *Input) { in.Category = "praise" }, Violation{"category", "invalid_input"}},
		{"empty category", func(in *Input) { in.Category = "" }, Violation{"category", "invalid_input"}},
		{"blank expected", func(in *Input) { in.Expected = " \t " }, Violation{"expected", "invalid_input"}},
		{"empty observed", func(in *Input) { in.Observed = "" }, Violation{"observed", "invalid_input"}},
		{"too large", func(in *Input) { in.Observed = strings.Repeat("a", 17000) }, Violation{"payload", "too_large"}},
		{"bad request id", func(in *Input) { in.RequestID = "not-a-uuid" }, Violation{"request_id", "invalid_input"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			tc.mutate(&in)
			_, vs, _ := Validate(in, testRoot())
			assert.Contains(t, vs, tc.want)
		})
	}
}

// expected and observed are capped at 255 bytes. Text over the cap is rejected with too_long, never truncated;
// the 16 KB payload cap stays as a separate guard.
func TestValidate_LongFieldIsTooLongNotTruncated(t *testing.T) {
	in := baseInput()
	in.Observed = strings.Repeat("The output was unclear. ", 400) // ~9.6 KB, no detector hits
	p, vs, _ := Validate(in, testRoot())
	assert.Equal(t, []Violation{{Field: fieldObserved, Detector: detTooLong}}, vs)
	assert.Equal(t, in.Observed, p.Observed, "the validator never truncates")
}

func TestValidate_RequestID(t *testing.T) {
	p, vs, _ := Validate(baseInput(), testRoot())
	assert.Empty(t, vs)
	assert.Regexp(t, uuidV4, p.RequestID)

	in := baseInput()
	in.RequestID = "11111111-1111-4111-8111-111111111111"
	p, vs, _ = Validate(in, testRoot())
	assert.Empty(t, vs)
	assert.Equal(t, in.RequestID, p.RequestID)

	// "zq" can never appear in a generated id (hex digits only), so this cannot
	// match by chance.
	in.RequestID = "zq value with <text>"
	p, _, _ = Validate(in, testRoot())
	assert.NotContains(t, p.RequestID, "zq", "invalid request id must not be echoed")
	assert.Regexp(t, uuidV4, p.RequestID, "a fresh id is used instead")
}

func TestValidate_OptionalFields(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mutate      func(*Input)
		wantCommand string
		wantCode    string
		wantModel   string
		wantHarness string
		wantDropped []string
	}{
		{"all valid", func(in *Input) {
			in.Command, in.ErrorCode, in.AgentModel, in.Harness = "query", "usage_error", "claude-opus-5-5", "claude-code"
		}, "query", "usage_error", "claude-opus-5-5", "claude-code", nil},
		{"nested command", func(in *Input) { in.Command = "credential dbms add" }, "credential dbms add", "", "", "", nil},
		{"command with flags dropped", func(in *Input) { in.Command = "query --uri x" }, "", "", "", "", []string{"command_id"}},
		{"hidden command dropped", func(in *Input) { in.Command = "secret-thing" }, "", "", "", "", []string{"command_id"}},
		{"feedback subtree dropped", func(in *Input) { in.Command = "feedback submit" }, "", "", "", "", []string{"command_id"}},
		{"bad error code dropped", func(in *Input) { in.ErrorCode = "oops" }, "", "", "", "", []string{"error_code"}},
		{"model not in family", func(in *Input) { in.AgentModel = "MyCustomBot" }, "", "", "", "", []string{"agent.model"}},
		{"model with spaces", func(in *Input) { in.AgentModel = "claude zephyrine" }, "", "", "", "", []string{"agent.model"}},
		{"model too long", func(in *Input) { in.AgentModel = "claude-" + strings.Repeat("x", 40) }, "", "", "", "", []string{"agent.model"}},
		{"o-series model", func(in *Input) { in.AgentModel = "o3-mini" }, "", "", "o3-mini", "", nil},
		{"harness only", func(in *Input) { in.Harness = "codex" }, "", "", "", "codex", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			tc.mutate(&in)
			p, vs, dropped := Validate(in, testRoot())
			assert.Empty(t, vs)
			assert.Equal(t, tc.wantDropped, dropped)
			assert.Equal(t, tc.wantCommand, p.CommandID)
			assert.Equal(t, tc.wantCode, p.ErrorCode)
			if tc.wantModel == "" && tc.wantHarness == "" {
				assert.Nil(t, p.Agent)
			} else {
				require.NotNil(t, p.Agent)
				assert.Equal(t, tc.wantModel, p.Agent.Model)
				assert.Equal(t, tc.wantHarness, p.Agent.Harness)
			}
		})
	}
}

func TestValidate_PayloadKeysAreAllowListed(t *testing.T) {
	in := baseInput()
	in.Command, in.ErrorCode, in.AgentModel, in.Harness = "query", "usage_error", "gpt-6-sol", "codex"
	p, vs, _ := Validate(in, testRoot())
	require.Empty(t, vs)
	b, err := json.Marshal(p)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	allowed := map[string]bool{"schema_version": true, "surface": true, "cli_version": true, "request_id": true,
		"category": true, "expected": true, "observed": true, "command_id": true, "error_code": true, "agent": true,
		"os": true, "os_arch": true, "invoker": true}
	for k := range m {
		assert.True(t, allowed[k], "unexpected key %q", k)
	}
	for k := range m["agent"].(map[string]any) {
		assert.Contains(t, []string{"harness", "model"}, k)
	}
}

func TestDetect_ExtraCases(t *testing.T) {
	for _, tc := range []struct {
		text string
		want string // detector expected; "" means none
	}{
		{"It took 10:30 to start.", ""},
		{"The CLI printed version 1.14.0 and exited.", ""},
		{"The IPv6 address ::1 was refused.", "ip_address"},
		{"Use ../data to find it.", "file_path"},
		{"Try ./run it.", "file_path"},
		{"Using node.js was fine.", ""},
		{"Setting OPTIONAL MATCH in prose is one keyword.", ""},
		{"I tried CALL then DELETE and it failed.", "cypher"},
	} {
		got := detect(tc.text)
		if tc.want == "" {
			assert.Empty(t, got, tc.text)
		} else {
			assert.Contains(t, got, tc.want, tc.text)
		}
	}
}

// Report text is shown in the consent dialog, so it must stay one plain line:
// a newline or a bidi mark could make it pose as other lines of the dialog,
// and a zero-width character could split a pattern the detectors look for.
func TestValidate_TextMustBeOnePlainLine(t *testing.T) {
	for name, text := range map[string]string{
		"newline":          "A CSV output format.\nobserved: nothing",
		"carriage return":  "A CSV output format.\rcategory: other_friction",
		"bidi override":    "A CSV output format.\u202e",
		"zero-width space": "An email like user@exa\u200bmple.com",
		"invalid UTF-8":    "A CSV output format.\xff",
	} {
		t.Run(name, func(t *testing.T) {
			in := baseInput()
			in.Expected = text
			_, vs, _ := Validate(in, testRoot())
			assert.Contains(t, vs, Violation{Field: fieldExpected, Detector: detControlChars})
		})
	}
}

func TestValidate_FieldByteCap(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		tooLong bool
	}{
		{"255 ASCII bytes", strings.Repeat("a", 255), false},
		{"256 ASCII bytes", strings.Repeat("a", 256), true},
		{"255 bytes, multibyte", strings.Repeat("é", 127) + "a", false}, // 254 + 1 bytes
		{"256 bytes, multibyte (128 chars)", strings.Repeat("é", 128), true},
		{"fewer than 255 chars but more than 255 bytes", strings.Repeat("é", 200), true},
	} {
		for _, field := range []string{fieldExpected, fieldObserved} {
			t.Run(tc.name+"/"+field, func(t *testing.T) {
				in := Input{Category: "unexpected_behavior", Expected: baseExpected, Observed: baseObserved}
				if field == fieldExpected {
					in.Expected = tc.text
				} else {
					in.Observed = tc.text
				}
				_, vs, _ := Validate(in, testRoot())
				got := false
				for _, v := range vs {
					if v.Field == field && v.Detector == detTooLong {
						got = true
					}
				}
				assert.Equal(t, tc.tooLong, got)
				if tc.tooLong {
					assert.Equal(t, reasonInvalidInput, rejectionReason(vs), "too_long alone is invalid_input")
				}
			})
		}
	}
}
