// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command releases-json regenerates the static releases.json served at
// https://neo4j.sh/releases.json (gh-pages branch root) from the GitHub
// Releases API. It is the producer half of CLI-244: the CLI's `update`
// command discovers releases from that file instead of the GitHub API.
//
// CI invocation (.github/workflows/update-releases-json.yml, with the
// gh-pages branch checked out into gh-pages/):
//
//	GH_TOKEN=${{ secrets.GITHUB_TOKEN }} go run ./tools/releases-json --out gh-pages/releases.json
//
// Local invocation (bootstrap or manual refresh; needs `gh auth login`):
//
//	go run ./tools/releases-json --out /path/to/gh-pages-checkout/releases.json
//
// Hermetic invocation (no network, no gh auth — reads a canned
// `gh api` response and prints the document instead of writing):
//
//	go run ./tools/releases-json --input testdata/gh-api-releases.json --dry-run
//
// Behaviour contract (shared with the CLI consumer):
//
//   - Input: `gh api repos/<repo>/releases --paginate`. With --paginate gh
//     prints each page's JSON array back to back (`[...][...]`), so decoding
//     loops over successive arrays until EOF.
//   - Drafts (draft == true) are EXCLUDED; the CI GITHUB_TOKEN can see them.
//   - Every other published release is included: prereleases, non-semver
//     tags, no time window (the CLI filters and semver-sorts itself).
//   - body is the API's raw markdown VERBATIM — never rendered or trimmed.
//   - Output is a whole-file rewrite: schema_version 1, generated_at (RFC3339
//     UTC), repository, releases sorted newest-first by published_at.
//   - Idempotent: when the existing --out file already carries the same
//     releases, its generated_at is preserved so repeated runs are
//     byte-identical and CI makes no empty commits.
//
// Exit codes: 0 on success, 1 on any error (gh failure, malformed API data,
// unwritable output) with the reason on stderr.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"time"
)

const (
	// schemaVersion is the releases.json schema version the CLI consumer
	// (CLI-244) reads. Bump only in lockstep with that consumer.
	schemaVersion = 1

	// defaultRepository is the GitHub repo whose releases feed neo4j.sh.
	defaultRepository = "neo4j-labs/neo4j-cli"
)

// apiRelease mirrors the fields of `gh api repos/<owner>/<repo>/releases`
// that releases.json is built from. body is raw markdown and is passed
// through verbatim.
type apiRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

// releaseEntry is one element of the releases.json `releases` array. Key
// order here is the emitted key order.
type releaseEntry struct {
	Tag         string `json:"tag"`
	Name        string `json:"name"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

// document is the top-level releases.json shape. Key order here is the
// emitted key order.
type document struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   string         `json:"generated_at"`
	Repository    string         `json:"repository"`
	Releases      []releaseEntry `json:"releases"`
}

// nowFn is the clock seam; tests pin it to make generated_at deterministic.
var nowFn = time.Now

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "releases-json: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("releases-json", flag.ContinueOnError)
	repo := fs.String("repo", defaultRepository, "GitHub repository (owner/name) to read releases from.")
	out := fs.String("out", "releases.json", "Path to write releases.json to (whole-file rewrite).")
	input := fs.String("input", "", "Read a canned `gh api` response from this file instead of invoking gh (hermetic mode).")
	dryRun := fs.Bool("dry-run", false, "Print the document to stdout instead of writing --out.")
	ghPath := fs.String("gh", "gh", "Path to the gh CLI binary (used unless --input is given).")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var raw []byte
	var err error
	if *input != "" {
		raw, err = os.ReadFile(*input)
		if err != nil {
			return fmt.Errorf("read --input: %w", err)
		}
	} else {
		raw, err = fetchReleases(*ghPath, *repo)
		if err != nil {
			return err
		}
	}

	api, err := parseAPIReleases(raw)
	if err != nil {
		return err
	}

	doc, err := buildDocument(api, *repo, nowFn())
	if err != nil {
		return err
	}

	// Idempotency: same content as the existing file -> keep its
	// generated_at so the rewrite is byte-identical (no empty CI commits).
	if existing, err := readDocument(*out); err == nil && sameContent(existing, doc) {
		doc.GeneratedAt = existing.GeneratedAt
	}

	rendered, err := render(doc)
	if err != nil {
		return err
	}

	if *dryRun {
		_, err := stdout.Write(rendered)
		return err
	}
	return writeFileAtomic(*out, rendered)
}

// fetchReleases shells out to `gh api repos/<repo>/releases --paginate`. Any
// gh failure (non-zero exit, missing binary) is an error: the caller exits
// non-zero rather than publishing a partial or empty feed.
func fetchReleases(ghPath, repo string) ([]byte, error) {
	cmd := exec.Command(ghPath, "api", "repos/"+repo+"/releases", "--paginate")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh api repos/%s/releases: %w: %s", repo, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return stdout.Bytes(), nil
}

// parseAPIReleases decodes `gh api --paginate` output: one JSON array of
// release objects per page, concatenated back to back. A single-page
// response (one array) is the degenerate case of the same loop.
func parseAPIReleases(raw []byte) ([]apiRelease, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("empty API response")
	}
	var releases []apiRelease
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		var page []apiRelease
		if err := dec.Decode(&page); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse API response: %w", err)
		}
		releases = append(releases, page...)
	}
	return releases, nil
}

// buildDocument filters drafts, maps API fields onto the releases.json
// schema (body verbatim) and sorts newest-first by published_at.
func buildDocument(api []apiRelease, repo string, now time.Time) (document, error) {
	type parsed struct {
		entry releaseEntry
		at    time.Time
	}
	kept := make([]parsed, 0, len(api))
	for _, r := range api {
		if r.Draft {
			continue
		}
		if r.TagName == "" {
			return document{}, errors.New("API response contains a release with an empty tag_name")
		}
		at, err := time.Parse(time.RFC3339, r.PublishedAt)
		if err != nil {
			return document{}, fmt.Errorf("release %s: published_at %q is not RFC3339: %w", r.TagName, r.PublishedAt, err)
		}
		kept = append(kept, parsed{
			entry: releaseEntry{
				Tag:         r.TagName,
				Name:        r.Name,
				Prerelease:  r.Prerelease,
				PublishedAt: r.PublishedAt,
				Body:        r.Body,
			},
			at: at,
		})
	}
	// Newest-first for human readability (compare instants, not strings —
	// RFC3339 offsets make lexical order unreliable). The CLI never trusts
	// file order (it semver-sorts itself); ties break on tag for determinism.
	sort.SliceStable(kept, func(i, j int) bool {
		if !kept[i].at.Equal(kept[j].at) {
			return kept[i].at.After(kept[j].at)
		}
		return kept[i].entry.Tag < kept[j].entry.Tag
	})
	releases := make([]releaseEntry, 0, len(kept))
	for _, k := range kept {
		releases = append(releases, k.entry)
	}
	return document{
		SchemaVersion: schemaVersion,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		Repository:    repo,
		Releases:      releases,
	}, nil
}

// render serialises the document with a stable 2-space indent and a trailing
// newline.
func render(doc document) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render document: %w", err)
	}
	return append(raw, '\n'), nil
}

// readDocument reads an existing releases.json, used only for the
// generated_at preservation check. Any error (missing file, corrupt JSON) is
// returned to the caller, which treats it as "no previous content" and
// rewrites from scratch (self-healing).
func readDocument(path string) (document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return document{}, err
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return document{}, err
	}
	return doc, nil
}

// sameContent reports whether two documents carry identical payload
// (everything except generated_at).
func sameContent(a, b document) bool {
	return a.SchemaVersion == b.SchemaVersion &&
		a.Repository == b.Repository &&
		reflect.DeepEqual(a.Releases, b.Releases)
}

// writeFileAtomic writes via a temp file + rename so a crash mid-write never
// leaves a truncated releases.json.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".releases-json-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // best-effort cleanup; rename removes it first on success
	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck // the write error is the one that matters
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
