// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedactPlanArguments_MarshalledJSONRedactsDetails proves the EXPLAIN shape
// end-to-end: a fake Plan whose Arguments carries a secret embedded literally
// in a Cypher predicate (which the planner echoes under "Details") marshals to
// JSON with that secret scrubbed to "***", while non-secret values survive
// verbatim with no type coercion.
func TestRedactPlanArguments_MarshalledJSONRedactsDetails(t *testing.T) {
	node := planNodeFromPlan(fakePlan{
		operator: "Filter",
		arguments: map[string]any{
			"Details":       "n.password = 'hunter2'",
			"EstimatedRows": 11.0,
			"planner":       "COST",
		},
	})
	require.NotNil(t, node)

	b, err := json.Marshal(node)
	require.NoError(t, err)
	out := string(b)
	assert.NotContains(t, out, "hunter2", "the literal secret must never reach serialized output")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))
	args, ok := decoded["arguments"].(map[string]any)
	require.True(t, ok, "arguments must survive marshalling: %s", out)

	details, ok := args["Details"].(string)
	require.True(t, ok)
	assert.Contains(t, details, "***", "the secret value must be replaced by the redaction placeholder")
	assert.Equal(t, float64(11), args["EstimatedRows"], "the non-string float must pass through unchanged")
	assert.Equal(t, "COST", args["planner"], "non-secret prose must be left intact")
}

// TestRedactPlanArguments_ScrubsNestedAggregates proves the recursion: string
// leaves embedded inside map[string]any and []any (at any depth) are scrubbed
// while structure and non-secret leaves survive.
func TestRedactPlanArguments_ScrubsNestedAggregates(t *testing.T) {
	node := planNodeFromPlan(fakePlan{
		operator: "Filter",
		arguments: map[string]any{
			"ListArg": []any{
				"token = abc123",
				"plain",
				map[string]any{"inner": "pwd = qz"},
				[]any{"token = xy", 1.5},
			},
			"MapArg": map[string]any{
				"nested": "token = abc123",
				"keep":   "plain text",
			},
		},
	})
	require.NotNil(t, node)

	b, err := json.Marshal(node)
	require.NoError(t, err)
	out := string(b)
	assert.NotContains(t, out, "abc123", "secret in a slice leaf must be scrubbed")
	assert.NotContains(t, out, "qz", "secret in a nested map leaf must be scrubbed")
	assert.NotContains(t, out, "xy", "secret in a nested slice leaf must be scrubbed")
	assert.Contains(t, out, "***")
	assert.Contains(t, out, "plain text", "non-secret leaves must survive")
}

// TestRedactPlanArguments_PreservesNonStringValues proves the helper itself
// keeps non-string plan values with their exact Go types (no stringify, no type
// coercion) and leaves non-secret strings untouched.
func TestRedactPlanArguments_PreservesNonStringValues(t *testing.T) {
	got := redactPlanArguments(map[string]any{
		"EstimatedRows": 11.0,
		"count":         5,
		"flag":          true,
		"nilVal":        nil,
		"plain":         "not a secret",
	})
	require.NotNil(t, got)
	assert.Equal(t, 11.0, got["EstimatedRows"])
	assert.Equal(t, 5, got["count"])
	assert.Equal(t, true, got["flag"])
	assert.Nil(t, got["nilVal"])
	assert.Equal(t, "not a secret", got["plain"])
}

// TestRedactPlanArguments_DoesNotMutateInput proves the helper copies rather
// than redacts in place, so the driver's Arguments map value stays raw.
func TestRedactPlanArguments_DoesNotMutateInput(t *testing.T) {
	src := map[string]any{"Details": "p.password = 'hunter2'"}
	got := redactPlanArguments(src)
	require.NotNil(t, got)
	assert.Equal(t, "p.password = 'hunter2'", src["Details"], "the input map must not be mutated")
	assert.Equal(t, "p.password = ***", got["Details"], "the copy must carry the scrubbed value")
}

// TestRedactPlanArguments_ProfileRoute proves the PROFILE shape routes its
// Arguments through the same redaction as the EXPLAIN shape.
func TestRedactPlanArguments_ProfileRoute(t *testing.T) {
	node := planNodeFromProfile(fakeProfiledPlan{
		operator:  "NodeByLabelScan",
		arguments: map[string]any{"Details": "secret = hunter2"},
		records:   1,
	})
	require.NotNil(t, node)

	b, err := json.Marshal(node)
	require.NoError(t, err)
	out := string(b)
	assert.NotContains(t, out, "hunter2")
	assert.Contains(t, out, "***")
}

// TestRedactPlanArguments_ScrubsStringSlices proves the shared gap both
// recursive walkers had — a []string Arguments value was passed through
// untouched — is closed at the single WalkStrings site: every string element,
// including a secret, is scrubbed.
func TestRedactPlanArguments_ScrubsStringSlices(t *testing.T) {
	node := planNodeFromPlan(fakePlan{
		operator: "Filter",
		arguments: map[string]any{
			"StringsArg": []string{"token = x", "plain"},
		},
	})
	require.NotNil(t, node)

	b, err := json.Marshal(node)
	require.NoError(t, err)
	out := string(b)
	assert.NotContains(t, out, "token = x", "the secret []string element must be scrubbed")
	assert.Contains(t, out, "***", "the scrub placeholder must replace the secret element")
	assert.Contains(t, out, "plain", "non-secret slice elements must survive")
}

// TestRedactPlanArguments_NeutralizesControlBytes proves the redaction pass
// applies the StripControl half of the Scrub combo itself, so server-supplied
// string leaves carrying terminal control bytes (ANSI escapes, NUL) are
// neutralized at the source instead of being left for the encoding/json escape
// or TOON's post-marshal backstop.
func TestRedactPlanArguments_NeutralizesControlBytes(t *testing.T) {
	t.Run("escape embedded after a secret is swallowed by the redaction", func(t *testing.T) {
		node := planNodeFromPlan(fakePlan{
			operator: "Filter",
			arguments: map[string]any{
				"Details": "n.password = 'hunter2'\x1b[2J",
			},
		})
		require.NotNil(t, node)

		b, err := json.Marshal(node)
		require.NoError(t, err)
		out := string(b)
		assert.NotContains(t, out, "hunter2", "the literal secret must never reach serialized output")
		assert.NotContains(t, out, "\x1b", "the raw ESC byte must not survive the redaction pass")
		assert.Contains(t, out, "***", "the secret value must be replaced by the redaction placeholder")
	})

	t.Run("escape in plain prose is stripped by the Scrub combo", func(t *testing.T) {
		// RedactText does not rewrite this leaf (no secret shape), so the ESC can
		// only be gone because the combo's StripControl half fired at the source.
		node := planNodeFromPlan(fakePlan{
			operator: "Filter",
			arguments: map[string]any{
				"planner\x00note": "prose\x1b[2J",
			},
		})
		require.NotNil(t, node)

		b, err := json.Marshal(node)
		require.NoError(t, err)
		out := string(b)
		assert.NotContains(t, out, "\x1b", "the raw ESC byte must be neutralized by StripControl")
		assert.NotContains(t, out, "\x00", "the raw NUL byte must be neutralized by StripControl")
		assert.Contains(t, out, "prose?", "the plain-text leaf must survive with the control byte replaced by ?")
	})
}

// TestRedactPlanArguments_StripsControlBytesFromMapKeys proves WalkStrings
// applies the scrub leaf to map keys too, preserving stripControlDeep's key
// behaviour now shared by the redaction pass.
func TestRedactPlanArguments_StripsControlBytesFromMapKeys(t *testing.T) {
	node := planNodeFromPlan(fakePlan{
		operator: "Filter",
		arguments: map[string]any{
			"Details\x1b[2J": "keep",
		},
	})
	require.NotNil(t, node)

	b, err := json.Marshal(node)
	require.NoError(t, err)
	out := string(b)
	assert.NotContains(t, out, "Details\x1b", "a control byte in an Arguments map key must be neutralized")
	assert.Contains(t, out, "Details?[2J", "the key must survive with the control byte replaced by ?")
	assert.Contains(t, out, "keep", "the key's value must survive")
}

// TestRedactPlanArguments_NilAndEmpty pins the degenerate inputs: a nil map
// stays nil and an empty map becomes an empty (non-nil) map without panicking.
func TestRedactPlanArguments_NilAndEmpty(t *testing.T) {
	assert.Nil(t, redactPlanArguments(nil))

	assert.NotPanics(t, func() {
		got := redactPlanArguments(map[string]any{})
		require.NotNil(t, got)
		assert.Empty(t, got)
	})
}

// TestRedactPlanArguments_OrdinaryPathUntouched is the regression guard for the
// no-op path: redaction must not add keys, mutate output, or introduce a plan
// side-channel on an ordinary read that has no plan to redact.
func TestRedactPlanArguments_OrdinaryPathUntouched(t *testing.T) {
	t.Run("nil arguments stays nil and omits the JSON key", func(t *testing.T) {
		plan := planNodeFromPlan(fakePlan{operator: "NodeByLabelScan"})
		require.NotNil(t, plan)
		assert.Nil(t, plan.Arguments)

		b, err := json.Marshal(plan)
		require.NoError(t, err)
		assert.NotContains(t, string(b), "arguments")
	})

	t.Run("ordinary read renders no plan or profile side-channel", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "json")
		renderResults(cmd, cfg, []renderResult{{
			columns: []string{"n"},
			rows:    []map[string]any{{"n": float64(1)}},
		}})

		out := stdout.String()
		assert.NotContains(t, out, `"plan"`)
		assert.NotContains(t, out, `"profile"`)
		assert.NotContains(t, out, `"arguments"`)
	})
}
