// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pinNow fixes the clock seam for the duration of a test so generated_at is
// deterministic.
func pinNow(t *testing.T, now time.Time) {
	t.Helper()
	orig := nowFn
	nowFn = func() time.Time { return now }
	t.Cleanup(func() { nowFn = orig })
}

func TestParseAPIReleases(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr string
	}{
		{
			name: "single page array",
			raw:  `[{"tag_name":"v1.0.0","name":"v1.0.0","draft":false,"prerelease":false,"published_at":"2026-01-01T00:00:00Z","body":"notes"}]`,
			want: 1,
		},
		{
			name: "paginated arrays concatenated back to back",
			raw: `[{"tag_name":"v1.1.0","name":"v1.1.0","draft":false,"prerelease":false,"published_at":"2026-02-01T00:00:00Z","body":"a"}]` +
				`[{"tag_name":"v1.0.0","name":"v1.0.0","draft":false,"prerelease":false,"published_at":"2026-01-01T00:00:00Z","body":"b"}]`,
			want: 2,
		},
		{
			name:    "empty response",
			raw:     "",
			wantErr: "empty API response",
		},
		{
			name:    "malformed JSON",
			raw:     `[{"tag_name":`,
			wantErr: "parse API response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAPIReleases([]byte(tt.raw))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("expected %d releases, got %d", tt.want, len(got))
			}
		})
	}
}

func TestBuildDocument(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	t.Run("drafts excluded, semver and non-semver tags kept, body verbatim, newest first", func(t *testing.T) {
		body := "## Release notes\n\n### Minor\n\n- Entry with `code` and trailing newline\n"
		api := []apiRelease{
			{TagName: "v1.16.0", Name: "v1.16.0", Prerelease: false, PublishedAt: "2026-09-01T09:00:00Z", Body: "old"},
			{TagName: "v9.9.9-draft", Name: "WIP", Draft: true, PublishedAt: "2026-10-16T00:00:00Z", Body: "secret"},
			{TagName: "v1.17.0", Name: "v1.17.0", Prerelease: true, PublishedAt: "2026-10-15T10:00:00Z", Body: body},
			{TagName: "nightly-2026-10", Name: "Nightly", Prerelease: true, PublishedAt: "2026-10-10T00:00:00Z", Body: "nightly"},
		}
		doc, err := buildDocument(api, "neo4j-labs/neo4j-cli", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.SchemaVersion != 1 {
			t.Errorf("schema_version = %d, want 1", doc.SchemaVersion)
		}
		if doc.GeneratedAt != "2026-10-15T12:00:00Z" {
			t.Errorf("generated_at = %q, want 2026-10-15T12:00:00Z", doc.GeneratedAt)
		}
		if doc.Repository != "neo4j-labs/neo4j-cli" {
			t.Errorf("repository = %q", doc.Repository)
		}
		wantTags := []string{"v1.17.0", "nightly-2026-10", "v1.16.0"}
		if len(doc.Releases) != len(wantTags) {
			t.Fatalf("releases = %d, want %d (draft must be excluded)", len(doc.Releases), len(wantTags))
		}
		for i, want := range wantTags {
			if doc.Releases[i].Tag != want {
				t.Errorf("releases[%d].tag = %q, want %q", i, doc.Releases[i].Tag, want)
			}
		}
		if doc.Releases[0].Body != body {
			t.Errorf("body not verbatim:\n got %q\nwant %q", doc.Releases[0].Body, body)
		}
		if !doc.Releases[0].Prerelease {
			t.Errorf("prerelease flag not carried through")
		}
	})

	t.Run("ordering compares instants, not strings (RFC3339 offsets)", func(t *testing.T) {
		api := []apiRelease{
			{TagName: "v1.0.0", Name: "a", PublishedAt: "2026-10-15T12:00:00+02:00"}, // 10:00Z
			{TagName: "v1.1.0", Name: "b", PublishedAt: "2026-10-15T11:00:00Z"},      // 11:00Z, newer
		}
		doc, err := buildDocument(api, "neo4j-labs/neo4j-cli", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.Releases[0].Tag != "v1.1.0" {
			t.Errorf("releases[0].tag = %q, want v1.1.0 (11:00Z > 10:00Z)", doc.Releases[0].Tag)
		}
	})

	t.Run("invalid published_at is an error", func(t *testing.T) {
		api := []apiRelease{{TagName: "v1.0.0", Name: "a", PublishedAt: "soon"}}
		if _, err := buildDocument(api, "neo4j-labs/neo4j-cli", now); err == nil {
			t.Fatal("expected error for non-RFC3339 published_at")
		}
	})

	t.Run("empty tag is an error", func(t *testing.T) {
		api := []apiRelease{{TagName: "", Name: "a", PublishedAt: "2026-01-01T00:00:00Z"}}
		if _, err := buildDocument(api, "neo4j-labs/neo4j-cli", now); err == nil {
			t.Fatal("expected error for empty tag_name")
		}
	})

	t.Run("no releases still emits an empty array, not null", func(t *testing.T) {
		doc, err := buildDocument(nil, "neo4j-labs/neo4j-cli", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		raw, err := render(doc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(raw), `"releases": []`) {
			t.Errorf("expected empty array in output, got:\n%s", raw)
		}
	})
}

// TestRun_InputDryRun drives the command hermetically: canned `gh api`
// response from testdata in, document on stdout out.
func TestRun_InputDryRun(t *testing.T) {
	pinNow(t, time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))

	var buf bytes.Buffer
	err := run([]string{
		"--input", filepath.Join("testdata", "gh-api-releases.json"),
		"--dry-run",
	}, &buf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var doc document
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, buf.String())
	}
	if doc.SchemaVersion != 1 || doc.Repository != "neo4j-labs/neo4j-cli" || doc.GeneratedAt != "2026-10-15T12:00:00Z" {
		t.Errorf("unexpected header: %+v", doc)
	}
	wantTags := []string{"v1.17.0", "nightly-2026-10", "v1.16.0"}
	if len(doc.Releases) != len(wantTags) {
		t.Fatalf("releases = %d, want %d (draft v9.9.9-draft excluded)", len(doc.Releases), len(wantTags))
	}
	for i, want := range wantTags {
		if doc.Releases[i].Tag != want {
			t.Errorf("releases[%d].tag = %q, want %q", i, doc.Releases[i].Tag, want)
		}
	}
	wantBody := "## Release notes\n\n### Minor\n\n- Example entry (raw GitHub release markdown, verbatim)\n\n```sh\nneo4j-cli update check\n```\n"
	if doc.Releases[0].Body != wantBody {
		t.Errorf("body not verbatim:\n got %q\nwant %q", doc.Releases[0].Body, wantBody)
	}
}

// TestRun_IdempotentRewrite proves the whole-file rewrite is byte-identical
// when the release set has not changed (generated_at is preserved), and
// advances generated_at when it has.
func TestRun_IdempotentRewrite(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "releases.json")
	input := filepath.Join("testdata", "gh-api-releases.json")

	pinNow(t, time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	if err := run([]string{"--input", input, "--out", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read first output: %v", err)
	}

	// Same input an hour later: byte-identical (no CI churn, no empty commit).
	pinNow(t, time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC))
	if err := run([]string{"--input", input, "--out", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read second output: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("unchanged input produced different output:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	// Changed input (a new release appears): generated_at advances.
	changed := filepath.Join(dir, "changed.json")
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	// Splice the new release into the fixture's single JSON array. Trim
	// trailing whitespace BEFORE the closing ']': the fixture is not pinned
	// to LF in .gitattributes, so a Windows checkout (core.autocrlf=true)
	// ends it with "]\r\n" and an LF-only TrimSuffix would not strip the
	// ']' — the appended ",{...}]" would then follow a complete array,
	// making the stream "[...]\r\n,{...}]" whose second top-level value
	// starts with ',' ("invalid character ',' looking for beginning of
	// value").
	raw = bytes.TrimSpace(raw)
	raw = bytes.TrimSuffix(raw, []byte("]"))
	raw = append(raw, `,{"tag_name":"v1.18.0","name":"v1.18.0","draft":false,"prerelease":true,"published_at":"2026-10-15T12:30:00Z","body":"new"}]`...)
	raw = append(raw, '\n')
	if err := os.WriteFile(changed, raw, 0o644); err != nil {
		t.Fatalf("write changed fixture: %v", err)
	}
	if err := run([]string{"--input", changed, "--out", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("third run: %v", err)
	}
	third, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read third output: %v", err)
	}
	if bytes.Equal(second, third) {
		t.Error("changed input produced identical output; generated_at should have advanced")
	}
	var doc document
	if err := json.Unmarshal(third, &doc); err != nil {
		t.Fatalf("parse third output: %v", err)
	}
	if doc.GeneratedAt != "2026-10-15T13:00:00Z" {
		t.Errorf("generated_at = %q, want 2026-10-15T13:00:00Z", doc.GeneratedAt)
	}
	if len(doc.Releases) != 4 || doc.Releases[0].Tag != "v1.18.0" {
		t.Errorf("expected v1.18.0 first of 4 releases, got %+v", doc.Releases)
	}
}

// TestRun_CorruptExistingFileIsSelfHealing: an unparsable existing --out file
// must not fail the run; it is rewritten from scratch.
func TestRun_CorruptExistingFileIsSelfHealing(t *testing.T) {
	pinNow(t, time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	dir := t.TempDir()
	out := filepath.Join(dir, "releases.json")
	if err := os.WriteFile(out, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	if err := run([]string{"--input", filepath.Join("testdata", "gh-api-releases.json"), "--out", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run over corrupt file: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestRun_Errors(t *testing.T) {
	t.Run("gh failure exits non-zero", func(t *testing.T) {
		err := run([]string{"--gh", filepath.Join(t.TempDir(), "no-such-gh")}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("expected error when gh cannot be executed")
		}
		if !strings.Contains(err.Error(), "gh api") {
			t.Errorf("error should name the gh call, got: %v", err)
		}
	})

	t.Run("missing --input file exits non-zero", func(t *testing.T) {
		if err := run([]string{"--input", filepath.Join(t.TempDir(), "nope.json"), "--dry-run"}, &bytes.Buffer{}); err == nil {
			t.Fatal("expected error for missing --input file")
		}
	})
}
