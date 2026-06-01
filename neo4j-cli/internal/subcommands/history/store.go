// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package history records and reads a local, best-effort log of executed
// neo4j-cli commands. Each command is appended as one JSON line to a
// history.jsonl file in the OS config dir (alongside config.json). Recording
// never crashes or aborts the underlying command — every write error is
// swallowed.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clievents"
	"github.com/spf13/afero"
	"golang.org/x/term"
)

const fileName = "history.jsonl"

// Entry is the on-disk shape of a single history record.
type Entry struct {
	Time       time.Time `json:"time"`
	Command    string    `json:"command"`
	Invoker    string    `json:"invoker"`
	Version    string    `json:"version"`
	Workspace  string    `json:"workspace,omitempty"`
	Credential string    `json:"credential,omitempty"`
}

// path returns the absolute path to the history file. clicfg.ConfigPrefix is
// set per-OS in clicfg/{darwin,linux,windows}.go.
func path() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", fileName)
}

// stdinIsTerminal is the package-level test seam for terminal detection on
// stdin. Production calls term.IsTerminal; tests override the var.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// invoker resolves "human" when stdin is a TTY and "agent" otherwise.
func invoker() string {
	if stdinIsTerminal() {
		return "human"
	}
	return "agent"
}

// Record appends a redacted history entry for the current invocation. It is
// best-effort: it short-circuits when history is disabled or the limit is 0,
// and every error (read, marshal, write) is swallowed so a bad history write
// can never crash or abort the underlying command.
func Record(cfg *clicfg.Config) {
	if cfg == nil || cfg.Global == nil {
		return
	}
	if !cfg.Global.HistoryEnabled() {
		return
	}
	limit := cfg.Global.HistoryLimit()
	if limit <= 0 {
		return
	}

	fs := cfg.Aura.Fs()
	if fs == nil {
		return
	}

	entry := Entry{
		Time:      time.Now().UTC(),
		Command:   strings.TrimSpace("neo4j-cli " + clievents.RedactArgs(os.Args[1:])),
		Invoker:   invoker(),
		Version:   cfg.Version,
		Workspace: cfg.Aura.DefaultWorkspace(),
	}
	if cfg.Credentials != nil && cfg.Credentials.Aura != nil {
		entry.Credential = cfg.Credentials.Aura.DefaultCredential
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return
	}

	lines := readLines(fs)
	lines = append(lines, string(line))
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	data := []byte(strings.Join(lines, "\n") + "\n")
	_ = fileutils.WriteFileErr(fs, path(), data)
}

// Load reads and parses every history entry, skipping unparseable lines.
// Returns an empty slice when the file is absent.
func Load(cfg *clicfg.Config) ([]Entry, error) {
	if cfg == nil {
		return nil, nil
	}
	fs := cfg.Aura.Fs()
	if fs == nil {
		return nil, nil
	}

	raw, err := afero.ReadFile(fs, path())
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	entries := make([]Entry, 0)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Clear truncates the history file. A missing file is not an error.
func Clear(cfg *clicfg.Config) error {
	if cfg == nil {
		return nil
	}
	fs := cfg.Aura.Fs()
	if fs == nil {
		return nil
	}
	return fileutils.WriteFileErr(fs, path(), []byte{})
}

// readLines returns the non-empty trimmed lines of the history file, or an
// empty slice when the file is absent or unreadable.
func readLines(fs afero.Fs) []string {
	raw, err := afero.ReadFile(fs, path())
	if err != nil {
		return []string{}
	}
	out := make([]string, 0)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
