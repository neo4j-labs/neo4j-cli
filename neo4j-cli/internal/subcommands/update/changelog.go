// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]
//
// This file is part of Neo4j.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package update

import (
	"strings"

	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const (
	// changelogURL is the URL to the full release history shown when
	// the changelog elides entries or the binary is too stale to range over.
	changelogURL = "https://github.com/neo4j-labs/neo4j-cli/releases"

	// maxChangelogEntries is the maximum number of release notes entries
	// printed after a successful update.
	maxChangelogEntries = 3
)

// releaseNotesEntry holds a single release's tag and trimmed body
// for rendering in the changelog.
type releaseNotesEntry struct {
	tag  string
	body string
}

// trimReleaseBody cuts the release body at the first line that is exactly "---",
// dropping the boilerplate sections (## Versions / ## Changes) that follow it.
// If the trimmed portion is blank or no "---" is found, the original body is
// returned unchanged. A leading "## Release notes" heading is also stripped
// since it would sit redundantly beneath the caller's own "## <tag>" header.
func trimReleaseBody(body string) string {
	if body == "" {
		return ""
	}

	// Find the first line that is exactly "---".
	idx := strings.Index(body, "\n---\n")
	if idx == -1 {
		// Also try at the very start (no leading newline).
		if !strings.HasPrefix(body, "---\n") {
			return body
		}
		idx = 0
	}

	// Take everything before the --- marker.
	var trimmed string
	if idx == 0 {
		trimmed = ""
	} else {
		trimmed = strings.TrimRight(body[:idx], "\n\r\t ")
	}

	// Strip a leading "## Release notes" heading.
	trimmed = stripReleaseNotesHeading(trimmed)

	// If the remainder is blank, return the original body.
	if strings.TrimSpace(trimmed) == "" {
		return body
	}

	return trimmed
}

// changelogForRange filters releases to the exclusive-inclusive range
// (current, target], newest first, capped at maxChangelogEntries. An empty
// body entry is skipped and does not consume a cap slot. elided is set true
// when the cap truncates the list or when current is not found in releases
// (stale binary more than 30 releases behind).
func changelogForRange(releases []Release, current, target string) (entries []releaseNotesEntry, elided bool) {
	// A downgrade yields an empty range by construction — no special branch.
	if semver.Compare(target, current) <= 0 {
		return nil, false
	}

	// Track whether current appears anywhere in the fetched releases
	// (not just the range). A missing current means stale binary.
	currentFound := false
	for _, r := range releases {
		if semver.Compare(r.TagName, current) == 0 {
			currentFound = true
			break
		}
	}

	for _, r := range releases {
		tag := r.TagName
		if semver.Compare(tag, current) <= 0 {
			continue
		}
		if semver.Compare(tag, target) > 0 {
			continue
		}

		// Skip empty-body entries — they don't consume a cap slot.
		if r.Body == "" {
			continue
		}

		if len(entries) >= maxChangelogEntries {
			elided = true
			continue
		}

		entries = append(entries, releaseNotesEntry{
			tag:  tag,
			body: trimReleaseBody(r.Body),
		})
	}

	// When current is not in the fetched releases (stale binary), always elide.
	if !currentFound && len(entries) > 0 {
		elided = true
	}

	return entries, elided
}

// printChangelog writes the release notes entries to stdout via cmd.Printf.
// Each entry renders as "## <tag>", a blank line, then the body (passed through
// output.StripControl), with a blank line between entries. When elided, a
// trailing "Full changelog: <changelogURL>" line is appended.
func printChangelog(cmd *cobra.Command, entries []releaseNotesEntry, elided bool) {
	for i, e := range entries {
		if i > 0 {
			cmd.Printf("\n")
		}
		cmd.Printf("## %s\n\n%s\n", e.tag, output.StripControl(e.body))
	}

	if elided {
		cmd.Printf("\nFull changelog: %s\n", changelogURL)
	}
}

// stripReleaseNotesHeading removes a leading "## Release notes" heading
// (with optional trailing whitespace) from s.
func stripReleaseNotesHeading(s string) string {
	const heading = "## Release notes"

	if !strings.HasPrefix(s, heading) {
		return s
	}

	rest := strings.TrimPrefix(s, heading)
	// Strip the trailing newline (or end of string if heading was the whole thing).
	rest = strings.TrimLeft(rest, "\n\r\t ")
	return rest
}
