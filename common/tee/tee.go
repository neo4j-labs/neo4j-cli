// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package tee persists a failing command's emitted output to a best-effort,
// redacted log file under the OS config dir, rotating to keep at most a
// configured number of files per command type. It is cobra-free so main.go can
// import it; every error is swallowed so capturing output can never crash or
// alter the behaviour of the underlying command.
package tee

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clievents"
	"github.com/spf13/afero"
)

// maxCaptureBytes bounds the in-memory capture buffer so an arbitrarily large
// output stream costs only this much memory; beyond it the head is kept and a
// truncation footer is appended.
const maxCaptureBytes = 5 << 20

// timestampFormat is filesystem-safe (no colons in the time portion) and
// lexicographically sortable, so a name sort is a chronological sort.
const timestampFormat = "2006-01-02T15-04-05Z"

// LimitedBuffer is an io.Writer that accumulates up to maxCaptureBytes of the
// written stream (the head), then silently discards the rest while recording
// the total seen. Write never errors so it is safe behind an io.MultiWriter
// fronting the real stdout/stderr.
type LimitedBuffer struct {
	head    []byte
	total   int
	dropped bool
}

// Write appends to the head buffer until the cap is reached, then drops the
// overflow. It always reports the full length as written so the MultiWriter
// peer (the real stream) is never short-changed.
func (b *LimitedBuffer) Write(p []byte) (int, error) {
	b.total += len(p)
	if room := maxCaptureBytes - len(b.head); room > 0 {
		if len(p) > room {
			b.head = append(b.head, p[:room]...)
			b.dropped = true
		} else {
			b.head = append(b.head, p...)
		}
	} else if len(p) > 0 {
		b.dropped = true
	}
	return len(p), nil
}

// Bytes returns the captured head, with a truncation footer appended when the
// stream exceeded the cap.
func (b *LimitedBuffer) Bytes() []byte {
	if !b.dropped {
		return b.head
	}
	footer := fmt.Sprintf("\n[output truncated: exceeded %d bytes]\n", b.total)
	return append(append([]byte{}, b.head...), footer...)
}

// Dir returns the directory tee files live in. clicfg.ConfigPrefix is set
// per-OS in clicfg/{darwin,linux,windows}.go.
func Dir() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "tee")
}

// Save writes a redacted log of content for commandSlug and returns its
// absolute path. It is best-effort: it returns ("", nil) when tee is disabled,
// the limit is non-positive, the filesystem or config is unavailable, or there
// is nothing to write. A failed write returns ("", err); a failed rotation does
// not prevent returning the written path.
func Save(cfg *clicfg.Config, commandSlug string, content []byte) (string, error) {
	if cfg == nil || cfg.Global == nil {
		return "", nil
	}
	if !cfg.Global.TeeEnabled() {
		return "", nil
	}
	limit := cfg.Global.TeeLimit()
	if limit <= 0 {
		return "", nil
	}
	fs := cfg.Aura.Fs()
	if fs == nil {
		return "", nil
	}
	if len(content) == 0 {
		return "", nil
	}

	dir := Dir()
	name := time.Now().UTC().Format(timestampFormat) + "_" + commandSlug + ".log"
	path := filepath.Join(dir, name)

	// Rotate to tee-limit-1 before the write so exactly tee-limit files remain
	// once the new file lands.
	rotate(fs, dir, commandSlug, limit-1)

	if err := fileutils.WriteFileErr(fs, path, []byte(clievents.RedactText(string(content)))); err != nil {
		return "", err
	}
	return path, nil
}

// rotate deletes the oldest *_<slug>.log files in dir until at most keep
// remain. Listing and deletion errors are swallowed — rotation is best-effort.
func rotate(fs afero.Fs, dir, slug string, keep int) {
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return
	}

	suffix := "_" + slug + ".log"
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n := e.Name(); len(n) > len(suffix) && strings.HasSuffix(n, suffix) {
			names = append(names, n)
		}
	}
	if len(names) <= keep {
		return
	}

	sort.Strings(names)
	for _, n := range names[:len(names)-keep] {
		_ = fs.Remove(filepath.Join(dir, n))
	}
}
