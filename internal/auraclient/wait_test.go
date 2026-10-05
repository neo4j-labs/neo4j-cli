// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

// The wait methods hand their IDs to transport poll functions that
// interpolate them into a URL path, so they must validate those IDs like
// every other method does. A zero-value service has a nil config: any
// transport call would panic, so a returned validation error proves no
// request was attempted.
func TestWaitMethods_ValidateIDsBeforeAnyRequest(t *testing.T) {
	good := Scope{OrgID: "org-1", ProjectID: "proj-1"}
	ctx := context.Background()

	inst := instanceService{}
	snap := snapshotService{}
	cmk := cmkService{}
	gql := graphqlService{}
	sess := sessionService{}
	vg := virtualGraphService{}

	cases := map[string]func() error{
		"instance wait bad id": func() error { _, err := inst.WaitWhile(ctx, good, "a/b", InstanceStatusCreating); return err },
		"instance wait bad org": func() error {
			_, err := inst.WaitWhile(ctx, Scope{OrgID: "a/b", ProjectID: "p"}, "id", InstanceStatusCreating)
			return err
		},
		"instance wait bad project": func() error {
			_, err := inst.WaitWhile(ctx, Scope{OrgID: "o", ProjectID: "a/b"}, "id", InstanceStatusCreating)
			return err
		},
		"snapshot wait bad instance": func() error { _, err := snap.WaitWhilePending(ctx, "a/b", "snap-1"); return err },
		"snapshot wait bad id":       func() error { _, err := snap.WaitWhilePending(ctx, "inst-1", "a/b"); return err },
		"cmk wait bad id":            func() error { _, err := cmk.WaitWhilePending(ctx, "a/b"); return err },
		"graphql wait bad instance":  func() error { _, err := gql.WaitWhile(ctx, "a/b", "gql-1", GraphQLStatusCreating); return err },
		"graphql wait bad id":        func() error { _, err := gql.WaitWhile(ctx, "inst-1", "a/b", GraphQLStatusCreating); return err },
		"session wait bad id":        func() error { _, err := sess.WaitUntilReady(ctx, good, "a/b"); return err },
		"session wait bad org": func() error {
			_, err := sess.WaitUntilReady(ctx, Scope{OrgID: "a/b", ProjectID: "p"}, "id")
			return err
		},
		"session wait bad project": func() error {
			_, err := sess.WaitUntilReady(ctx, Scope{OrgID: "o", ProjectID: "a/b"}, "id")
			return err
		},
		"vg wait bad id": func() error { _, err := vg.WaitWhile(ctx, good, "a/b", VirtualGraphStatusCreating); return err },
		"vg wait bad org": func() error {
			_, err := vg.WaitWhile(ctx, Scope{OrgID: "a/b", ProjectID: "p"}, "id", VirtualGraphStatusCreating)
			return err
		},
		"vg wait bad project": func() error {
			_, err := vg.WaitWhile(ctx, Scope{OrgID: "o", ProjectID: "a/b"}, "id", VirtualGraphStatusCreating)
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			err := call()
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "want a clierr, got %v", err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}
