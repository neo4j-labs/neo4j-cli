// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command fakebin is the "binary" packaged inside the e2e fixture's release
// archive. After the e2e harness drives `neo4j-cli update --pre-releases`,
// it executes the swapped binary at the runner's neo4j-cli path; this
// program prints a recognizable string (`swapped: <TAG>`) so the harness
// can confirm the swap actually happened (vs. a no-op + exit 0 that would
// otherwise look identical).
//
// The tag is baked in via -ldflags='-X main.tag=<TAG>' at build time by the
// fixture server; we don't read it from the environment so a stale env var
// in the runner can't false-pass the assertion.
package main

import "fmt"

// tag is overridden via ldflags by the fixture server. Each archive build
// produces a binary baked with the matching release tag so the assertion in
// the e2e harness can match the swap target.
var tag = "unset"

func main() {
	fmt.Printf("swapped: %s\n", tag)
}
