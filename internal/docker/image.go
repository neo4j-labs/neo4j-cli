// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"regexp"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// versionPattern is the package-level allowlist for `--version` values flowing
// into the docker image tag (REQ-F-002). Compiled once via regexp.MustCompile
// per the in-repo precompiled-regex idiom (see internal/skill/installer.go:32).
// Accepts digit-dot sequences with an optional `-enterprise` suffix
// (covers semver `5`, `5.20`, `5.20.0`, calver `2026.04`, and the redundant
// `-enterprise` suffix that the edition branch in create.go strips before
// re-applying) plus the bare literal `latest`.
var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*(-enterprise)?$|^latest$`)

// EnterpriseImage maps a Neo4j version token to the enterprise Docker image tag.
// "latest" → neo4j:enterprise (Docker Hub does NOT publish neo4j:latest-enterprise);
// any explicit version → neo4j:<version>-enterprise. Shared by docker create and the
// docker load loader so the tag scheme cannot drift between them.
func EnterpriseImage(version string) string {
	if version == "latest" {
		return "neo4j:enterprise"
	}
	return "neo4j:" + version + "-enterprise"
}

// ValidateVersion enforces the `--version` allowlist (REQ-F-001..005) before
// the value flows into the docker image tag at create.go's image-construction
// block. The contract:
//   - TrimSpace the input before matching so `--version " 5.20 "` is accepted
//     and the trimmed value flows downstream unchanged.
//   - Regex-match against versionPattern. On miss return a clierr.UsageError
//     that names BOTH the expected format and the ORIGINAL (untrimmed) input
//     so the operator sees exactly what they passed.
//   - On hit, strip any trailing `-enterprise` suffix. The image-construction
//     block re-appends `-enterprise` when --edition enterprise, so leaving
//     the suffix in place would yield e.g. `neo4j:5.20-enterprise-enterprise`
//     (unpublished tag, broken pull). Stripping makes the suffix harmless in
//     both editions: enterprise re-adds it, community drops it.
func ValidateVersion(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	if !versionPattern.MatchString(trimmed) {
		return "", clierr.NewUsageError(
			"invalid argument %q for \"--version\" flag: must match digits/dots with optional -enterprise suffix (e.g. 5.20, 5.20.0, 5.20-enterprise, latest)",
			version,
		)
	}
	return strings.TrimSuffix(trimmed, "-enterprise"), nil
}
