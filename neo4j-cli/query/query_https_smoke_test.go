// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !windows

package query

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
)

const (
	httpsContainerName = "neo4j-https-test"
	httpsImage         = "neo4j:latest"
	httpsPassword      = "testtest"
	httpsReadyTimeout  = 60 * time.Second
)

// TestHTTPS_Smoke is an env-gated pure-Go integration test. It boots a real
// neo4j:latest container with HTTPS enabled (using a stdlib-generated self-signed
// cert) and verifies the `--insecure` flag end-to-end. Skipped by default so
// `go test ./...` is unaffected; opt in with NEO4J_HTTPS_TEST=1.
//
// Requires: docker on PATH. No openssl, curl, or shell scripts.
//
// Build constraint: Unix-only (Linux container model).
func TestHTTPS_Smoke(t *testing.T) {
	if os.Getenv("NEO4J_HTTPS_TEST") != "1" {
		t.Skip("set NEO4J_HTTPS_TEST=1 to run; needs docker")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required on PATH but was not found: %v", err)
	}

	certDir := t.TempDir()
	generateSelfSignedCert(t, certDir)

	httpsPort := freePort(t)
	httpPort := freePort(t)

	// Pre-clean any leftover container from a crashed previous run.
	_ = exec.Command("docker", "rm", "-f", httpsContainerName).Run()

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", httpsContainerName).Run()
	})

	bootNeo4jHTTPS(t, certDir, httpsPort, httpPort)
	waitForHTTPSReady(t, httpsPort)

	uri := fmt.Sprintf("https://127.0.0.1:%d", httpsPort)

	// Positive: --insecure must succeed and stdout must contain "1".
	t.Run("positive_insecure_succeeds", func(t *testing.T) {
		stdout, stderr, err := runQueryCmd(t, []string{
			"--uri", uri,
			"--insecure",
			"-u", "neo4j",
			"-p", httpsPassword,
			"RETURN 1 AS n",
		})
		require.NoError(t, err, "stdout=%q stderr=%q", stdout, stderr)
		assert.Contains(t, stdout, "1", "positive output must contain '1'")
	})

	// Negative: default verification must reject the self-signed cert with
	// a TLS-related error.
	t.Run("negative_default_rejects_self_signed", func(t *testing.T) {
		stdout, stderr, err := runQueryCmd(t, []string{
			"--uri", uri,
			"-u", "neo4j",
			"-p", httpsPassword,
			"RETURN 1",
		})
		require.Error(t, err, "default verification must fail; stdout=%q stderr=%q", stdout, stderr)
		// Wording varies by Go version; match any of tls/x509/certificate.
		matched := regexp.MustCompile(`(?i)tls|x509|certificate`).MatchString(err.Error())
		assert.True(t, matched, "error message must mention TLS/x509/certificate; got: %v", err)
	})
}

// freePort allocates and immediately releases a TCP port on 127.0.0.1. The
// tiny TOCTOU window between Close and `docker run -p` is acceptable for a
// dev-only smoke test and is far better than failing hard on hard-coded
// 7473/7474 collisions.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// generateSelfSignedCert writes private.key + public.crt (0644) and trusted/
// + revoked/ subdirs (0755) into dir. RSA-2048, CN=localhost, 30-day
// validity. Neo4j 5 requires both trusted/ and revoked/ to exist even when
// client_auth=NONE.
func generateSelfSignedCert(t *testing.T, dir string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPath := filepath.Join(dir, "public.crt")
	keyPath := filepath.Join(dir, "private.key")

	certPEM, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}))
	require.NoError(t, certPEM.Close())

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	keyPEM, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(keyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	require.NoError(t, keyPEM.Close())

	// Re-assert 0644 on cert files; default umask may have masked OpenFile
	// mode bits. Container reader runs as uid 7474 and the bind mount is
	// read-only.
	require.NoError(t, os.Chmod(certPath, 0o644))
	require.NoError(t, os.Chmod(keyPath, 0o644))

	for _, sub := range []string{"trusted", "revoked"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o755))
	}

	// t.TempDir creates with 0700 by default; loosen so the in-container uid
	// 7474 user can traverse it on the read-only bind mount.
	require.NoError(t, os.Chmod(dir, 0o755))
}

// bootNeo4jHTTPS starts the neo4j:latest container detached, with HTTPS enabled
// and the cert dir bind-mounted read-only at /ssl. Same env vars as the
// previous shell script.
func bootNeo4jHTTPS(t *testing.T, certDir string, httpsPort, httpPort int) {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--name", httpsContainerName,
		"-p", fmt.Sprintf("%d:7473", httpsPort),
		"-p", fmt.Sprintf("%d:7474", httpPort),
		"-e", "NEO4J_AUTH=neo4j/"+httpsPassword,
		"-e", "NEO4J_server_https_enabled=true",
		"-e", "NEO4J_dbms_ssl_policy_https_enabled=true",
		"-e", "NEO4J_dbms_ssl_policy_https_base__directory=/ssl",
		"-e", "NEO4J_dbms_ssl_policy_https_private__key=private.key",
		"-e", "NEO4J_dbms_ssl_policy_https_public__certificate=public.crt",
		"-e", "NEO4J_dbms_ssl_policy_https_client__auth=NONE",
		"-v", certDir+":/ssl:ro",
		httpsImage,
	).CombinedOutput()
	require.NoError(t, err, "docker run failed: %s", string(out))
}

// waitForHTTPSReady polls the HTTPS endpoint with InsecureSkipVerify for up
// to httpsReadyTimeout. On timeout dumps the last 50 lines of container
// logs to test output before failing.
func waitForHTTPSReady(t *testing.T, httpsPort int) {
	t.Helper()
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // smoke test
		},
		Timeout: 5 * time.Second,
	}
	url := fmt.Sprintf("https://127.0.0.1:%d", httpsPort)

	deadline := time.Now().Add(httpsReadyTimeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(1 * time.Second)
	}

	logs, _ := exec.Command("docker", "logs", "--tail", "50", httpsContainerName).CombinedOutput()
	t.Logf("--- last 50 lines of %s logs ---\n%s", httpsContainerName, string(logs))
	t.Fatalf("Neo4j HTTPS endpoint at %s did not become ready within %s", url, httpsReadyTimeout)
}

// runQueryCmd drives the `query` cobra command in-process with the given
// args, returning captured stdout, stderr, and the Execute() error. Mirrors
// the harness shape in run_test.go but inlined here so the smoke test stays
// self-contained.
func runQueryCmd(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"aura":{"output":"table"}}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := NewCmd(cfg)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	execErr := cmd.Execute()
	return stdout.String(), stderr.String(), execErr
}
