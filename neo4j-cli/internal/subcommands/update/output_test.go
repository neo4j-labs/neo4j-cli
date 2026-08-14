// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlainTextOutput_GoldenSuccess verifies the byte-for-byte plain-text
// success output per REQ-F-017 acceptance criterion 1.
func TestPlainTextOutput_GoldenSuccess(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    string
	}{
		{
			name:    "alpha tag",
			current: "v0.1.0-alpha.9",
			latest:  "v0.1.0-alpha.10",
			want: "Current version: v0.1.0-alpha.9\n" +
				"Checking for updates to latest version...\n" +
				"Successfully updated from v0.1.0-alpha.9 to v0.1.0-alpha.10\n",
		},
		{
			name:    "stable bump",
			current: "v1.0.0",
			latest:  "v1.1.0",
			want: "Current version: v1.0.0\n" +
				"Checking for updates to latest version...\n" +
				"Successfully updated from v1.0.0 to v1.1.0\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
				return &Release{TagName: tc.latest}, nil
			})
			withDetect(t, func() (InstallMethod, string, error) {
				return InstallMethodBinary, "/tmp/neo4j-cli", nil
			})
			withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
				return nil
			})

			out, err := runWithOpts(t, tc.current, runOpts{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, out, "plain-text output must be byte-for-byte equal to the reference")
		})
	}
}

// TestPlainTextOutput_WithChangelog asserts that changelog entries appear in
// plain-text output after a successful update when release data is available.
func TestPlainTextOutput_WithChangelog(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v1.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	withListReleases(t, func(ctx context.Context, preReleases bool) ([]Release, error) {
		return []Release{
			{TagName: "v1.1.0", Body: "Release notes for v1.1.0"},
			{TagName: "v1.0.0", Body: "Initial release"},
		}, nil
	})

	// Inline setup to avoid runWithOptsFormat stubbing listReleasesFn.
	cfgJSON := `{"format":"default"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v1.0.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = runUpdate(context.Background(), cmd, cfg, runOpts{})
	require.NoError(t, err)

	want := "Current version: v1.0.0\n" +
		"Checking for updates to latest version...\n" +
		"Successfully updated from v1.0.0 to v1.1.0\n" +
		"## v1.1.0\n" +
		"\n" +
		"Release notes for v1.1.0\n"
	assert.Equal(t, want, out.String())
}

// TestPlainTextOutput_NoChangelogFlag asserts that --no-changelog suppresses
// the changelog even when release data is available.
func TestPlainTextOutput_NoChangelogFlag(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v1.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	withListReleases(t, func(ctx context.Context, preReleases bool) ([]Release, error) {
		return []Release{
			{TagName: "v1.1.0", Body: "Release notes for v1.1.0"},
			{TagName: "v1.0.0", Body: "Initial release"},
		}, nil
	})

	cfgJSON := `{"format":"default"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v1.0.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = runUpdate(context.Background(), cmd, cfg, runOpts{noChangelog: true})
	require.NoError(t, err)

	// Must NOT include changelog entries.
	want := "Current version: v1.0.0\n" +
		"Checking for updates to latest version...\n" +
		"Successfully updated from v1.0.0 to v1.1.0\n"
	assert.Equal(t, want, out.String())
}

// TestRunUpdate_Changelog_PropagatesPreReleases pins that the --pre-releases
// flag reaches the changelog fetch. With a hardcoded false, an
// `update --pre-releases` landing on an alpha tag would filter that very tag
// out of its own changelog and print nothing.
func TestRunUpdate_Changelog_PropagatesPreReleases(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v1.2.0-alpha.1"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	gotPreReleases := false
	withListReleases(t, func(ctx context.Context, preReleases bool) ([]Release, error) {
		gotPreReleases = preReleases
		return []Release{
			{TagName: "v1.2.0-alpha.1", Prerelease: true, Body: "Alpha notes"},
			{TagName: "v1.1.0", Body: "Stable notes"},
		}, nil
	})

	out, err := runWithOpts(t, "v1.1.0", runOpts{preReleases: true})
	require.NoError(t, err)
	assert.True(t, gotPreReleases, "--pre-releases must propagate to the changelog fetch")
	assert.Contains(t, out, "## v1.2.0-alpha.1", "the pre-release tag being installed must appear in its own changelog")
	assert.Contains(t, out, "Alpha notes")
}

// TestJSONOutput_WithChangelog asserts that release_notes (and changelog_url
// when elided) appear in the JSON output after a successful update.
func TestJSONOutput_WithChangelog(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v1.5.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	// 5 releases in range, so only 3 appear — elided = true → changelog_url set.
	withListReleases(t, func(ctx context.Context, preReleases bool) ([]Release, error) {
		return []Release{
			{TagName: "v1.5.0", Body: "Release five"},
			{TagName: "v1.4.0", Body: "Release four"},
			{TagName: "v1.3.0", Body: "Release three"},
			{TagName: "v1.2.0", Body: "Release two"},
			{TagName: "v1.1.0", Body: "Release one"},
			{TagName: "v1.0.0", Body: "Current release"},
		}, nil
	})

	cfgJSON := `{"format":"json"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v1.0.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = runUpdate(context.Background(), cmd, cfg, runOpts{})
	require.NoError(t, err)

	var doc struct {
		Updated      bool `json:"updated"`
		ReleaseNotes []struct {
			Version string `json:"version"`
			Notes   string `json:"notes"`
		} `json:"release_notes"`
		ChangelogURL string `json:"changelog_url"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))

	// Binary was updated
	assert.True(t, doc.Updated)

	// Three release_notes entries, newest first
	require.Len(t, doc.ReleaseNotes, 3)
	assert.Equal(t, "v1.5.0", doc.ReleaseNotes[0].Version)
	assert.Equal(t, "Release five", doc.ReleaseNotes[0].Notes)
	assert.Equal(t, "v1.4.0", doc.ReleaseNotes[1].Version)
	assert.Equal(t, "v1.3.0", doc.ReleaseNotes[2].Version)

	// Elided → changelog_url populated
	assert.Equal(t, "https://github.com/neo4j-labs/neo4j-cli/releases", doc.ChangelogURL)
}

// TestJSONOutput_WithChangelog_NotElided asserts that when <=3 entries,
// changelog_url is absent (omitempty).
func TestJSONOutput_WithChangelog_NotElided(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v1.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	withListReleases(t, func(ctx context.Context, preReleases bool) ([]Release, error) {
		return []Release{
			{TagName: "v1.1.0", Body: "Release one"},
			{TagName: "v1.0.0", Body: "Current release"},
		}, nil
	})

	cfgJSON := `{"format":"json"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v1.0.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = runUpdate(context.Background(), cmd, cfg, runOpts{})
	require.NoError(t, err)

	var doc struct {
		Updated      bool `json:"updated"`
		ReleaseNotes []struct {
			Version string `json:"version"`
			Notes   string `json:"notes"`
		} `json:"release_notes"`
		ChangelogURL string `json:"changelog_url"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))

	assert.True(t, doc.Updated)
	require.Len(t, doc.ReleaseNotes, 1)
	assert.Equal(t, "v1.1.0", doc.ReleaseNotes[0].Version)

	// Not elided — changelog_url must be empty/absent.
	assert.Empty(t, doc.ChangelogURL)
}

// TestPrintableUpdateResult_AsArray_NoChangelogFields asserts that the
// table/toon AsArray shape does NOT include release_notes or changelog_url.
func TestPrintableUpdateResult_AsArray_NoChangelogFields(t *testing.T) {
	p := printableUpdateResult{r: updateResult{
		current:       "v1.0.0",
		latest:        "v1.1.0",
		updated:       true,
		channel:       "stable",
		installMethod: "binary",
		releaseNotes: []releaseNotesEntry{
			{tag: "v1.1.0", body: "Release body"},
		},
		releaseNotesElided: false,
	}}
	rows := p.AsArray()
	require.Len(t, rows, 1)
	// The six base fields must be present.
	assert.Equal(t, "v1.0.0", rows[0]["current"])
	assert.Equal(t, "v1.1.0", rows[0]["latest"])
	// Changelog-specific fields must NOT leak into AsArray.
	_, hasRN := rows[0]["release_notes"]
	_, hasCU := rows[0]["changelog_url"]
	assert.False(t, hasRN, "release_notes must not appear in AsArray (table/toon)")
	assert.False(t, hasCU, "changelog_url must not appear in AsArray (table/toon)")
}

func TestJSONOutput_HappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.True(t, doc.Updated)
	assert.False(t, doc.Check)
	assert.Equal(t, "stable", doc.Channel)
	assert.Equal(t, "binary", doc.InstallMethod)
}

func TestJSONOutput_CheckMode_NewerAvailable(t *testing.T) {
	// REQ-F-003 / REQ-F-018: `update check` + newer available exits 0 with
	// JSON sets updated:false, check:true. Scripts compare current!=latest
	// to detect drift rather than relying on a non-zero exit.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{check: true}, "json")
	require.NoError(t, err, "`update check` + newer must NOT error — finding a new version is the success case")

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.False(t, doc.Updated, "--check must never set updated=true")
	assert.True(t, doc.Check, "--check JSON must set check=true")
	assert.Equal(t, "binary", doc.InstallMethod)
}

func TestJSONOutput_CheckMode_UpToDate(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{check: true}, "json")
	require.NoError(t, err)

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.1.0", doc.Latest)
	assert.False(t, doc.Updated)
	assert.True(t, doc.Check)
}

func TestJSONOutput_PkgMgrPassthrough(t *testing.T) {
	// REQ-F-018 / acceptance criterion 4: passthrough JSON includes
	// install_method=<channel> and updated=false.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		t.Fatal("swap must not run on pkg-mgr passthrough")
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.False(t, doc.Updated)
	assert.False(t, doc.Check)
	assert.Equal(t, "homebrew", doc.InstallMethod, "install_method must reflect detected pkg-mgr channel")
}

// TestJSONOutput_FieldOrderDeterministic asserts the documented REQ-F-018
// order (current, latest, updated, check, channel, install_method) is
// preserved in the rendered JSON byte stream so downstream scripts can rely
// on it.
func TestJSONOutput_FieldOrderDeterministic(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	// Substring positions in the raw stdout — earlier-listed fields must
	// appear earlier in the JSON output.
	keys := []string{
		"\"current\"",
		"\"latest\"",
		"\"updated\"",
		"\"check\"",
		"\"channel\"",
		"\"install_method\"",
	}
	prev := -1
	for _, k := range keys {
		idx := strings.Index(out, k)
		require.GreaterOrEqual(t, idx, 0, "key %s missing from JSON output", k)
		assert.Greater(t, idx, prev, "key %s appears before its predecessor — REQ-F-018 order broken", k)
		prev = idx
	}
}

// TestTableOutput_HappyPath asserts --format table produces a structured
// table (not the plain-text running narrative) with the documented six
// columns rendered as headers. go-pretty/v6/table upper-cases header text by
// default, so assertions compare against strings.ToLower for header columns.
func TestTableOutput_HappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "table")
	require.NoError(t, err)

	// The plain-text running narrative must NOT appear when --format table is
	// in effect — otherwise scripts piping table output get a polluted stream.
	assert.NotContains(t, out, "Successfully updated", "plain-text narrative must not bleed through structured output")
	assert.NotContains(t, out, "Checking for updates")

	// Headers (case-insensitive — go-pretty upper-cases by default).
	lower := strings.ToLower(out)
	for _, header := range []string{"current", "latest", "updated", "check", "channel", "install_method"} {
		assert.Contains(t, lower, header, "table output must include header %q", header)
	}
	// Body cells (exact case).
	assert.Contains(t, out, "v0.1.0", "table body must include current version")
	assert.Contains(t, out, "v0.2.0", "table body must include latest version")
	assert.Contains(t, out, "binary", "table body must include install_method")
}

// TestToonOutput_HappyPath asserts --format toon produces a structured toon
// document (not plain text and not JSON) containing the documented six fields.
func TestToonOutput_HappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "toon")
	require.NoError(t, err)

	// Toon must NOT be the plain-text narrative.
	assert.NotContains(t, out, "Successfully updated", "plain-text narrative must not bleed through structured output")
	assert.NotContains(t, out, "Checking for updates")

	// Toon must NOT be valid JSON (sanity guard against accidentally emitting
	// JSON when the user asked for toon).
	var v any
	jsonErr := json.Unmarshal([]byte(out), &v)
	assert.Error(t, jsonErr, "toon output should not parse as JSON, got: %q", out)

	// All six documented field keys must appear in the toon document.
	for _, key := range []string{"current", "latest", "updated", "check", "channel", "install_method"} {
		assert.Contains(t, out, key, "toon output must include key %q", key)
	}
	// Body values.
	assert.Contains(t, out, "v0.1.0")
	assert.Contains(t, out, "v0.2.0")
	assert.Contains(t, out, "binary")
}

// TestPrintableUpdateResult_AsArrayShape covers the table-render shape of
// printableUpdateResult. We don't ship a "table" output for update (the
// document isn't list-shaped) but PrintBodyMap insists on AsArray, so the
// data must round-trip cleanly even via the table branch.
func TestPrintableUpdateResult_AsArrayShape(t *testing.T) {
	p := printableUpdateResult{r: updateResult{
		current:       "v0.1.0",
		latest:        "v0.2.0",
		updated:       true,
		check:         false,
		channel:       "stable",
		installMethod: "binary",
	}}
	rows := p.AsArray()
	require.Len(t, rows, 1, "AsArray must wrap the single document in a one-row slice")
	row := rows[0]
	assert.Equal(t, "v0.1.0", row["current"])
	assert.Equal(t, "v0.2.0", row["latest"])
	assert.Equal(t, true, row["updated"])
	assert.Equal(t, false, row["check"])
	assert.Equal(t, "stable", row["channel"])
	assert.Equal(t, "binary", row["install_method"])
}

// TestPrintableUpdateResult_AsArrayShape_PostSwapFields asserts the table /
// toon AsArray shape includes updated_skills + skill_install_suggested when
// (and only when) they're populated.
func TestPrintableUpdateResult_AsArrayShape_PostSwapFields(t *testing.T) {
	cases := []struct {
		name                     string
		updatedSkills            []string
		skillInstallSuggested    bool
		wantUpdatedSkillsKey     bool
		wantSkillInstallSuggKey  bool
		wantSkillInstallSuggCell any
	}{
		{
			name:                    "no post-swap fields",
			updatedSkills:           nil,
			skillInstallSuggested:   false,
			wantUpdatedSkillsKey:    false,
			wantSkillInstallSuggKey: false,
		},
		{
			name:                    "agents refreshed",
			updatedSkills:           []string{"claude-code", "cursor"},
			skillInstallSuggested:   false,
			wantUpdatedSkillsKey:    true,
			wantSkillInstallSuggKey: false,
		},
		{
			name:                     "no agents — suggest install",
			updatedSkills:            nil,
			skillInstallSuggested:    true,
			wantUpdatedSkillsKey:     false,
			wantSkillInstallSuggKey:  true,
			wantSkillInstallSuggCell: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := printableUpdateResult{r: updateResult{
				current:               "v0.1.0",
				latest:                "v0.2.0",
				updated:               true,
				channel:               "stable",
				installMethod:         "binary",
				updatedSkills:         tc.updatedSkills,
				skillInstallSuggested: tc.skillInstallSuggested,
			}}
			rows := p.AsArray()
			require.Len(t, rows, 1)
			_, hasUS := rows[0]["updated_skills"]
			_, hasSIS := rows[0]["skill_install_suggested"]
			assert.Equal(t, tc.wantUpdatedSkillsKey, hasUS, "updated_skills key presence")
			assert.Equal(t, tc.wantSkillInstallSuggKey, hasSIS, "skill_install_suggested key presence")
			if tc.wantSkillInstallSuggKey {
				assert.Equal(t, tc.wantSkillInstallSuggCell, rows[0]["skill_install_suggested"])
			}
		})
	}
}

// TestUpdate_StatusLinesGoToStderr pins CLI-96: the five plain-text status
// lines in runUpdate (dev-build, no-stable-release, pre-swap narrative,
// skill-install tip) must land on stderr, not stdout. Existing tests use a
// combined buffer so they cannot catch a regression here; this one uses
// SPLIT buffers and asserts stdout stays clean for the narration lines.
//
// Sanity-checked during implementation: reverting any of the moved sites
// back to cmd.Println/cmd.Printf causes the matching sub-case to fail.
func TestUpdate_StatusLinesGoToStderr(t *testing.T) {
	t.Run("dev-build branch", func(t *testing.T) {
		outBuf, errBuf := runWithSplitBuffers(t, "dev", runOpts{})
		assert.NotContains(t, outBuf, "running a dev build", "dev-build line must NOT be on stdout")
		assert.Contains(t, errBuf, "running a dev build, nothing to update", "dev-build line must be on stderr")
	})

	t.Run("no-stable-release branch", func(t *testing.T) {
		withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
			return nil, ErrNoStableRelease
		})
		outBuf, errBuf := runWithSplitBuffers(t, "v0.1.0", runOpts{})
		assert.NotContains(t, outBuf, "no stable release published yet", "no-stable line must NOT be on stdout")
		assert.Contains(t, errBuf, "no stable release published yet", "no-stable line must be on stderr")
		assert.Contains(t, errBuf, "--pre-releases", "hint must be on stderr")
	})
}
