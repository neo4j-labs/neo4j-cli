// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"fmt"
	"io"

	"github.com/neo4j/cli/internal/clicfg"
)

// keyAutoRegistered records why the one-time hint is finished: the tool was set
// up by an explicit command ("done"), the hint was shown ("hinted"), or a person
// turned feedback off ("disabled"). Any value stops further hints.
const (
	keyAutoRegistered = "feedback.auto-registered"
	autoDone          = "done"
	autoDisabled      = "disabled"
	autoHinted        = "hinted"
)

// registerFor stores the endpoint and instructions variant and registers the
// tool with Claude Code. It authorizes nothing: no report can be sent until a
// person accepts the first-report question or runs `feedback enable`.
func registerFor(ctx context.Context, cfg *clicfg.Config, ep, instr string) (agents, notes []string, err error) {
	if err := setValues(cfg, [2]string{keyEndpoint, ep}, [2]string{keyInstructions, instr}, [2]string{keyAutoRegistered, autoDone}); err != nil {
		return nil, nil, err
	}
	agents, notes = registerAgents(ctx, instr)
	return agents, notes, nil
}

// MaybeRegisterHint prints a one-time hint the first time the CLI runs inside
// Claude Code, saying how to set up agent feedback. It changes nothing in the
// agents' configuration (it only records in neo4j-cli's own config that the hint
// was shown): a read-only command such as --version must not write to the user's
// agent configuration (an
// earlier version registered the tool here; Claude Code flagged that as an
// unrequested side effect and reverted it). It prints at most once per machine,
// and only when: the feature flag is on, the caller is Claude Code or Codex, DO_NOT_TRACK
// is not 1, the command is not a `feedback` command, nobody ran `feedback
// register`, `enable` or `disable`, and an allowed endpoint is known. It writes
// one line to w (stderr) and never touches stdout, the error envelope or the exit code.
func MaybeRegisterHint(cfg *clicfg.Config, args []string, w io.Writer) {
	if cfg == nil || cfg.Global == nil || cfg.Flags == nil || w == nil {
		return
	}
	if !cfg.Flags.Enabled("flag.feedback-command") || optOut(cfg) != "" {
		return
	}
	for _, a := range args {
		if a == "feedback" {
			return
		}
	}
	if getString(cfg, keyAutoRegistered) != "" || !detectAgent() || (harnessLabel() != claudeCodeAgent && harnessLabel() != codexAgent) {
		return
	}
	ep := endpoint(cfg)
	if ep == "" {
		ep = fallbackEndpoint()
	}
	if ep == "" || !endpointAllowed(ep) {
		return
	}
	if err := setValues(cfg, [2]string{keyAutoRegistered, autoHinted}); err != nil {
		return
	}
	_, _ = fmt.Fprintln(w, "neo4j-cli: agent feedback is not set up. A person can turn it on with: neo4j-cli feedback enable. "+
		"Or run: neo4j-cli feedback register (registers a tool that sends nothing; the first time an agent reports a problem, you are asked to accept or decline).")
}
