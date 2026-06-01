// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

// SetSeamsForTest overrides the agent-detection and stdin-TTY seams so that
// callers in other packages can drive Invoker()/Detect() deterministically in
// tests. It returns a restore func; call it (e.g. via t.Cleanup) to revert.
// This exists so the human/agent classifier stays the single source of truth
// in common/agent while remaining testable from dependent packages.
func SetSeamsForTest(detected, tty bool) (restore func()) {
	origGetenv := getenv
	origTTY := stdinIsTerminal
	if detected {
		getenv = func(key string) string {
			if key == "CLAUDECODE" {
				return "1"
			}
			return ""
		}
	} else {
		getenv = func(string) string { return "" }
	}
	stdinIsTerminal = func() bool { return tty }
	return func() {
		getenv = origGetenv
		stdinIsTerminal = origTTY
	}
}
