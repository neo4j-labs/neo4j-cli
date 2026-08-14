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
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrimReleaseBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "trims at --- and strips ## Release notes heading",
			body: "## Release notes\n\n### Highlights\n- Feature X\n\n---\n\n## Versions\n- v1.1.0",
			want: "### Highlights\n- Feature X",
		},
		{
			name: "returns original when no --- found",
			body: "## Release notes\n\n### Highlights\n- Feature X",
			want: "## Release notes\n\n### Highlights\n- Feature X",
		},
		{
			name: "returns original when remainder is blank after ---",
			body: "## Release notes\n\n---\n\n## Versions\n- v1.1.0",
			want: "## Release notes\n\n---\n\n## Versions\n- v1.1.0",
		},
		{
			name: "returns original when body is only boilerplate after ---",
			body: "---\n\n## Versions\n- v1.1.0",
			want: "---\n\n## Versions\n- v1.1.0",
		},
		{
			name: "empty body returns empty",
			body: "",
			want: "",
		},
		{
			name: "trims content before --- with no heading",
			body: "Some release notes\n\n---\n\n## Changes\n- v1.1.0",
			want: "Some release notes",
		},
		{
			name: "strips ## Release notes heading and trims at ---",
			body: "## Release notes\nSome content\n\n---\n\n## Versions",
			want: "Some content",
		},
		{
			name: "body with multiple --- trims at first only",
			body: "Top section\n\n---\n\nMiddle section\n\n---\n\nBottom",
			want: "Top section",
		},
		{
			name: "heading with trailing spaces stripped",
			body: "## Release notes   \n\nContent\n\n---\n\nBoilerplate",
			want: "Content",
		},
		{
			name: "body with only whitespace after trimming returns original",
			body: "## Release notes\n\n   \n\n---\n\n## Versions",
			want: "## Release notes\n\n   \n\n---\n\n## Versions",
		},
		{
			name: "content after heading no --- returns original",
			body: "## Release notes\n\nSome content here",
			want: "## Release notes\n\nSome content here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimReleaseBody(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestChangelogForRange(t *testing.T) {
	mkRelease := func(tag, body string) Release {
		return Release{TagName: tag, Body: body}
	}

	// Releases API order: newest first.
	fourReleasesWithCurrent := []Release{
		mkRelease("v1.3.0", "Release three body"),
		mkRelease("v1.2.0", "Release two body"),
		mkRelease("v1.1.0", "Release one body"),
		mkRelease("v1.0.0", "current body"),
	}

	sixReleases := []Release{
		mkRelease("v1.5.0", "Release five body"),
		mkRelease("v1.4.0", "Release four body"),
		mkRelease("v1.3.0", "Release three body"),
		mkRelease("v1.2.0", "Release two body"),
		mkRelease("v1.1.0", "Release one body"),
		mkRelease("v1.0.0", "current body"),
	}

	withEmptyBody := []Release{
		mkRelease("v1.4.0", "Release four body"),
		mkRelease("v1.3.0", ""), // empty body — skip, no cap consumed
		mkRelease("v1.2.0", "Release two body"),
		mkRelease("v1.1.0", ""), // empty body — skip, no cap consumed
		mkRelease("v1.0.0", "current body"),
	}

	tests := []struct {
		name     string
		releases []Release
		current  string
		target   string
		wantTags []string
		elided   bool
	}{
		{
			name:     "3-version range returns 3 entries, no elide",
			releases: fourReleasesWithCurrent,
			current:  "v1.0.0",
			target:   "v1.3.0",
			wantTags: []string{"v1.3.0", "v1.2.0", "v1.1.0"},
			elided:   false,
		},
		{
			name:     "range wider than 3 returns 3 newest, elided",
			releases: sixReleases,
			current:  "v1.0.0",
			target:   "v1.5.0",
			wantTags: []string{"v1.5.0", "v1.4.0", "v1.3.0"},
			elided:   true,
		},
		{
			name:     "current absent from releases sets elided (stale binary)",
			releases: sixReleases,
			current:  "v0.5.0",
			target:   "v1.5.0",
			wantTags: []string{"v1.5.0", "v1.4.0", "v1.3.0"},
			elided:   true,
		},
		{
			name:     "empty body entries skipped, don't consume cap slot",
			releases: withEmptyBody,
			current:  "v1.0.0",
			target:   "v1.4.0",
			wantTags: []string{"v1.4.0", "v1.2.0"},
			elided:   false,
		},
		{
			name:     "downgrade returns zero entries",
			releases: fourReleasesWithCurrent,
			current:  "v1.3.0",
			target:   "v1.0.0",
			wantTags: nil,
			elided:   false,
		},
		{
			name:     "same version returns zero entries",
			releases: fourReleasesWithCurrent,
			current:  "v1.2.0",
			target:   "v1.2.0",
			wantTags: nil,
			elided:   false,
		},
		{
			name:     "single version bump returns one entry, no elide",
			releases: fourReleasesWithCurrent,
			current:  "v1.2.0",
			target:   "v1.3.0",
			wantTags: []string{"v1.3.0"},
			elided:   false,
		},
		{
			name:     "exactly 3 in range, current found, no elide",
			releases: fourReleasesWithCurrent,
			current:  "v1.0.0",
			target:   "v1.3.0",
			wantTags: []string{"v1.3.0", "v1.2.0", "v1.1.0"},
			elided:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, elided := changelogForRange(tt.releases, tt.current, tt.target)
			assert.Equal(t, tt.elided, elided)

			if tt.wantTags == nil {
				assert.Empty(t, entries)
				return
			}

			require.Len(t, entries, len(tt.wantTags))
			for i, e := range entries {
				assert.Equal(t, tt.wantTags[i], e.tag, "entry %d tag", i)
			}
		})
	}
}

func TestPrintChangelog(t *testing.T) {
	mkEntry := func(tag, body string) releaseNotesEntry {
		return releaseNotesEntry{tag: tag, body: body}
	}

	tests := []struct {
		name    string
		entries []releaseNotesEntry
		elided  bool
		want    string
	}{
		{
			name: "single entry, no elide",
			entries: []releaseNotesEntry{
				mkEntry("v1.1.0", "Normal release body"),
			},
			elided: false,
			want:   "## v1.1.0\n\nNormal release body\n",
		},
		{
			name: "multiple entries with blank line separator",
			entries: []releaseNotesEntry{
				mkEntry("v1.2.0", "Second release"),
				mkEntry("v1.1.0", "First release"),
			},
			elided: false,
			want:   "## v1.2.0\n\nSecond release\n\n## v1.1.0\n\nFirst release\n",
		},
		{
			name: "elided appends full changelog link",
			entries: []releaseNotesEntry{
				mkEntry("v1.1.0", "Release body"),
			},
			elided: true,
			want:   "## v1.1.0\n\nRelease body\n\nFull changelog: https://github.com/neo4j-labs/neo4j-cli/releases\n",
		},
		{
			name: "ANSI escapes neutralised in body",
			entries: []releaseNotesEntry{
				mkEntry("v1.1.0", "Normal \x1b[31mred\x1b[0m text"),
			},
			elided: false,
			want:   "## v1.1.0\n\nNormal ?[31mred?[0m text\n",
		},
		{
			name: "C0 control bytes neutralised in body",
			entries: []releaseNotesEntry{
				mkEntry("v1.1.0", "Line1\x00null\x01body"),
			},
			elided: false,
			want:   "## v1.1.0\n\nLine1?null?body\n",
		},
		{
			name:    "no entries, elided false prints nothing",
			entries: nil,
			elided:  false,
			want:    "",
		},
		{
			name:    "no entries, elided true prints link only",
			entries: nil,
			elided:  true,
			want:    "\nFull changelog: https://github.com/neo4j-labs/neo4j-cli/releases\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			printChangelog(cmd, tt.entries, tt.elided)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestReleaseNotesEntry_IsUnexported(t *testing.T) {
	// This test verifies that releaseNotesEntry is unexported by asserting
	// it can be used from the same package (not imported from outside). The
	// acceptance criterion is that common/output/casing_gate_test.go needs
	// no allowlist entry — an unexported struct is invisible to its reflection.
	var e releaseNotesEntry
	assert.Empty(t, e.tag)
	assert.Empty(t, e.body)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "https://github.com/neo4j-labs/neo4j-cli/releases", changelogURL)
	assert.Equal(t, 3, maxChangelogEntries)
}
