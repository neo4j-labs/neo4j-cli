// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command check_json validates the JSON output of `neo4j-cli update check`.
//
// It is invoked from the CI e2e steps (.github/workflows/test.yml). The step
// pipes the freshly-built binary's stdout into this helper and the helper
// asserts the documented REQ-F-018 shape:
//
//	{
//	  "current": "<string>",
//	  "latest":  "<valid semver>",
//	  "updated": false,
//	  "check":   true,
//	  "channel": "<string>",
//	  "install_method": "<string>"
//	}
//
// Optional fields `updated_skills` and `skill_install_suggested` MUST be
// absent for the `check` subcommand (no swap occurred), and the helper
// enforces that.
//
// Modes:
//
//   - Default: shape + types + post-swap-absence + optional value coupling
//     via --expect-channel / --cross-check. Used by the hermetic Tier-1 step
//     where the canned fixture pins both channel and latest tag.
//   - --schema-only: shape + types + post-swap-absence + enum membership
//     (channel ∈ {stable, pre-release}, install_method ∈
//     {binary, homebrew, npm, pipx, uv} per
//     neo4j-cli/internal/subcommands/update/install_method.go). Skips
//     value-coupling so the live-API smoke step is calendar-immune.
//
// Flags:
//
//	--expect-channel <string>   Asserts the channel field equals the value (e.g. "pre-release").
//	                            Mutually exclusive with --schema-only.
//	--cross-check    <tag>      Asserts the latest field equals the value (the tag fetched
//	                            via `gh release list --limit 1`). Detects filter drift between
//	                            release.go's Latest and the head of GitHub's API.
//	                            Mutually exclusive with --schema-only.
//	--schema-only               Skip value-coupling assertions; enforce shape + enum
//	                            membership only. Mutually exclusive with the two flags
//	                            above; passing them together exits 1 before reading stdin.
//
// Exit codes: 0 on success, 1 on any assertion failure (with a clear stderr
// message). Reads JSON from stdin.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/mod/semver"
)

// validChannels is the enum used by schema-only mode to assert `channel`
// membership. The values mirror release.go's documented channel labels and
// are stable across the production binary's JSON output (REQ-F-018).
var validChannels = map[string]struct{}{
	"stable":      {},
	"pre-release": {},
}

// validInstallMethods is the enum used by schema-only mode to assert
// `install_method` membership. The values mirror the InstallMethod constants
// declared in neo4j-cli/internal/subcommands/update/install_method.go:45-49
// and are stable across the production binary's JSON output (REQ-F-018).
var validInstallMethods = map[string]struct{}{
	"binary":   {},
	"homebrew": {},
	"npm":      {},
	"pipx":     {},
	"uv":       {},
}

// resultDoc mirrors the REQ-F-018 JSON shape emitted by `neo4j-cli update check
// -f json`. The optional updated_skills / skill_install_suggested fields are
// captured as raw json.RawMessage so we can detect "field present" vs "absent":
// json.Unmarshal of a missing field leaves the RawMessage at its zero value (nil).
type resultDoc struct {
	Current               string          `json:"current"`
	Latest                string          `json:"latest"`
	Updated               bool            `json:"updated"`
	Check                 bool            `json:"check"`
	Channel               string          `json:"channel"`
	InstallMethod         string          `json:"install_method"`
	UpdatedSkills         json.RawMessage `json:"updated_skills,omitempty"`
	SkillInstallSuggested json.RawMessage `json:"skill_install_suggested,omitempty"`
}

func main() {
	expectChannel := flag.String("expect-channel", "", "Assert the channel field equals this value (e.g. 'pre-release'). Mutually exclusive with --schema-only.")
	crossCheck := flag.String("cross-check", "", "Assert the latest field equals this tag (cross-check against `gh release list`). Mutually exclusive with --schema-only.")
	schemaOnly := flag.Bool("schema-only", false, "Skip value-coupling; enforce shape + enum membership only. Mutually exclusive with --expect-channel / --cross-check.")
	flag.Parse()

	// Mutex check BEFORE reading stdin so misuse is caught regardless of
	// pipeline state.
	if *schemaOnly && (*expectChannel != "" || *crossCheck != "") {
		fail("--schema-only is mutually exclusive with --expect-channel and --cross-check")
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail("read stdin: %v", err)
	}
	if len(raw) == 0 {
		fail("empty stdin — expected JSON document")
	}

	var doc resultDoc
	dec := json.NewDecoder(newRawReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		// Re-emit the input so the CI log shows what we tried to parse.
		fail("parse JSON: %v\n--- input ---\n%s\n--- end input ---", err, string(raw))
	}

	// Required fields.
	if doc.Current == "" {
		fail("`current` is empty")
	}
	if doc.Latest == "" {
		fail("`latest` is empty")
	}
	if !semver.IsValid(doc.Latest) {
		fail("`latest` is not a valid semver tag: %q", doc.Latest)
	}
	if doc.Channel == "" {
		fail("`channel` is empty")
	}
	if doc.InstallMethod == "" {
		fail("`install_method` is empty")
	}

	// `check` subcommand semantics.
	if doc.Updated {
		fail("`updated` must be false from the `check` subcommand, got true")
	}
	if !doc.Check {
		fail("`check` must be true from the `check` subcommand, got false")
	}

	// Post-swap-only fields must NOT be present from the `check` subcommand (no swap occurred).
	if len(doc.UpdatedSkills) > 0 {
		fail("`updated_skills` must be absent from the `check` subcommand, got %s", string(doc.UpdatedSkills))
	}
	if len(doc.SkillInstallSuggested) > 0 {
		fail("`skill_install_suggested` must be absent from the `check` subcommand, got %s", string(doc.SkillInstallSuggested))
	}

	if *schemaOnly {
		// Schema-only: enforce enum membership but no value coupling. The
		// hermetic Tier-1 step exercises specific values via --expect-channel
		// / --cross-check; this branch is for the live-API smoke that runs
		// against a moving release feed.
		if _, ok := validChannels[doc.Channel]; !ok {
			fail("`channel` %q not in enum {stable, pre-release}", doc.Channel)
		}
		if _, ok := validInstallMethods[doc.InstallMethod]; !ok {
			fail("`install_method` %q not in enum {binary, homebrew, npm, pipx, uv}", doc.InstallMethod)
		}
	} else {
		// Optional flag-driven value assertions.
		if *expectChannel != "" && doc.Channel != *expectChannel {
			fail("expected channel %q, got %q", *expectChannel, doc.Channel)
		}
		if *crossCheck != "" && doc.Latest != *crossCheck {
			fail("expected latest %q (cross-checked from `gh release list`), got %q — filter drift", *crossCheck, doc.Latest)
		}
	}

	fmt.Fprintf(os.Stderr, "ok: current=%s latest=%s channel=%s install_method=%s\n",
		doc.Current, doc.Latest, doc.Channel, doc.InstallMethod)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "check_json: "+format+"\n", args...)
	os.Exit(1)
}

// newRawReader wraps a byte slice in an io.Reader so we can hand it to
// json.NewDecoder. We don't use bytes.NewReader directly so the helper stays
// dependency-light and the failure message can re-emit the raw input verbatim.
func newRawReader(b []byte) io.Reader { return &byteReader{b: b} }

type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
