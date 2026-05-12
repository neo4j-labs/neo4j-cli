// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command release_fixture is the CI-only fixture server for the tier-1 e2e
// test of `neo4j-cli update check`. It impersonates the GitHub releases API
// against a single localhost port, driven by the e2e_seams build of
// neo4j-cli (see neo4j-cli/internal/subcommands/update/seams_e2e.go).
//
// Layout: pure stdlib, no module deps. Two canned release lists are embedded
// at compile time from testdata/ and selected via --scenario. Unlike its
// sibling update_fixture, this fixture serves ONLY the releases-list endpoint
// — the tier-1 test exercises `update check`, which never downloads archives.
//
// Endpoints:
//
//   - GET /repos/neo4j-labs/neo4j-cli/releases
//     Returns the canned release list for the selected scenario.
//
// Lifecycle:
//
//   - Listens on 127.0.0.1:0, prints `listening on http://127.0.0.1:<port>`
//     to stdout (single line, FIRST line of stdout) so the harness can
//     scrape and capture the port.
//   - Shuts down cleanly on SIGINT / SIGTERM.
//   - Exits non-zero on any setup error (bad scenario, port bind).
//
// Flags:
//
//	--scenario  one of "stable-head" | "pre-release-head" (required)
//
// Channel invariant by scenario (under `update check --pre-releases`):
//
//   - stable-head      → stable v9.9.0 at list head; Latest() returns it →
//     channel "stable" (even with --pre-releases since the stable is the
//     newer release in the list AND outranks its sibling by semver).
//   - pre-release-head → prerelease v9.9.1-alpha.1 at list head; Latest()
//     with --pre-releases returns it → channel "pre-release".
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed testdata/releases-stable-head.json
var releasesStableHead []byte

//go:embed testdata/releases-pre-release-head.json
var releasesPreReleaseHead []byte

func main() {
	scenario := flag.String("scenario", "", "release scenario: stable-head | pre-release-head (required)")
	flag.Parse()

	var body []byte
	switch *scenario {
	case "stable-head":
		body = releasesStableHead
	case "pre-release-head":
		body = releasesPreReleaseHead
	case "":
		fmt.Fprintln(os.Stderr, "release_fixture: --scenario is required (stable-head | pre-release-head)")
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "release_fixture: unknown --scenario %q (want stable-head | pre-release-head)\n", *scenario)
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/neo4j-labs/neo4j-cli/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "release_fixture: bind: %v\n", err)
		os.Exit(1)
	}
	addr := ln.Addr().(*net.TCPAddr)
	// FIRST line of stdout — harness scrapes this. Print BEFORE Serve blocks.
	fmt.Printf("listening on http://127.0.0.1:%d\n", addr.Port)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "release_fixture: serve: %v\n", err)
		os.Exit(1)
	}
}
