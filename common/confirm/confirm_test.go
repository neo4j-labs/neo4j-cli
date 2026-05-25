// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package confirm_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

func newTestCmd(t *testing.T, parentName string, in string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	parent := &cobra.Command{Use: parentName}
	leaf := &cobra.Command{Use: "delete"}
	parent.AddCommand(leaf)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	leaf.SetOut(out)
	leaf.SetErr(errOut)
	leaf.SetIn(strings.NewReader(in))
	return leaf, out, errOut
}

func TestRequire(t *testing.T) {
	type want struct {
		errSentinel       error
		errContains       string
		expectUsageExit2  bool
		expectPromptWrite bool
	}

	tests := []struct {
		name       string
		isTTY      bool
		yes, force bool
		stdin      string
		want       want
	}{
		{
			name:  "non-TTY neither flag",
			isTTY: false,
			want: want{
				expectUsageExit2: true,
				errContains:      "pass both --yes and --force",
			},
		},
		{
			name:  "non-TTY only --yes",
			isTTY: false,
			yes:   true,
			want: want{
				expectUsageExit2: true,
				errContains:      "pass both --yes and --force",
			},
		},
		{
			name:  "non-TTY only --force",
			isTTY: false,
			force: true,
			want: want{
				expectUsageExit2: true,
				errContains:      "pass both --yes and --force",
			},
		},
		{
			name:  "non-TTY both flags",
			isTTY: false,
			yes:   true,
			force: true,
			want:  want{},
		},
		{
			name:  "TTY missing flags answer y",
			isTTY: true,
			stdin: "y\n",
			want:  want{expectPromptWrite: true},
		},
		{
			name:  "TTY missing flags answer Y",
			isTTY: true,
			stdin: "Y\n",
			want:  want{expectPromptWrite: true},
		},
		{
			name:  "TTY missing flags answer yes",
			isTTY: true,
			stdin: "yes\n",
			want:  want{expectPromptWrite: true},
		},
		{
			name:  "TTY missing flags answer N",
			isTTY: true,
			stdin: "N\n",
			want: want{
				errSentinel:       confirm.ErrCancelled,
				expectPromptWrite: true,
			},
		},
		{
			name:  "TTY missing flags empty line cancels",
			isTTY: true,
			stdin: "\n",
			want: want{
				errSentinel:       confirm.ErrCancelled,
				expectPromptWrite: true,
			},
		},
		{
			name:  "TTY missing flags EOF cancels",
			isTTY: true,
			stdin: "",
			want: want{
				errSentinel:       confirm.ErrCancelled,
				expectPromptWrite: true,
			},
		},
		{
			name:  "TTY both flags skips prompt",
			isTTY: true,
			yes:   true,
			force: true,
			stdin: "n\n",
			want:  want{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			confirm.SetStdinIsTerminalForTest(t, func() bool { return tc.isTTY })

			leaf, _, errOut := newTestCmd(t, "instance", tc.stdin)
			f := confirm.Register(leaf)
			f.Yes = tc.yes
			f.Force = tc.force

			err := f.Require(leaf, "inst-1")

			if tc.want.errSentinel != nil {
				if !errors.Is(err, tc.want.errSentinel) {
					t.Fatalf("err = %v, want errors.Is sentinel %v", err, tc.want.errSentinel)
				}
			} else if tc.want.expectUsageExit2 {
				var ce *clierr.CLIError
				if !errors.As(err, &ce) {
					t.Fatalf("err = %v, want *clierr.CLIError", err)
				}
				if ce.Code != 2 {
					t.Fatalf("exit code = %d, want 2", ce.Code)
				}
				if tc.want.errContains != "" && !strings.Contains(ce.Error(), tc.want.errContains) {
					t.Fatalf("err message %q missing %q", ce.Error(), tc.want.errContains)
				}
			} else if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			wroteSomething := errOut.Len() > 0
			if tc.want.expectPromptWrite && !wroteSomething {
				t.Fatalf("expected prompt written to stderr; got nothing")
			}
			if !tc.want.expectPromptWrite && wroteSomething {
				t.Fatalf("expected NO prompt; stderr=%q", errOut.String())
			}
		})
	}
}

func TestRequire_EmptyResourceID_OmitsEmptyQuotes(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	leaf, _, errOut := newTestCmd(t, "deployment", "N\n")
	f := confirm.Register(leaf)

	err := f.Require(leaf, "")
	if !errors.Is(err, confirm.ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	prompt := errOut.String()
	if strings.Contains(prompt, `""`) {
		t.Fatalf("prompt has empty quotes: %q", prompt)
	}
	if !strings.Contains(prompt, "this deployment") {
		t.Fatalf("prompt missing 'this deployment': %q", prompt)
	}
}

func TestRequire_EmptyResourceID_NonTTY_OmitsEmptyQuotes(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	leaf, _, _ := newTestCmd(t, "deployment", "")
	f := confirm.Register(leaf)

	err := f.Require(leaf, "")
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *clierr.CLIError", err)
	}
	if strings.Contains(ce.Error(), `""`) {
		t.Fatalf("error has empty quotes: %q", ce.Error())
	}
	if !strings.Contains(ce.Error(), "this deployment") {
		t.Fatalf("error missing 'this deployment': %q", ce.Error())
	}
}

func TestRegister_BothFlagsBound(t *testing.T) {
	leaf, _, _ := newTestCmd(t, "instance", "")
	_ = confirm.Register(leaf)

	for _, name := range []string{"yes", "force"} {
		if leaf.Flags().Lookup(name) == nil {
			t.Fatalf("flag --%s not bound", name)
		}
	}
}

func TestRequire_ResourceTypeFromParent(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	leaf, _, _ := newTestCmd(t, "agent", "")
	f := confirm.Register(leaf)

	err := f.Require(leaf, "agt-42")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `agent "agt-42"`) {
		t.Fatalf("error %q missing resource type/ID", err.Error())
	}
}
