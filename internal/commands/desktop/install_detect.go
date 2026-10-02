// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktop — install_detect.go owns already-installed detection.
// Detection runs BEFORE any network call so an idempotent re-run on an
// installed system prints the breadcrumb and exits without touching network.
//
// Per-OS:
//   - macOS: stat `/Applications/Neo4j Desktop 2.app` and
//     `~/Applications/Neo4j Desktop 2.app`. Reads Info.plist's
//     `CFBundleShortVersionString`.
//   - Linux: glob `~/Applications/neo4j-desktop-*.AppImage`.
//   - Windows: stat `%LOCALAPPDATA%\Programs\neo4j-desktop\`.
package desktop

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// installedHit is the result of a successful detection. Version is "" when
// detection didn't recover one.
type installedHit struct {
	Path    string
	Version string
}

// InstalledHit is the exported alias used by tests.
type InstalledHit = installedHit

// DetectInstalledForTest is the exported wrapper around detectInstalled.
func DetectInstalledForTest(goos string) (InstalledHit, bool) {
	return detectInstalled(goos)
}

var (
	detectHomeDirFn = os.UserHomeDir

	detectLocalAppDataFn = func() string { return os.Getenv("LOCALAPPDATA") }

	detectStatFn = os.Stat

	// detectReadFileFn shadows os.ReadFile for the macOS Info.plist parse.
	// Linux and Windows treat filesystem layout as authoritative and do not
	// use this seam.
	detectReadFileFn = os.ReadFile

	detectGlobFn = filepath.Glob
)

// SetDetectHomeDirFnForTest overrides the home-dir resolver.
func SetDetectHomeDirFnForTest(fn func() (string, error)) func() {
	prev := detectHomeDirFn
	detectHomeDirFn = fn
	return func() { detectHomeDirFn = prev }
}

// SetDetectLocalAppDataFnForTest overrides the %LOCALAPPDATA% resolver.
func SetDetectLocalAppDataFnForTest(fn func() string) func() {
	prev := detectLocalAppDataFn
	detectLocalAppDataFn = fn
	return func() { detectLocalAppDataFn = prev }
}

// SetDetectStatFnForTest overrides os.Stat for the detection path.
func SetDetectStatFnForTest(fn func(string) (os.FileInfo, error)) func() {
	prev := detectStatFn
	detectStatFn = fn
	return func() { detectStatFn = prev }
}

// SetDetectReadFileFnForTest overrides os.ReadFile for the detection path.
func SetDetectReadFileFnForTest(fn func(string) ([]byte, error)) func() {
	prev := detectReadFileFn
	detectReadFileFn = fn
	return func() { detectReadFileFn = prev }
}

// SetDetectGlobFnForTest overrides filepath.Glob for the detection path.
func SetDetectGlobFnForTest(fn func(string) ([]string, error)) func() {
	prev := detectGlobFn
	detectGlobFn = fn
	return func() { detectGlobFn = prev }
}

// detectInstalled returns the first per-OS hit. OS-agnostic at file level
// so a Linux host can exercise the macOS / Windows branches via test seams.
func detectInstalled(goos string) (installedHit, bool) {
	switch goos {
	case "darwin":
		return detectMacOS()
	case "linux":
		return detectLinux()
	case "windows":
		return detectWindows()
	default:
		return installedHit{}, false
	}
}

// detectMacOS stats `/Applications/Neo4j Desktop 2.app` first, then
// `~/Applications/Neo4j Desktop 2.app`, returning the Info.plist version.
func detectMacOS() (installedHit, bool) {
	candidates := []string{"/Applications/Neo4j Desktop 2.app"}
	if home, err := detectHomeDirFn(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, "Applications", "Neo4j Desktop 2.app"))
	}

	for _, p := range candidates {
		info, err := detectStatFn(p)
		if err != nil || !info.IsDir() {
			continue
		}
		version := readMacOSVersion(filepath.Join(p, "Contents", "Info.plist"))
		return installedHit{Path: p, Version: version}, true
	}
	return installedHit{}, false
}

// macOSVersionPattern avoids pulling a plist parser dependency for one field.
var macOSVersionPattern = regexp.MustCompile(
	`<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)

// readMacOSVersion best-effort reads Info.plist's CFBundleShortVersionString.
// Returns "" on any error so the caller falls back to "version unknown".
func readMacOSVersion(path string) string {
	body, err := detectReadFileFn(path)
	if err != nil {
		return ""
	}
	m := macOSVersionPattern.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

// linuxAppImagePattern extracts the version suffix from the AppImage
// filename, supporting semver (`X.Y.Z`) and `-rcN` suffixes.
var linuxAppImagePattern = regexp.MustCompile(
	`^neo4j-desktop-(\d+(?:\.\d+){0,3}(?:-[A-Za-z0-9.]+)?)\.AppImage$`)

// detectLinux globs `~/Applications/neo4j-desktop-*.AppImage`. First match
// wins (alphabetical via filepath.Glob); a non-matching filename still
// returns a hit with version="".
func detectLinux() (installedHit, bool) {
	home, err := detectHomeDirFn()
	if err != nil || home == "" {
		return installedHit{}, false
	}
	pattern := filepath.Join(home, "Applications", "neo4j-desktop-*.AppImage")
	matches, err := detectGlobFn(pattern)
	if err != nil || len(matches) == 0 {
		return installedHit{}, false
	}
	first := matches[0]
	version := ""
	if m := linuxAppImagePattern.FindStringSubmatch(filepath.Base(first)); len(m) == 2 {
		version = m[1]
	}
	return installedHit{Path: first, Version: version}, true
}

// detectWindows stats `%LOCALAPPDATA%\Programs\neo4j-desktop\`. Registry
// `DisplayVersion` lookup is out of scope; the breadcrumb degrades to
// "version unknown" rather than failing the leaf.
func detectWindows() (installedHit, bool) {
	localAppData := detectLocalAppDataFn()
	if localAppData == "" {
		return installedHit{}, false
	}
	path := filepath.Join(localAppData, "Programs", "neo4j-desktop")
	info, err := detectStatFn(path)
	if err != nil || !info.IsDir() {
		return installedHit{}, false
	}
	return installedHit{Path: path, Version: readWindowsVersion(path)}, true
}

// readWindowsVersion best-effort reads a version from a hint file installed
// by the NSIS installer if present. Returns "" when no version recovers.
func readWindowsVersion(installDir string) string {
	candidates := []string{
		filepath.Join(installDir, "version"),
		filepath.Join(installDir, "VERSION"),
		filepath.Join(installDir, "resources", "app.asar.unpacked", "package.json"),
	}
	for _, c := range candidates {
		body, err := detectReadFileFn(c)
		if err != nil {
			continue
		}
		if v := extractWindowsVersionLine(body); v != "" {
			return v
		}
	}
	return ""
}

// windowsPackageJSONPattern avoids pulling encoding/json for one field.
var windowsPackageJSONPattern = regexp.MustCompile(
	`"version"\s*:\s*"([^"]+)"`)

// extractWindowsVersionLine tries `package.json` shape first, falling back
// to the first non-blank line (plain `version` / `VERSION` drop files).
func extractWindowsVersionLine(body []byte) string {
	if m := windowsPackageJSONPattern.FindSubmatch(body); len(m) == 2 {
		return strings.TrimSpace(string(m[1]))
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
