// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

// parsedEndpoint is the positional endpoint argument split into the pieces a raw
// Aura request needs: the literal version segment ("v1", "v2beta1", …), the rest
// of the path, and any inline query params.
type parsedEndpoint struct {
	versionPath string
	path        string
	query       url.Values
}

// orgPlaceholders and projectPlaceholders are the tokens substituted from
// --organization-id / --project-id or the default workspace. They are wire path
// shapes rather than CLI input identifiers, hence snake_case.
var (
	orgPlaceholders     = []string{"{org_id}", "{org}"}
	projectPlaceholders = []string{"{project_id}", "{project}"}

	placeholderPattern = regexp.MustCompile(`\{[^{}]*\}`)
)

// resolveEndpoint substitutes the org/project placeholders in raw and then parses
// and validates the result.
//
// Substitution runs first so the structural checks in parseEndpoint also see the
// substituted values, and so a placeholder is found before percent-encoding would
// hide its braces.
func resolveEndpoint(cmd *cobra.Command, cfg *clicfg.Config, raw string) (*parsedEndpoint, error) {
	substituted, err := substitutePlaceholders(cmd, cfg, raw)
	if err != nil {
		return nil, err
	}
	return parseEndpoint(substituted)
}

// substitutePlaceholders replaces the org and project placeholder tokens with
// their resolved, validated IDs. Only the IDs the endpoint actually references
// are resolved, so an unscoped path needs no workspace at all and an org-scoped
// path (e.g. v2beta1/organizations/{org_id}/projects, the call that discovers a
// project ID) does not demand a project.
func substitutePlaceholders(cmd *cobra.Command, cfg *clicfg.Config, raw string) (string, error) {
	if containsAny(raw, orgPlaceholders) {
		orgID, err := utils.ResolveOrgID(cmd, cfg)
		if err != nil {
			return "", err
		}
		if err := checkPlaceholderValue("organization", orgID); err != nil {
			return "", err
		}
		raw = replaceTokens(raw, orgPlaceholders, orgID)
	}

	if containsAny(raw, projectPlaceholders) {
		projectID, err := utils.ResolveProjectID(cmd, cfg)
		if err != nil {
			return "", err
		}
		if err := checkPlaceholderValue("project", projectID); err != nil {
			return "", err
		}
		raw = replaceTokens(raw, projectPlaceholders, projectID)
	}

	// A token left over here is a typo or an unsupported placeholder; without
	// this it would be percent-encoded and shipped upstream as a literal, coming
	// back as a puzzling 404.
	if leftover := placeholderPattern.FindString(raw); leftover != "" {
		return "", clierr.NewUsageError("endpoint contains unsupported placeholder %s", leftover).
			WithSuggestion("Supported placeholders are '{org_id}' (alias '{org}') and '{project_id}' (alias '{project}').")
	}

	return raw, nil
}

// checkPlaceholderValue rejects the characters utils.ValidateResourceID allows
// but which would restructure the endpoint once spliced into it: substitution
// runs before parsing, so a "?" would start the query string, "#" a fragment, and
// "%" an escape sequence.
func checkPlaceholderValue(resourceType, id string) error {
	if strings.ContainsAny(id, "?#%") {
		return clierr.NewValidationError("invalid %s id %q", resourceType, id)
	}
	return nil
}

// parseEndpoint splits the endpoint into a version segment, the remaining path,
// and its inline query, rejecting anything that would send the request somewhere
// other than the already-SSRF-gated base URL.
func parseEndpoint(raw string) (*parsedEndpoint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, clierr.NewUsageError("endpoint is required").
			WithSuggestion("Pass the path including its API version segment, e.g. 'neo4j-cli aura api v1/instances'.")
	}

	// An absolute URL would send the bearer token to a host of the caller's
	// choosing, so only a path relative to the configured base-url is accepted.
	if strings.Contains(trimmed, "://") || strings.HasPrefix(trimmed, "//") {
		return nil, errAbsoluteEndpoint(trimmed)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, clierr.NewUsageError("endpoint %q is not a valid path: %s", trimmed, err.Error())
	}
	if u.Scheme != "" || u.Host != "" {
		return nil, errAbsoluteEndpoint(trimmed)
	}
	if u.Fragment != "" {
		return nil, clierr.NewUsageError("endpoint %q must not contain a '#' fragment", trimmed).
			WithSuggestion("Pass only the path and query string, e.g. 'v1/instances?include_deleted=true'.")
	}

	// url.JoinPath resolves "." and ".." against the base URL, so such a segment
	// would silently retarget the request (the hazard utils.ValidateResourceID
	// guards against for IDs). The decoded path is scanned so a percent-encoded
	// "%2e%2e" is caught too.
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, clierr.NewUsageError("endpoint %q must not contain a %q path segment", trimmed, segment)
		}
	}

	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, clierr.NewUsageError("endpoint query string %q is invalid: %s", u.RawQuery, err.Error())
	}

	// EscapedPath rather than Path, so a percent-encoded segment reaches the
	// server as written.
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	if path == "" {
		return nil, clierr.NewUsageError("endpoint %q has no path", trimmed).
			WithSuggestion("Pass the path including its API version segment, e.g. 'neo4j-cli aura api v1/instances'.")
	}

	versionPath, rest, _ := strings.Cut(path, "/")

	return &parsedEndpoint{versionPath: versionPath, path: rest, query: query}, nil
}

func errAbsoluteEndpoint(endpoint string) error {
	return clierr.NewUsageError("endpoint %q must be a path relative to the Aura API base URL, not an absolute URL", endpoint).
		WithSuggestion("Pass only the path, e.g. 'v1/instances', and use '--base-url' to target a different host.")
}

func containsAny(s string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func replaceTokens(s string, tokens []string, value string) string {
	for _, token := range tokens {
		s = strings.ReplaceAll(s, token, value)
	}
	return s
}
