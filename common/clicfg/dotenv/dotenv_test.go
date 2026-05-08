// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dotenv

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// withHome swaps the package-level homeDirFn for the duration of t. The
// override means we don't have to mutate the real $HOME env or rely on
// os.UserHomeDir, which keeps tests hermetic on every platform.
func withHome(t *testing.T, home string) {
	t.Helper()
	prev := homeDirFn
	homeDirFn = func() (string, error) {
		if home == "" {
			return "", nil
		}
		return home, nil
	}
	t.Cleanup(func() { homeDirFn = prev })
}

// writeFile is a tiny convenience over afero.WriteFile for tests; the mode
// is irrelevant for MemMapFs but kept realistic so tests read close to
// production usage.
func writeFile(t *testing.T, fs afero.Fs, path, body string) {
	t.Helper()
	if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdirall %q: %v", filepath.Dir(path), err)
	}
	if err := afero.WriteFile(fs, path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(fs afero.Fs)
		home           string
		startDir       string
		wantPath       string
		wantFound      bool
		wantAboveCWD   bool
		wantNoEnvAtAll bool // true means "no .env was loaded"
	}{
		{
			name: ".env in cwd is found, walkedAboveCWD false",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/home/u/proj/.env", "X=1")
			},
			home:         "/home/u",
			startDir:     "/home/u/proj",
			wantPath:     "/home/u/proj/.env",
			wantFound:    true,
			wantAboveCWD: false,
		},
		{
			name: "ancestor .env is found, walkedAboveCWD true",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/home/u/proj/.env", "X=1")
			},
			home:         "/home/u",
			startDir:     "/home/u/proj/sub/deeper",
			wantPath:     "/home/u/proj/.env",
			wantFound:    true,
			wantAboveCWD: true,
		},
		{
			name: "stops at .git ancestor before reaching higher .env",
			setup: func(fs afero.Fs) {
				// poison .env above the .git boundary
				writeFile(t, fs, "/home/u/.env", "POISON=1")
				// .git marker at /home/u/proj — repo root
				writeFile(t, fs, "/home/u/proj/.git", "")
			},
			home:           "/home/u",
			startDir:       "/home/u/proj/sub",
			wantNoEnvAtAll: true,
		},
		{
			name: ".env at .git repo root IS picked up (boundary inclusive)",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/home/u/proj/.git", "")
				writeFile(t, fs, "/home/u/proj/.env", "X=1")
			},
			home:         "/home/u",
			startDir:     "/home/u/proj/sub",
			wantPath:     "/home/u/proj/.env",
			wantFound:    true,
			wantAboveCWD: true,
		},
		{
			name: "stops at $HOME boundary, does not load /.env above home",
			setup: func(fs afero.Fs) {
				// poison at /
				writeFile(t, fs, "/.env", "POISON=1")
			},
			home:           "/home/u",
			startDir:       "/home/u/proj/sub",
			wantNoEnvAtAll: true,
		},
		{
			name: ".env at $HOME IS picked up (boundary inclusive)",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/home/u/.env", "X=1")
			},
			home:         "/home/u",
			startDir:     "/home/u/proj/sub",
			wantPath:     "/home/u/.env",
			wantFound:    true,
			wantAboveCWD: true,
		},
		{
			name: "no .env anywhere returns not found",
			setup: func(fs afero.Fs) {
				// nothing
			},
			home:           "/home/u",
			startDir:       "/home/u/proj/sub",
			wantNoEnvAtAll: true,
		},
		{
			name: "deepest .env wins over ancestor .env",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/home/u/proj/.env", "X=ancestor")
				writeFile(t, fs, "/home/u/proj/sub/.env", "X=child")
			},
			home:         "/home/u",
			startDir:     "/home/u/proj/sub",
			wantPath:     "/home/u/proj/sub/.env",
			wantFound:    true,
			wantAboveCWD: false,
		},
		{
			name: "empty home is tolerated; .git boundary still applies",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/repo/.git", "")
				writeFile(t, fs, "/repo/.env", "X=1")
			},
			home:         "",
			startDir:     "/repo/sub",
			wantPath:     "/repo/.env",
			wantFound:    true,
			wantAboveCWD: true,
		},
		{
			name:           "empty startDir returns not found",
			setup:          func(fs afero.Fs) {},
			home:           "/home/u",
			startDir:       "",
			wantNoEnvAtAll: true,
		},
		{
			name: "no .git and no $HOME stops at filesystem root",
			setup: func(fs afero.Fs) {
				// nothing
			},
			home:           "",
			startDir:       "/some/where",
			wantNoEnvAtAll: true,
		},
		{
			name: ".git as a file (worktree/submodule) still acts as boundary",
			setup: func(fs afero.Fs) {
				writeFile(t, fs, "/home/u/.env", "POISON=1")
				// .git as plain file, not a directory
				writeFile(t, fs, "/home/u/proj/.git", "gitdir: ../.git/worktrees/foo")
			},
			home:           "/home/u",
			startDir:       "/home/u/proj/sub",
			wantNoEnvAtAll: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setup(fs)
			withHome(t, tc.home)

			path, found, above := Find(fs, tc.startDir)

			if tc.wantNoEnvAtAll {
				if found || path != "" || above {
					t.Fatalf("expected no .env, got path=%q found=%v aboveCWD=%v", path, found, above)
				}
				return
			}
			if !found {
				t.Fatalf("expected found=true, got found=false (path=%q)", path)
			}
			if path != tc.wantPath {
				t.Fatalf("expected path=%q, got %q", tc.wantPath, path)
			}
			if above != tc.wantAboveCWD {
				t.Fatalf("expected walkedAboveCWD=%v, got %v", tc.wantAboveCWD, above)
			}
		})
	}
}
