// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/neo4j/cli/internal/clierr"
)

const (
	operatorRepository = "neo4j-partners/neo4j-kubernetes-operator"
	// The manifest is currently about 1.6 MiB. The cap prevents a broken or
	// malicious response from filling the temporary filesystem.
	maxOperatorCRDBytes     int64 = 32 << 20
	maxReleaseMetadataBytes int64 = 1 << 20
)

// operatorCRDHTTPDoFn is the HTTP seam for release metadata and asset downloads.
// The default client has a timeout so a stalled GitHub connection cannot block
// an install indefinitely. Tests replace the function and never use the network.
var operatorCRDHTTPDoFn = func(req *http.Request) (*http.Response, error) {
	return (&http.Client{Timeout: 2 * time.Minute}).Do(req)
}

// operatorCRDRelease describes the asset identity and its GitHub API endpoint.
type operatorCRDRelease struct {
	assetName string
	apiURL    string
}

// operatorCRDReleaseForSource accepts only the fixed HTTPS GitHub release URL
// produced by OperatorCRDManifest. This prevents an arbitrary URL from becoming
// a new unauthenticated CRD source through ApplyCRDs.
func operatorCRDReleaseForSource(source string) (operatorCRDRelease, error) {
	u, err := url.Parse(source)
	if err != nil {
		return operatorCRDRelease{}, fmt.Errorf("parse operator CRD URL: %w", err)
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return operatorCRDRelease{}, fmt.Errorf("operator CRD URL must be the HTTPS GitHub release asset")
	}
	prefix := "/" + operatorRepository + "/releases/download/v"
	if !strings.HasPrefix(u.Path, prefix) || !strings.HasSuffix(u.Path, ".yaml") {
		return operatorCRDRelease{}, fmt.Errorf("operator CRD URL is not a supported release asset")
	}
	remainder := strings.TrimSuffix(strings.TrimPrefix(u.Path, prefix), ".yaml")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "neo4j-crd-"+parts[0] {
		return operatorCRDRelease{}, fmt.Errorf("operator CRD URL is not a supported release asset")
	}
	return operatorCRDRelease{
		assetName: parts[1] + ".yaml",
		apiURL:    "https://api.github.com/repos/" + operatorRepository + "/releases/tags/v" + parts[0],
	}, nil
}

// releaseAssetDigest gets the digest GitHub recorded for the exact CRD asset.
// GitHub serves this metadata over HTTPS separately from the asset bytes. A
// release without a digest is refused rather than treated as verified.
func releaseAssetDigest(ctx context.Context, release operatorCRDRelease) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.apiURL, nil)
	if err != nil {
		return "", clierr.NewFatalError("build operator release metadata request: %s", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "neo4j-cli")
	resp, err := operatorCRDHTTPDoFn(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", clierr.NewUpstreamError("download operator release metadata: %s", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", clierr.NewUpstreamError("download operator release metadata: GitHub returned HTTP %d", resp.StatusCode)
	}
	var metadata struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseMetadataBytes)).Decode(&metadata); err != nil {
		return "", clierr.NewUpstreamError("decode operator release metadata: %s", err)
	}
	for _, asset := range metadata.Assets {
		if asset.Name != release.assetName {
			continue
		}
		digest, ok := strings.CutPrefix(asset.Digest, "sha256:")
		if !ok || len(digest) != sha256.Size*2 {
			return "", crdIntegrityError("release asset %q has no valid SHA256 digest", release.assetName)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", crdIntegrityError("release asset %q has no valid SHA256 digest", release.assetName)
		}
		return strings.ToLower(digest), nil
	}
	return "", crdIntegrityError("release asset %q is missing from the GitHub release metadata", release.assetName)
}

// downloadVerifiedCRD downloads source and returns a local path only after its
// SHA256 matches the digest in the GitHub release metadata. The caller owns the
// returned cleanup function and must call it after kubectl finishes.
func downloadVerifiedCRD(ctx context.Context, source string) (string, func(), error) {
	release, err := operatorCRDReleaseForSource(source)
	if err != nil {
		return "", nil, crdIntegrityError("%s", err)
	}
	expected, err := releaseAssetDigest(ctx, release)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", nil, clierr.NewFatalError("build operator CRD download request: %s", err)
	}
	req.Header.Set("User-Agent", "neo4j-cli")
	resp, err := operatorCRDHTTPDoFn(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, clierr.NewUpstreamError("download operator CRD manifest: %s", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", nil, clierr.NewUpstreamError("download operator CRD manifest: GitHub returned HTTP %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "neo4j-cli-operator-crd-*.yaml")
	if err != nil {
		return "", nil, clierr.NewFatalError("create temporary operator CRD manifest: %s", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	removeOnError := func(err error) (string, func(), error) {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(file, io.TeeReader(io.LimitReader(resp.Body, maxOperatorCRDBytes+1), hash))
	if ctx.Err() != nil {
		return removeOnError(ctx.Err())
	}
	if copyErr != nil {
		return removeOnError(clierr.NewUpstreamError("read operator CRD manifest: %s", copyErr))
	}
	if written > maxOperatorCRDBytes {
		return removeOnError(crdIntegrityError("operator CRD manifest exceeds the %d-byte download limit", maxOperatorCRDBytes))
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return removeOnError(crdIntegrityError("SHA256 mismatch for %q: expected %s, got %s", release.assetName, expected, actual))
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, clierr.NewFatalError("close temporary operator CRD manifest: %s", err)
	}
	return path, cleanup, nil
}

func crdIntegrityError(format string, args ...any) *clierr.CLIError {
	return clierr.NewFatalError("operator CRD integrity verification failed: "+format, args...)
}
