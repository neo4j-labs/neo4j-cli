// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// p converts forward-slash test paths to the OS-native separator so test
// expectations match what filepath.Join produces on Windows. On Windows a
// leading `/` is not an absolute path (filepath.IsAbs returns false), so
// inputs that look like Unix-absolute paths (`/cfg`, `/data/a`, ...) get a
// `C:` drive-letter prefix to stay absolute under Windows path semantics.
func p(s string) string {
	if runtime.GOOS == "windows" && strings.HasPrefix(s, "/") {
		return "C:" + filepath.FromSlash(s)
	}
	return filepath.FromSlash(s)
}

func withUserConfigDir(t *testing.T, dir string) {
	t.Helper()
	restore := SetUserConfigDirFnForTest(func() (string, error) { return dir, nil })
	t.Cleanup(restore)
}

// writeEnvJSON writes a relate env JSON into the directory EnvConfigDir()
// resolves to under the test seams. configDir is unused now that EnvConfigDir
// branches per OS; callers should still pass the same value they handed to
// withUserConfigDir for clarity at the call site.
func writeEnvJSON(t *testing.T, fs afero.Fs, configDir, filename, body string) string {
	t.Helper()
	_ = configDir // retained for caller clarity; the actual dir comes from EnvConfigDir()
	dir, err := EnvConfigDir()
	if err != nil {
		t.Fatalf("EnvConfigDir: %v", err)
	}
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	full := filepath.Join(dir, filename)
	if err := afero.WriteFile(fs, full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q: %v", full, err)
	}
	return full
}

func TestEnvConfigDir_PerOS(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"darwin", p("/cfg/com.Neo4j.Relate/Config/environments")},
		{"windows", p("/cfg/Neo4j/Relate/Config/environments")},
		{"linux", p("/cfg/neo4j-relate/environments")},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			withUserConfigDir(t, p("/cfg"))
			restoreGOOS := SetGOOSFnForTest(func() string { return tc.goos })
			t.Cleanup(restoreGOOS)

			got, err := EnvConfigDir()
			if err != nil {
				t.Fatalf("EnvConfigDir: %v", err)
			}
			if got != tc.want {
				t.Fatalf("goos=%s: EnvConfigDir = %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}

// TestEnvConfigDir_NativeOS exercises the unmocked runtime.GOOS branch so each
// CI matrix entry (ubuntu/macos/windows) asserts its own native path shape.
// Catches drift in os.UserConfigDir shape that the mocked seam tests cannot
// see.
func TestEnvConfigDir_NativeOS(t *testing.T) {
	got, err := EnvConfigDir()
	if err != nil {
		t.Fatalf("EnvConfigDir: %v", err)
	}
	var wantSuffix string
	switch runtime.GOOS {
	case "darwin":
		wantSuffix = filepath.FromSlash("com.Neo4j.Relate/Config/environments")
	case "windows":
		wantSuffix = filepath.FromSlash("Neo4j/Relate/Config/environments")
	default:
		wantSuffix = filepath.FromSlash("neo4j-relate/environments")
	}
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("goos=%s: EnvConfigDir = %q, want suffix %q", runtime.GOOS, got, wantSuffix)
	}
}

func TestLoadEnvs_MissingDir(t *testing.T) {
	withUserConfigDir(t, p("/cfg"))
	fs := afero.NewMemMapFs()

	envs, err := LoadEnvs(fs)
	if err != nil {
		t.Fatalf("LoadEnvs returned error on missing dir: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected 0 envs on missing dir, got %d", len(envs))
	}
}

func TestLoadEnvs_HappyPath(t *testing.T) {
	withUserConfigDir(t, p("/cfg"))
	fs := afero.NewMemMapFs()
	writeEnvJSON(t, fs, p("/cfg"), "a.json", `{
		"name": "Default",
		"id": "env-a",
		"active": true,
		"type": "LOCAL",
		"relateDataPath": "/data/a",
		"httpOrigin": "http://localhost:44222"
	}`)
	writeEnvJSON(t, fs, p("/cfg"), "b.json", `{
		"name": "Other",
		"id": "env-b",
		"active": false,
		"type": "LOCAL"
	}`)

	envs, err := LoadEnvs(fs)
	if err != nil {
		t.Fatalf("LoadEnvs: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(envs))
	}

	// File order is directory-listing order — both names start with letters
	// and afero ReadDir sorts lexically, so a.json is first.
	if envs[0].Name != "Default" || envs[0].ID != "env-a" {
		t.Fatalf("envs[0] = %+v", envs[0])
	}
	if !envs[0].Active {
		t.Fatalf("envs[0].Active = false, want true")
	}
	if envs[0].RelateDataPath != "/data/a" {
		t.Fatalf("envs[0].RelateDataPath = %q", envs[0].RelateDataPath)
	}
	if envs[1].Name != "Other" || envs[1].Active {
		t.Fatalf("envs[1] = %+v", envs[1])
	}
	// b.json omits httpOrigin + relateDataPath; parser should tolerate.
	if envs[1].RelateDataPath != "" || envs[1].HTTPOrigin != "" {
		t.Fatalf("envs[1] should have empty optionals, got %+v", envs[1])
	}

	// Path field is populated for diagnostics.
	if !filepath.IsAbs(envs[0].Path) {
		t.Fatalf("envs[0].Path is not absolute: %q", envs[0].Path)
	}
}

func TestLoadEnvs_IgnoresCorruptAndNonJSON(t *testing.T) {
	withUserConfigDir(t, p("/cfg"))
	fs := afero.NewMemMapFs()
	writeEnvJSON(t, fs, p("/cfg"), "good.json", `{"name":"OK","id":"x","active":true,"type":"LOCAL"}`)
	writeEnvJSON(t, fs, p("/cfg"), "bad.json", `not-json`)
	writeEnvJSON(t, fs, p("/cfg"), "notes.txt", `ignored`)

	envs, err := LoadEnvs(fs)
	if err != nil {
		t.Fatalf("LoadEnvs: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 env (corrupt + non-json filtered), got %d", len(envs))
	}
	if envs[0].Name != "OK" {
		t.Fatalf("envs[0].Name = %q", envs[0].Name)
	}
}

func TestActiveEnv(t *testing.T) {
	envs := []EnvJSON{
		{Name: "A", Active: false},
		{Name: "B", Active: true},
		{Name: "C", Active: false},
	}

	t.Run("returns active when override empty", func(t *testing.T) {
		got := ActiveEnv(envs, "")
		if got == nil || got.Name != "B" {
			t.Fatalf("got=%+v want B", got)
		}
	})

	t.Run("override picks named env even when not active", func(t *testing.T) {
		got := ActiveEnv(envs, "C")
		if got == nil || got.Name != "C" {
			t.Fatalf("got=%+v want C", got)
		}
	})

	t.Run("override miss returns nil (not an error)", func(t *testing.T) {
		got := ActiveEnv(envs, "Nope")
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})

	t.Run("no active and no override returns nil", func(t *testing.T) {
		got := ActiveEnv([]EnvJSON{{Name: "A"}, {Name: "B"}}, "")
		if got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
}
