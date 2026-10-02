// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package quip

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

var (
	fixA       = mustDecode("bmVv")
	fixAMsg    = mustDecode("SGUgaXMgdGhlIG9uZS4=")
	fixB       = mustDecode("dHJpbml0eQ==")
	fixCase    = mustDecode("TkVP")
	fixVariant = mustDecode("VGhlLU9uZQ==")
)

func mustDecode(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestNormalise(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"AB-CD", "abcd"},
		{"AB_CD", "abcd"},
		{"AB CD", "abcd"},
		{"abcd", "abcd"},
		{"ABCD", "abcd"},
		{"", ""},
		{"--", ""},
		{"X.Y", "x.y"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, normalise(c.in))
		})
	}
}

func TestMessagesB64_AllDecode(t *testing.T) {
	for i, m := range messagesB64 {
		out, err := base64.StdEncoding.DecodeString(m)
		assert.NoError(t, err, "entry %d", i)
		assert.NotEmpty(t, out, "entry %d", i)
	}
}

func TestKeyHashes_PointToValidIndex(t *testing.T) {
	for h, idx := range keyHashes {
		assert.GreaterOrEqual(t, idx, 0, "hash %s", h)
		assert.Less(t, idx, len(messagesB64), "hash %s", h)
	}
}

func TestEmit_TTYGate(t *testing.T) {
	var buf bytes.Buffer
	Emit(&buf, false, false, fixA)
	assert.Empty(t, buf.String())
}

func TestEmit_Suppressed(t *testing.T) {
	var buf bytes.Buffer
	Emit(&buf, true, true, fixA)
	assert.Empty(t, buf.String())
}

func TestEmit_NilWriter(t *testing.T) {
	Emit(nil, true, false, fixA)
}

func TestEmit_NoMatch(t *testing.T) {
	var buf bytes.Buffer
	Emit(&buf, true, false, "definitely-not-a-trigger", "")
	assert.Empty(t, buf.String())
}

func TestEmit_Match(t *testing.T) {
	withDice(t, true)
	cases := []string{fixA, fixCase, fixVariant}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			var buf bytes.Buffer
			Emit(&buf, true, false, c)
			assert.Contains(t, buf.String(), fixAMsg)
			assert.Equal(t, byte('\n'), buf.Bytes()[buf.Len()-1])
		})
	}
}

func TestEmit_FirstMatchWins(t *testing.T) {
	withDice(t, true)
	var buf bytes.Buffer
	Emit(&buf, true, false, fixA, fixB)
	assert.Contains(t, buf.String(), fixAMsg)
}

func TestEmit_SkipsEmptyCandidates(t *testing.T) {
	withDice(t, true)
	var buf bytes.Buffer
	Emit(&buf, true, false, "", "  ", fixA)
	assert.Contains(t, buf.String(), fixAMsg)
}

func TestEmit_DiceGate(t *testing.T) {
	withDice(t, false)
	var buf bytes.Buffer
	Emit(&buf, true, false, fixA)
	assert.Empty(t, buf.String())
}

func newTestRoot(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	var stderr bytes.Buffer
	root := &cobra.Command{Use: "root"}
	leaf := &cobra.Command{
		Use:  "leaf",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	leaf.Flags().String("name", "", "")
	leaf.Flags().String("plugin", "", "")
	leaf.Flags().String("password", "", "")
	root.AddCommand(leaf)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	return root, &stderr
}

func withTTY(t *testing.T, isTTY bool) {
	t.Helper()
	prev := stderrIsTerminal
	stderrIsTerminal = func() bool { return isTTY }
	t.Cleanup(func() { stderrIsTerminal = prev })
}

func withDice(t *testing.T, fire bool) {
	t.Helper()
	prev := dice
	dice = func() bool { return fire }
	t.Cleanup(func() { dice = prev })
}

func TestHook_PicksUpPositionalArg(t *testing.T) {
	withTTY(t, true)
	withDice(t, true)
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", fixA})
	assert.NoError(t, root.Execute())
	assert.Contains(t, stderr.String(), fixAMsg)
}

func TestHook_PicksUpNameFlag(t *testing.T) {
	withTTY(t, true)
	withDice(t, true)
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", "--name", fixA})
	assert.NoError(t, root.Execute())
	assert.Contains(t, stderr.String(), fixAMsg)
}

func TestHook_PicksUpPluginFlag(t *testing.T) {
	withTTY(t, true)
	withDice(t, true)
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", "--plugin", fixA})
	assert.NoError(t, root.Execute())
	assert.Contains(t, stderr.String(), fixAMsg)
}

func TestHook_DoesNotScanPassword(t *testing.T) {
	withTTY(t, true)
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", "--password", fixA})
	assert.NoError(t, root.Execute())
	assert.Empty(t, stderr.String())
}

func TestHook_NoTTY_NoOutput(t *testing.T) {
	withTTY(t, false)
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", fixA})
	assert.NoError(t, root.Execute())
	assert.Empty(t, stderr.String())
}

func TestHook_EnvSuppression(t *testing.T) {
	withTTY(t, true)
	t.Setenv(suppressEnv, "1")
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", fixA})
	assert.NoError(t, root.Execute())
	assert.Empty(t, stderr.String())
}

func TestHook_NoTriggerInArgs(t *testing.T) {
	withTTY(t, true)
	root, stderr := newTestRoot(t)
	Hook(root)
	root.SetArgs([]string{"leaf", "--name", "boring"})
	assert.NoError(t, root.Execute())
	assert.Empty(t, stderr.String())
}

func TestHook_ChainsExistingPostRunE(t *testing.T) {
	withTTY(t, true)
	withDice(t, true)
	root, stderr := newTestRoot(t)
	called := false
	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		called = true
		return nil
	}
	Hook(root)
	root.SetArgs([]string{"leaf", fixA})
	assert.NoError(t, root.Execute())
	assert.True(t, called)
	assert.Contains(t, stderr.String(), fixAMsg)
}

func TestHook_ExistingPostRunErrorShortCircuits(t *testing.T) {
	withTTY(t, true)
	root, stderr := newTestRoot(t)
	root.SilenceErrors = true
	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		return errors.New("boom")
	}
	Hook(root)
	root.SetArgs([]string{"leaf", fixA})
	assert.Error(t, root.Execute())
	assert.NotContains(t, stderr.String(), fixAMsg)
}
