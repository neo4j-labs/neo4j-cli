// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Command update_fixture is the CI-only fixture server for the tier-2 e2e
// test of `neo4j-cli update`. It impersonates the GitHub releases API and
// the GoReleaser archive-download URLs against a single localhost port,
// driven by the e2e_seams build of neo4j-cli (see
// neo4j-cli/internal/subcommands/update/seams_e2e.go).
//
// Layout: pure stdlib + golang.org/x/mod/semver, no new module deps. The
// fake binary inside each archive is compiled on the fly (via `go build`)
// from test/e2e/update_fixture/fakebin so the swapped binary actually runs
// on the runner's OS/arch.
//
// Endpoints:
//
//   - GET /repos/neo4j-labs/neo4j-cli/releases?per_page=30
//     Returns a synthetic release list with one stable + one prerelease tag.
//
//   - GET /neo4j-labs/neo4j-cli/releases/download/<TAG>/neo4j-cli_<VER>_<OS>_<ARCH>.<ext>
//     Returns a tar.gz (linux/darwin) or zip (windows) containing the
//     fakebin compiled with -ldflags='-X main.tag=<TAG>'. <ext> is .tar.gz
//     on linux/darwin and .zip on windows.
//
//   - GET /neo4j-labs/neo4j-cli/releases/download/<TAG>/neo4j-cli_<VER>_checksums.txt
//     Returns sha256 sum + filename per line for the archive served above.
//
// Lifecycle:
//
//   - Listens on 127.0.0.1:0, prints `listening on http://127.0.0.1:<port>`
//     to stdout (single line, FIRST line of stdout) so the harness can
//     scrape and capture the port.
//   - Shuts down cleanly on SIGINT / SIGTERM.
//   - Exits non-zero on any setup error (port bind, fakebin build fail).
//
// Flags:
//
//	--stable     stable release tag (default "v9.9.0")
//	--prerelease prerelease tag (default "v9.9.1-alpha.1")
//
// The harness does not pass these — the defaults are fine.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// release mirrors the slim subset of the GitHub release JSON neo4j-cli's
// release.go decodes. Extra fields are ignored on the client side.
type release struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func main() {
	stableTag := flag.String("stable", "v9.9.0", "synthetic stable release tag")
	prereleaseTag := flag.String("prerelease", "v9.9.1-alpha.1", "synthetic prerelease tag")
	flag.Parse()

	tags := []string{*stableTag, *prereleaseTag}

	// Pre-build all archives + checksums once at startup. Avoids `go build`
	// running on every request and makes a server slow in a way that could
	// race against neo4j-cli's HTTP timeout.
	archives, err := buildArchives(tags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture: build archives: %v\n", err)
		os.Exit(1)
	}

	releases := []release{
		{TagName: *prereleaseTag, Draft: false, Prerelease: true},
		{TagName: *stableTag, Draft: false, Prerelease: false},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/neo4j-labs/neo4j-cli/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(releases)
	})
	mux.HandleFunc("/neo4j-labs/neo4j-cli/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		// URL shape: /neo4j-labs/neo4j-cli/releases/download/<TAG>/<FILENAME>
		base := filepath.Base(r.URL.Path)
		// Walk archives in deterministic order.
		for _, a := range archives {
			if base == a.archiveName {
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(a.archiveBytes)
				return
			}
			if base == a.checksumName {
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write(a.checksumBody)
				return
			}
		}
		http.NotFound(w, r)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture: bind: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "fixture: serve: %v\n", err)
		os.Exit(1)
	}
}

// archive bundles the built archive and matching checksums.txt for one tag.
type archive struct {
	tag          string
	archiveName  string // e.g. neo4j-cli_9.9.1-alpha.1_Linux_x86_64.tar.gz
	checksumName string // e.g. neo4j-cli_9.9.1-alpha.1_checksums.txt
	archiveBytes []byte
	checksumBody []byte
}

// buildArchives compiles the fakebin once per tag (with -ldflags pinning the
// tag string into the binary) and packages it into the archive shape
// neo4j-cli expects.
func buildArchives(tags []string) ([]archive, error) {
	fakebinSrc, err := locateFakebinDir()
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "fixture-bins-")
	if err != nil {
		return nil, fmt.Errorf("create tmp: %w", err)
	}
	// Caller is the long-lived fixture process; the tmpDir is fine to keep
	// for the process lifetime — it'll be reaped on exit by the OS or the
	// runner cleanup. Avoid removing here so the build cache is durable
	// across handler invocations.

	out := make([]archive, 0, len(tags))
	for _, tag := range tags {
		verNoV := strings.TrimPrefix(tag, "v")
		osTitle, archUname, ext, binName := platformParts()

		archiveName := fmt.Sprintf("neo4j-cli_%s_%s_%s%s", verNoV, osTitle, archUname, ext)
		checksumName := fmt.Sprintf("neo4j-cli_%s_checksums.txt", verNoV)

		// Compile fakebin with the tag baked in.
		binPath := filepath.Join(tmpDir, fmt.Sprintf("fakebin-%s%s", verNoV, exeExt()))
		cmd := exec.Command("go", "build",
			"-ldflags", fmt.Sprintf("-X main.tag=%s", tag),
			"-o", binPath,
			fakebinSrc,
		)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("build fakebin %s: %w", tag, err)
		}
		binBody, err := os.ReadFile(binPath)
		if err != nil {
			return nil, fmt.Errorf("read fakebin %s: %w", tag, err)
		}

		// Package into archive (tar.gz on linux/darwin, zip on windows).
		archiveBytes, err := packageBinary(binBody, binName, ext)
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", tag, err)
		}

		// Build matching checksums.txt: "<sha256hex>  <archive-filename>".
		sum := sha256.Sum256(archiveBytes)
		checksumBody := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName))

		out = append(out, archive{
			tag:          tag,
			archiveName:  archiveName,
			checksumName: checksumName,
			archiveBytes: archiveBytes,
			checksumBody: checksumBody,
		})
	}
	return out, nil
}

// platformParts mirrors the GoReleaser filename layout. Keep in sync with
// neo4j-cli's release.go BuildAssetURLs.
func platformParts() (osTitle, archUname, ext, binName string) {
	switch runtime.GOOS {
	case "linux":
		osTitle = "Linux"
		ext = ".tar.gz"
		binName = "neo4j-cli"
	case "darwin":
		osTitle = "Darwin"
		ext = ".tar.gz"
		binName = "neo4j-cli"
	case "windows":
		osTitle = "Windows"
		ext = ".zip"
		binName = "neo4j-cli.exe"
	default:
		osTitle = "UNSUPPORTED"
		ext = ".tar.gz"
		binName = "neo4j-cli"
	}
	switch runtime.GOARCH {
	case "amd64":
		archUname = "x86_64"
	case "arm64":
		archUname = "arm64"
	case "386":
		archUname = "i386"
	default:
		archUname = "UNSUPPORTED"
	}
	return
}

// exeExt returns ".exe" on windows, "" elsewhere — matches the convention
// `go build` uses for the output filename when a target is windows.
func exeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// packageBinary wraps body in a tar.gz or zip archive, naming the entry
// `name`. The archive contains exactly one regular file at mode 0755 — the
// shape neo4j-cli's swap.go expects (it'll match by basename, ignore mode
// in zip aside from the regular-file gate).
func packageBinary(body []byte, name, ext string) ([]byte, error) {
	switch ext {
	case ".tar.gz":
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, err
		}
		if err := tw.Close(); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case ".zip":
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.SetMode(0o755)
		w, err := zw.CreateHeader(fh)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(body); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported archive ext %q", ext)
	}
}

// locateFakebinDir returns the absolute path to the fakebin Go package so
// `go build` can compile it. Looks for fakebin/ next to the running source
// (for `go run ./test/e2e/update_fixture`) and falls back to a sibling
// lookup if the working dir is something unexpected.
func locateFakebinDir() (string, error) {
	// `go run ./test/e2e/update_fixture` runs with the repo root as
	// the working directory, so the fakebin source lives under
	// `./test/e2e/update_fixture/fakebin`.
	candidates := []string{
		filepath.Join("test", "e2e", "update_fixture", "fakebin"),
		filepath.Join("..", "fakebin"),
		"fakebin",
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "fakebin.go")); err == nil {
			return abs, nil
		}
	}
	wd, _ := os.Getwd()
	return "", fmt.Errorf("locate fakebin source: searched %v from cwd %s", candidates, wd)
}
