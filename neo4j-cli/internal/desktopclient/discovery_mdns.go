// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktopclient — discovery_mdns.go isolates ALL mDNS/DNS-SD discovery
// of the Neo4j Desktop 2 relate API. Every import of the mDNS library
// (github.com/hashicorp/mdns) and the macOS `dns-sd` shell-out lives only in
// this file, so the dependency surface is contained to a single place.
//
// macOS Local Network permission rationale:
//
// On macOS 15+ a bare CLI binary (one not bundled as a .app with the Local
// Network entitlement) may have its multicast traffic on 224.0.0.251:5353
// SILENTLY DROPPED by the OS — there is no permission prompt and no error
// returned to the process, so the in-process mDNS browse simply observes zero
// responders. This is indistinguishable, from the CLI's point of view, from
// "Desktop isn't advertising". To survive this, macOS gets a `dns-sd` fallback
// tier: `dns-sd` runs inside `mDNSResponder`, the system daemon that already
// holds the Local Network grant, so it can see responders the CLI's own socket
// cannot. When the in-process browse comes back empty on macOS, we shell out to
// `dns-sd` before giving up and falling through to the legacy port scan.

package desktopclient

import (
	"bufio"
	"context"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	// MDNSServiceType is the DNS-SD service type Neo4j Desktop 2 advertises its
	// relate API under (PR #381, branch feature/multicast-dns).
	MDNSServiceType = "_neo4j-desktop-2._tcp"
	// mdnsInstanceName is the DNS-SD instance name Desktop registers; used as
	// the `-L` argument to `dns-sd`.
	mdnsInstanceName = "Neo4j Desktop 2"
	// mdnsDomain is the multicast DNS domain.
	mdnsDomain = "local."

	// mdnsBrowseTimeout caps the in-process multicast browse. Kept tight so the
	// common (Desktop-present) case resolves quickly and the absent case falls
	// through to the next tier without dragging out the worst-case budget.
	mdnsBrowseTimeout = 750 * time.Millisecond
	// dnssdLookupTimeout caps the macOS `dns-sd -L` shell-out. `dns-sd -L`
	// streams forever, so this deadline is what actually terminates it.
	dnssdLookupTimeout = 1500 * time.Millisecond
)

// mdnsBrowseFn browses for the Desktop relate API over in-process mDNS
// multicast. Replaceable in tests via SetMDNSBrowseFnForTest.
var mdnsBrowseFn = browseDesktopMDNS

func SetMDNSBrowseFnForTest(fn func(ctx context.Context) (port int, ok bool)) func() {
	prev := mdnsBrowseFn
	mdnsBrowseFn = fn
	return func() { mdnsBrowseFn = prev }
}

// dnssdLookupFn resolves the Desktop relate API via the macOS `dns-sd` binary.
// Replaceable in tests via SetDNSSDLookupFnForTest.
var dnssdLookupFn = lookupDesktopDNSSD

func SetDNSSDLookupFnForTest(fn func(ctx context.Context) (port int, ok bool)) func() {
	prev := dnssdLookupFn
	dnssdLookupFn = fn
	return func() { dnssdLookupFn = prev }
}

// browseDesktopMDNS queries `_neo4j-desktop-2._tcp.local` over multicast and
// returns the port of the first responder. Only the SRV port is taken; the SRV
// target / A / TXT host is ignored because the server always binds 127.0.0.1
// and the auth layer forces that origin. Any error, no responder, or a deadline
// yields (0, false) so the caller can fall through to the next tier.
func browseDesktopMDNS(ctx context.Context) (port int, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, mdnsBrowseTimeout)
	defer cancel()

	entries := make(chan *mdns.ServiceEntry, 4)
	params := mdns.DefaultParams(MDNSServiceType)
	params.Domain = strings.TrimSuffix(mdnsDomain, ".")
	params.Entries = entries
	// Silence the library's stderr logging — its socket-bind warnings (common
	// on macOS when the Local Network grant is missing) are not actionable for
	// the user and would otherwise leak into CLI output.
	params.Logger = log.New(io.Discard, "", 0)

	resultCh := make(chan int, 1)
	go func() {
		for entry := range entries {
			if entry != nil && entry.Port > 0 {
				select {
				case resultCh <- entry.Port:
				default:
				}
				return
			}
		}
	}()

	if err := mdns.QueryContext(ctx, params); err != nil {
		return 0, false
	}
	close(entries)

	select {
	case p := <-resultCh:
		return p, true
	default:
		return 0, false
	}
}

// lookupDesktopDNSSD resolves the relate API port via the macOS `dns-sd`
// binary, which runs inside mDNSResponder (the daemon that holds the Local
// Network grant). Gated on darwin and `dns-sd` being on PATH; either gate
// failing yields (0, false). `dns-sd -L` streams and never exits, so it runs
// under a context deadline: we read stdout, parse the first
// `... can be reached at <host>:<port> ...` line, then cancel/kill. A missing
// binary, parse failure, or timeout all yield (0, false).
func lookupDesktopDNSSD(ctx context.Context) (port int, ok bool) {
	if goosFn() != "darwin" {
		return 0, false
	}
	if _, err := exec.LookPath("dns-sd"); err != nil {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(ctx, dnssdLookupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "dns-sd", "-L", mdnsInstanceName, MDNSServiceType)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, false
	}
	if err := cmd.Start(); err != nil {
		return 0, false
	}
	// Ensure the streaming process is reaped once we're done reading.
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if p, found := parseDNSSDPort(scanner.Text()); found {
			return p, true
		}
	}
	return 0, false
}

// parseDNSSDPort extracts the port from a `dns-sd -L` resolution line of the
// form `... can be reached at <host>:<port> ...`. Returns (0, false) when the
// line doesn't carry a reachable host:port.
func parseDNSSDPort(line string) (int, bool) {
	const marker = "can be reached at "
	idx := strings.Index(line, marker)
	if idx < 0 {
		return 0, false
	}
	rest := strings.TrimSpace(line[idx+len(marker):])
	// rest is like "neo4j-desktop-2.local.:49500 (interface ...)" — take the
	// first whitespace-delimited token, then the segment after the last colon.
	token := rest
	if sp := strings.IndexAny(token, " \t"); sp >= 0 {
		token = token[:sp]
	}
	colon := strings.LastIndex(token, ":")
	if colon < 0 || colon == len(token)-1 {
		return 0, false
	}
	p, err := strconv.Atoi(token[colon+1:])
	if err != nil || p <= 0 {
		return 0, false
	}
	return p, true
}
