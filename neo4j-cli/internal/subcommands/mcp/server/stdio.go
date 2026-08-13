// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/neo4j/cli/common/clierr"
)

// stdioClaimed guards the process-global swap, so a second ClaimStdio fails
// loudly instead of handing out the redirected streams.
var stdioClaimed atomic.Bool

// ClaimStdio hands the caller exclusive use of the process's real stdin and
// stdout — the two halves of an MCP stdio transport — and points the os.Stdin
// and os.Stdout variables somewhere harmless: stdout at stderr, stdin at
// os.DevNull. Anything that afterwards writes through the os.Stdout variable (a
// stray Println, a subprocess handed cmd.Stdout = os.Stdout) lands on stderr
// instead of corrupting a JSON-RPC frame, and anything that reads os.Stdin gets
// EOF instead of eating the protocol stream — `query` with no positional
// argument falls through to io.ReadAll(os.Stdin), and the password prompts read
// its file descriptor.
//
// The swap is at the variable, not the file descriptor: a write to fd 1 that
// never consults os.Stdout still reaches the frame. Closing that gap needs
// dup2, which is not portable and would fight handing the transport an *os.File.
//
// The returned files must be given to the SDK's IOTransport, NOT its
// StdioTransport: StdioTransport reads the os.Stdin and os.Stdout variables at
// Connect time, i.e. after this swap, so it would talk to /dev/null and stderr.
//
// Note that TTY detection also reads the os.Stdout variable
// (output.StdoutIsTerminal), so after the swap it observes stderr; commands
// dispatched over MCP must be given an explicit --format rather than relying on
// the terminal default.
//
// Call when starting a server, never from package init or a test that does not
// restore: the swap is process-global. A second claim before restore is refused
// rather than served, because it would hand out the swapped values — /dev/null
// and stderr — as if they were the transport. restore is safe to call more than
// once and returns the process to its original streams.
func ClaimStdio() (in *os.File, out *os.File, restore func(), err error) {
	if !stdioClaimed.CompareAndSwap(false, true) {
		return nil, nil, nil, clierr.NewFatalError("the process stdio is already claimed by an MCP transport, please report an issue in %s", clierr.IssuesURL)
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		stdioClaimed.Store(false)
		return nil, nil, nil, clierr.NewFatalError("cannot open %s to detach stdin from the MCP transport: %s", os.DevNull, err.Error())
	}

	in, out = os.Stdin, os.Stdout
	os.Stdin, os.Stdout = devNull, os.Stderr

	var once sync.Once
	restore = func() {
		once.Do(func() {
			os.Stdin, os.Stdout = in, out
			_ = devNull.Close()
			stdioClaimed.Store(false)
		})
	}
	return in, out, restore, nil
}
