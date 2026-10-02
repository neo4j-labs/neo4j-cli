// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestVirtualGraphAndSessionValidateScopeAndIDBeforeAnyRequest(t *testing.T) {
	// A nil config proves no request is attempted: validation fails first.
	vg := virtualGraphService{}
	sess := sessionService{}
	good := Scope{OrgID: "org-1", ProjectID: "proj-1"}
	ctx := context.Background()

	cases := map[string]func() error{
		"vg get bad id":          func() error { _, err := vg.Get(ctx, good, "../x"); return err },
		"vg get bad org":         func() error { _, err := vg.Get(ctx, Scope{OrgID: "a/b", ProjectID: "p"}, "id"); return err },
		"vg update bad id":       func() error { _, err := vg.Update(ctx, good, "a?b", VirtualGraphPatch{}); return err },
		"vg delete bad project":  func() error { return vg.Delete(ctx, Scope{OrgID: "o", ProjectID: ".."}, "id") },
		"vg list negative limit": func() error { _, err := vg.List(ctx, good, -1); return err },
		"vg allowed configs":     func() error { _, err := vg.AllowedConfigs(ctx, Scope{ProjectID: "p"}); return err },
		"session get bad id":     func() error { _, err := sess.Get(ctx, good, "../../x"); return err },
		"session delete bad id":  func() error { _, err := sess.Delete(ctx, good, "a#b"); return err },
		"session list bad org":   func() error { _, err := sess.List(ctx, Scope{OrgID: "", ProjectID: "p"}, ""); return err },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			err := call()
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "want a clierr, got %v", err)
		})
	}
}

func TestNewSession_NormalisesV2Beta1Fields(t *testing.T) {
	sess := newSession(map[string]any{
		"id": "s1", "name": "n", "legacy_status": "Ready", "tenant_id": "p1", "brand_new": true,
	})

	assert.Equal(t, "Ready", sess.Status)
	assert.Equal(t, "p1", sess.ProjectID)
	assert.NotContains(t, sess.Record, "legacy_status")
	assert.NotContains(t, sess.Record, "tenant_id")
	assert.Equal(t, true, sess.Record["brand_new"], "unmodelled fields survive into the rendered record")
}

func TestSingle(t *testing.T) {
	rec, err := single([]map[string]any{{"id": "a"}}, "fetching x")
	require.NoError(t, err)
	assert.Equal(t, "a", rec["id"])

	for _, rows := range [][]map[string]any{nil, {}, {{"id": "a"}, {"id": "b"}}} {
		_, err := single(rows, "fetching x")
		var ce *clierr.CLIError
		assert.True(t, errors.As(err, &ce), "%d rows must be an error, not an index panic", len(rows))
	}
}

func TestSessionCreateReadyConstant(t *testing.T) {
	assert.Equal(t, "Ready", SessionStatusReady)
}
