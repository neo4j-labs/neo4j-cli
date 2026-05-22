// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command desktop_fixture is the CI-only fixture server for the desktop e2e
// test suite. It impersonates Neo4j Desktop 2's relate API on a single
// localhost port so CI runners can exercise `bin/neo4j-cli desktop ...` and
// `query -c desktop[-connection:<uuid>]` end-to-end without installing the
// real Desktop app. The neo4j-cli binary under test is built with the
// `-tags e2e_desktop_seams` build (see neo4j-cli/internal/desktopclient/
// seams_e2e.go in task-013) so the port probe, salt loader, data-dir lookup,
// and HTTP-origin lookup are all redirected to the fixture URL + a fixed
// test salt instead of the real Desktop discovery paths.
//
// Endpoints (relate-shaped — see neo4j-desktop-2
// `packages/web/src/fastify/routes/{dbms,connection}.routes.ts` and
// `packages/electron/src/api/credentials.routes.ts`):
//
//   - GET    /fastify/api-docs                      → probe target (200)
//   - GET    /fastify/api/dbmss                     → []Dbms (NO status field)
//   - GET    /fastify/api/dbmss/info                → []DbmsInfo (WITH status)
//   - GET    /fastify/api/dbmss/:id                 → DbmsInfo
//   - GET    /fastify/api/dbmss/versions            → []DbmsVersion
//   - POST   /fastify/api/dbmss                     → create
//   - DELETE /fastify/api/dbmss/:id
//   - POST   /fastify/api/dbmss/:id/start
//   - POST   /fastify/api/dbmss/:id/stop
//   - GET    /fastify/api/connections               → []Connection
//   - POST   /fastify/api/connections
//   - PATCH  /fastify/api/connections/:id
//   - DELETE /fastify/api/connections/:id
//   - GET    /fastify/api/credentials/:key          → {username,password}|null
//
// Auth: every /fastify/api/* request must carry X-Client-Id + a valid HS256
// X-API-Token signed with the composite key `<salt>-<httpOrigin>-<clientId>`
// (Desktop's `packages/common/src/token.service.ts` derivation). The salt
// defaults to the fixed test value below; tweak with --salt for negative
// tests. The httpOrigin defaults to the fixture's listen URL but can be
// pinned with --http-origin so the neo4j-cli e2e seam can supply the same
// value during signing.
//
// Scenario admin (out-of-band control plane, unauthenticated):
//
//   - POST /_scenario/reset
//   - POST /_scenario/dbms
//   - POST /_scenario/connection
//   - POST /_scenario/credential
//   - POST /_scenario/auth_mode  body {"mode":"accept"|"reject"|"500"|"close"}
//   - POST /_scenario/transition body {"id":..,"to_status":..,"after_calls":N}
//
// Lifecycle:
//
//   - Listens on 127.0.0.1:0 by default (use --port to pin).
//   - Prints `listening on http://127.0.0.1:<port>` as the FIRST line of
//     stdout so the harness can scrape and capture the port.
//   - Logs every authed request to stderr for debuggability.
//   - Graceful shutdown on SIGINT / SIGTERM.
//
// Flags:
//
//	--port         listen port (0 = pick free; default 0)
//	--salt         JWT signing salt (default "testsalt")
//	--http-origin  origin folded into the JWT key (default = listen URL)
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := flag.Int("port", 0, "listen port (0 = pick free)")
	salt := flag.String("salt", "testsalt", "JWT signing salt (fixed test value)")
	httpOrigin := flag.String("http-origin", "", "origin folded into JWT key (default = listen URL)")
	flag.Parse()

	state := newState(*salt)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "desktop_fixture: bind: %v\n", err)
		os.Exit(1)
	}
	addr := ln.Addr().(*net.TCPAddr)
	listenURL := fmt.Sprintf("http://127.0.0.1:%d", addr.Port)
	if *httpOrigin == "" {
		state.setOrigin(listenURL)
	} else {
		state.setOrigin(*httpOrigin)
	}

	mux := newMux(state)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// FIRST line of stdout — harness scrapes this. Print BEFORE Serve blocks.
	fmt.Printf("listening on %s\n", listenURL)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "desktop_fixture: serve: %v\n", err)
		os.Exit(1)
	}
}
