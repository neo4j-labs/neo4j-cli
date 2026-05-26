// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package confirmtest_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/spf13/cobra"
)

// newStubLeaf wires a tiny parent+leaf tree with a sink counter, mirroring
// the shape every real destructive leaf takes: parent.Name() supplies the
// prompt label; confirm.Register binds --yes/--force; confirm.Require gates
// the sink.
func newStubLeaf(t *testing.T, parentName, args, stdin string) (run func() error, errOut *bytes.Buffer, invoked *bool) {
	t.Helper()
	parent := &cobra.Command{Use: parentName}
	out := &bytes.Buffer{}
	errOut = &bytes.Buffer{}
	sinkFired := false
	invoked = &sinkFired
	leaf := &cobra.Command{
		Use: "delete <id>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := confirm.Require(cmd, "stub-id"); err != nil {
				return err
			}
			sinkFired = true
			return nil
		},
	}
	confirm.Register(leaf)
	parent.AddCommand(leaf)
	parent.SetOut(out)
	parent.SetErr(errOut)
	parent.SetIn(strings.NewReader(stdin))
	parent.SetArgs(append([]string{"delete"}, strings.Fields(args)...))
	run = parent.Execute
	return run, errOut, invoked
}

func TestAssertLeafGate_AgainstWellFormedLeaf(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "stub delete",
		NoFlagsArgs:   "id-1",
		BothFlagsArgs: "id-1 --yes --force",
		ResourceLabel: "stub",
		Run: func(t *testing.T, args string, stdin string) confirmtest.GateRunResult {
			run, errOut, invoked := newStubLeaf(t, "stub", args, stdin)
			err := run()
			return confirmtest.GateRunResult{
				Err:     err,
				Stderr:  errOut.String(),
				Invoked: *invoked,
			}
		},
	})
}
