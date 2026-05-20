// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// homeDirFn is the injectable seam for resolving the operator's home dir when
// expanding a leading `~` in a `--data-dir` / `--logs-dir` / `--import-dir`
// flag value. Production wires os.UserHomeDir; tests can swap a deterministic
// stub. Kept at package scope so the test seam matches the rest of this
// package's seams (clientFactory, randSource, listenerFactory, waitForBoltFn).
var homeDirFn = os.UserHomeDir

// maxNameSuffix caps the auto-suffix walk for name-collision resolution
// (REQ-F-014). The contract is `<name>-1` … `<name>-99`; exceeding it is
// almost certainly operator error (stale containers piling up) and we
// surface that explicitly rather than spinning forever.
const maxNameSuffix = 99

// maxPortOffset caps the port-pair fallback walk (REQ-F-002). Parity with
// `maxNameSuffix=99`: 100 offsets (0..99) is enough headroom for everyday
// collisions but not so high that exhaustion silently hides a deeper bug
// (e.g. stale containers piling up on the host).
const maxPortOffset = 100

// clientFactory is the injectable seam for the dockerClient used by leaves.
// Production wires the exec-backed client (client.go newClient); tests swap
// in a fakeDockerClient (helpers_test.go) without touching the leaf code.
var clientFactory = newClient

// randSource is the injectable seam for crypto-grade random bytes used to
// generate a default Neo4j password (REQ-F-015). Tests swap in a deterministic
// reader so the rendered password is assertable.
var randSource io.Reader = rand.Reader

// listenerFactory is the injectable seam for the port-conflict pre-flight
// (REQ-F-013). Production binds an ephemeral TCP listener on the requested
// host port (closing immediately on success); tests swap in a fake that
// returns sentinel errors keyed by port so we never touch the network.
var listenerFactory = func(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}

// generatedPasswordBytes is the byte length consumed from randSource before
// base64-URL-safe encoding without padding. 16 bytes → 22 base64 characters,
// which is well above the entropy floor for a local Bolt password.
const generatedPasswordBytes = 16

// waitTimeout is the fixed budget for the post-`docker run` Bolt readiness
// probe when --wait is passed (REQ-F-018). The contract pins this at 60s for
// v1 — there is intentionally no --wait-timeout flag. Exposed as a package
// var so tests can shrink it to keep the timeout path fast.
var waitTimeout = 60 * time.Second

// waitForBoltFn is the injectable seam create.go uses to perform the readiness
// probe when --wait is set. Production wires WaitForBolt directly; tests swap
// in a deterministic fake so the wait path can be exercised without standing
// up a real Bolt endpoint.
var waitForBoltFn = WaitForBolt

// versionPattern is the package-level allowlist for `--version` values flowing
// into the docker image tag (REQ-F-002). Compiled once via regexp.MustCompile
// per the in-repo precompiled-regex idiom (see common/skill/installer.go:32).
// Accepts digit-dot sequences with an optional `-enterprise` suffix
// (covers semver `5`, `5.20`, `5.20.0`, calver `2026.04`, and the redundant
// `-enterprise` suffix that the edition branch in create.go strips before
// re-applying) plus the bare literal `latest`.
var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*(-enterprise)?$|^latest$`)

// newCreateCmd builds the `neo4j-cli docker create` leaf. The leaf performs
// the port-conflict pre-flight (REQ-F-013) and the name-collision auto-suffix
// (REQ-F-014) before touching docker so a clash never leaves a half-created
// container behind. --wait, --ephemeral, and --env-out-file land in later tasks.
func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name              string
		version           string
		edition           string
		acceptLicense     bool
		boltPort          int
		httpPort          int
		password          string
		noStoreCredential bool
		wait              bool
		ephemeral         bool
		envOutFile        string
		dataDir           string
		logsDir           string
		importDir         string
	)

	const (
		nameFlag              = "name"
		versionFlag           = "version"
		editionFlag           = "edition"
		acceptLicenseFlag     = "accept-license"
		boltPortFlag          = "bolt-port"
		httpPortFlag          = "http-port"
		passwordFlag          = "password"
		noStoreCredentialFlag = "no-store-credential"
		ephemeralFlag         = "ephemeral"
		envOutFileFlag        = "env-out-file"
		dataDirFlag           = "data-dir"
		logsDirFlag           = "logs-dir"
		importDirFlag         = "import-dir"
	)

	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a local Neo4j Docker container",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a local Neo4j Docker container via `docker run -d` and (unless --no-store-credential) " +
			"store a matching dbms credential so `neo4j-cli query --credential <name>` can connect immediately. " +
			"The container carries `org.neo4j.cli.managed=true` plus a small set of metadata labels — " +
			"Docker itself is the source of truth, no separate state file is maintained. " +
			"When --password is omitted, a 16-byte base64 URL-safe password is generated and surfaced in the output. " +
			"If --name collides with an existing container or stored dbms credential, the chosen name is auto-suffixed " +
			"(`<name>-1`, `<name>-2`, …) and the chosen name is logged to stderr. " +
			"Pass --wait to block until the container's Bolt endpoint accepts sessions (60s timeout); " +
			"on timeout the container is left running so the operator can inspect it with `docker logs <name>`. " +
			"Pass --ephemeral for a throwaway container (`docker run --rm`): no dbms credential is stored and an env-file " +
			"blob (NEO4J_URI / NEO4J_USERNAME / NEO4J_PASSWORD / NEO4J_DATABASE) is emitted to stdout — or, with " +
			"--env-out-file <path>, written to that path (mode 0600) while stdout stays silent so it can be piped into " +
			"`neo4j-cli query --env <path>`. The env-file is written via a temp file in the same directory and " +
			"atomically renamed; a pre-existing symlink at the target path is REPLACED by a regular file (the " +
			"symlink is not followed). " +
			"When the requested --bolt-port and --http-port pair is taken, both ports are auto-incremented by " +
			"the same offset (up to 100 attempts) and the chosen pair is reported on stderr. " +
			"Use --data-dir / --logs-dir / --import-dir to bind-mount host directories at /data, /logs, /import " +
			"inside the container. Paths support `~` and environment-variable expansion and are resolved to absolute " +
			"paths; missing directories are created at mode 0o755. All three volume flags are incompatible with " +
			"--ephemeral.",
		Example: `# Create an enterprise container with auto-generated password and store a dbms credential
neo4j-cli docker create --name dev --rw

# Create a community container on a non-default bolt port; emit JSON for scripting
neo4j-cli docker create --name local --edition community --bolt-port 7688 --http-port 7475 --rw --format json

# Create an enterprise container and block until Bolt is reachable before returning
neo4j-cli docker create --name dev --wait --rw

# Create an ephemeral container and emit an env-file blob to stdout for piping into another tool
neo4j-cli docker create --name tmp --ephemeral --rw

# Create an ephemeral container and write the env-file to a path that 'query --env' can consume
neo4j-cli docker create --name tmp --ephemeral --env-out-file /tmp/n.env --rw

# Persist data on the host so it survives delete + recreate
neo4j-cli docker create --name dev --data-dir ~/n4j-data --rw

# Create an enterprise container with the commercial license accepted and a custom password (no credential stored)
neo4j-cli docker create --name licensed --edition enterprise --accept-license --password mysecret --no-store-credential --rw`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --edition. Cobra has no built-in enum validator that
			// surfaces a clierr.UsageError; we do the check manually so the
			// error rendering matches the rest of the docker subtree.
			if edition != "community" && edition != "enterprise" {
				return clierr.NewUsageError(`invalid argument %q for "--%s" flag: must be one of "community" or "enterprise"`, edition, editionFlag)
			}

			// Validate --version against the package-level allowlist BEFORE
			// any other pre-flight or docker side effect (REQ-F-001..005).
			// The canonical form is reassigned to the outer `version` so the
			// image-construction block, LabelVersion label, and output row
			// all see the trimmed / -enterprise-stripped value.
			canonicalVersion, err := validateVersion(version)
			if err != nil {
				return err
			}
			version = canonicalVersion

			// --env-out-file / --ephemeral compatibility (REQ-F-017). --env-out-file
			// is a child of --ephemeral (it only changes WHERE the env blob
			// goes); rejecting it standalone keeps the contract honest.
			// --no-store-credential + --ephemeral is redundant: ephemeral
			// already skips persistence — error out so the operator notices.
			if envOutFile != "" && !ephemeral {
				return clierr.NewUsageError("--%s requires --%s", envOutFileFlag, ephemeralFlag)
			}
			if ephemeral && noStoreCredential {
				return clierr.NewUsageError("--%s is incompatible with --%s (ephemeral already skips credential persistence)", noStoreCredentialFlag, ephemeralFlag)
			}

			// Volume-mount flags (--data-dir / --logs-dir / --import-dir) are
			// incompatible with --ephemeral: ephemeral containers do not
			// persist data, so a bind-mount on an ephemeral container is
			// almost certainly operator error. Fire BEFORE port pre-flight so
			// a misconfigured invocation doesn't waste cycles on listener
			// checks.
			volumeFlags := []struct {
				flag      string
				value     string
				container string
			}{
				{dataDirFlag, dataDir, "/data"},
				{logsDirFlag, logsDir, "/logs"},
				{importDirFlag, importDir, "/import"},
			}
			for _, vol := range volumeFlags {
				if vol.value != "" && ephemeral {
					return clierr.NewUsageError(
						"--%s is incompatible with --%s (ephemeral containers do not persist data; mount and ephemeral are mutually exclusive)",
						vol.flag, ephemeralFlag,
					)
				}
			}

			// Port-conflict pre-flight (REQ-F-013, REQ-F-001..007). Run BEFORE
			// any docker side effect so a port clash never leaves a half-
			// created container behind. Equal-ports check fires first so we
			// don't confusingly walk the loop with a pair that can never be
			// valid. On clash we auto-increment BOTH ports by the same
			// offset (up to maxPortOffset) so the bolt/http delta the
			// operator picked is preserved.
			if boltPort == httpPort {
				return clierr.NewUsageError("--%s and --%s must be different (got %d for both)", boltPortFlag, httpPortFlag, boltPort)
			}
			reqBoltPort, reqHTTPPort := boltPort, httpPort
			resolvedBolt, resolvedHTTP, err := findFreePortPair(reqBoltPort, reqHTTPPort)
			if err != nil {
				return err
			}
			boltPort, httpPort = resolvedBolt, resolvedHTTP
			if boltPort != reqBoltPort || httpPort != reqHTTPPort {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: ports %d/%d in use; using %d/%d (bolt/http)\n", reqBoltPort, reqHTTPPort, boltPort, httpPort)
			}

			// Name-collision pre-flight (REQ-F-014). Enumerate ALL container
			// names from docker (managed or not — docker enforces global name
			// uniqueness) AND every stored dbms credential name. Pick the
			// requested name when free; otherwise try <name>-1 … <name>-99.
			client := clientFactory()
			ctx := cmd.Context()
			chosenName, err := resolveContainerName(ctx, client, cfg, name)
			if err != nil {
				return err
			}
			if chosenName != name {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: name %q already in use; using %q\n", name, chosenName)
			}

			// Resolve password: honour --password verbatim, otherwise generate
			// crypto/rand bytes and base64 URL-safe encode without padding
			// (REQ-F-015). The seam lets tests assert determinism.
			resolvedPassword := password
			if resolvedPassword == "" {
				buf := make([]byte, generatedPasswordBytes)
				if _, err := io.ReadFull(randSource, buf); err != nil {
					return fmt.Errorf("docker create: generate password: %w", err)
				}
				resolvedPassword = base64.RawURLEncoding.EncodeToString(buf)
			}

			// Resolve image (REQ-F-011). Tag scheme verified against
			// https://hub.docker.com/_/neo4j and the v2 tags API:
			//   - community + any version → neo4j:<version>      (latest, 5, 5.26, 2026.04, …)
			//   - enterprise + explicit version → neo4j:<version>-enterprise
			//   - enterprise + "latest" → neo4j:enterprise        (Docker Hub does NOT publish neo4j:latest-enterprise)
			image := "neo4j:" + version
			if edition == "enterprise" {
				if version == "latest" {
					image = "neo4j:enterprise"
				} else {
					image = "neo4j:" + version + "-enterprise"
				}
			}

			// Build the docker run argv. Order matters for tests asserting
			// shape; keep ports → env → labels → image (last). When
			// --ephemeral, prepend --rm so the docker daemon auto-removes
			// the container on exit and flip the ephemeral label to "true"
			// so `docker list` / `docker get` surface the choice (REQ-F-017).
			argv := []string{"--name", chosenName}
			if ephemeral {
				argv = append(argv, "--rm")
			}
			argv = append(argv, "-p", fmt.Sprintf("%d:7474", httpPort))
			argv = append(argv, "-p", fmt.Sprintf("%d:7687", boltPort))
			argv = append(argv, "-e", "NEO4J_AUTH=neo4j/"+resolvedPassword)
			if edition == "enterprise" {
				licenseValue := "eval"
				if acceptLicense {
					licenseValue = "yes"
				}
				argv = append(argv, "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT="+licenseValue)
			}
			// Resolve and mount host directories. Each resolved path goes
			// through expand-home + ExpandEnv + filepath.Abs + mkdir-if-missing
			// via resolveHostDir; errors here are fail-loud so the operator
			// sees the bad path before any docker side effect. Slotted between
			// env and labels per the documented argv shape.
			for _, vol := range volumeFlags {
				if vol.value == "" {
					continue
				}
				resolved, err := resolveHostDir(cmd, cfg.Aura.Fs(), vol.flag, vol.value)
				if err != nil {
					return err
				}
				argv = append(argv, "-v", resolved+":"+vol.container)
			}
			ephemeralLabelValue := "false"
			if ephemeral {
				ephemeralLabelValue = "true"
			}
			argv = append(argv, "--label", LabelManaged+"=true")
			argv = append(argv, "--label", LabelEdition+"="+edition)
			argv = append(argv, "--label", LabelVersion+"="+version)
			argv = append(argv, "--label", LabelBoltPort+"="+strconv.Itoa(boltPort))
			argv = append(argv, "--label", LabelHTTPPort+"="+strconv.Itoa(httpPort))
			argv = append(argv, "--label", LabelEphemeral+"="+ephemeralLabelValue)
			argv = append(argv, image)

			if _, err := client.Run(ctx, argv); err != nil {
				// dockerClient.Run already wraps stderr verbatim (REQ-F-061)
				// in a clierr.UsageError, so we surface as-is.
				cmd.SilenceUsage = true
				return err
			}

			uri := fmt.Sprintf("neo4j://localhost:%d", boltPort)

			// Persist a matching dbms credential unless explicitly opted out
			// or running ephemerally. Ephemeral containers leave no on-disk
			// footprint — the credential travels via the env-file blob
			// emitted below (REQ-F-017). Database name defaults to "neo4j"
			// for local containers per existing credential conventions.
			if !noStoreCredential && !ephemeral {
				if cfg.Credentials == nil || cfg.Credentials.Dbms == nil {
					return clierr.NewUsageError("credential storage is not available; use --%s to skip storing credentials locally", noStoreCredentialFlag)
				}
				if err := cfg.Credentials.Dbms.Add(chosenName, "neo4j", resolvedPassword, "neo4j", uri); err != nil {
					return err
				}
			}

			// --wait (REQ-F-018): block until the container's Bolt endpoint
			// accepts sessions or waitTimeout elapses. Narrate ONCE on stderr
			// before polling so an operator watching the terminal knows the
			// CLI is waiting on purpose. On timeout we surface the
			// WaitForBolt error verbatim and leave the container running —
			// the partially-started Neo4j may still finish booting after we
			// return, and `docker logs <name>` is the right next step (the
			// error message points there).
			if wait {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: waiting for Bolt on localhost:%d...\n", boltPort)
				if err := waitForBoltFn(ctx, uri, "neo4j", resolvedPassword, waitTimeout); err != nil {
					cmd.SilenceUsage = true
					return err
				}
			}

			// --ephemeral replaces the standard table/JSON output with a
			// `.env` file blob suitable for `query --env <path>` (REQ-F-017).
			// With --env-out-file we write to disk via cfg.Aura.Fs() with 0600
			// perms and stay silent on stdout (so callers can pipe). Without
			// --env-out-file we emit the blob to stdout.
			if ephemeral {
				blob := renderEnvFile(chosenName, image, uri, resolvedPassword)
				if envOutFile != "" {
					if err := writeEnvFile(cfg.Aura.Fs(), envOutFile, blob); err != nil {
						cmd.SilenceUsage = true
						return err
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: wrote credentials to %s\n", envOutFile)
				} else {
					_, _ = fmt.Fprint(cmd.OutOrStdout(), blob)
				}
				return nil
			}

			// Render the result. Field order mirrors what an operator wants
			// at a glance: identity, image identity, ports, connection details.
			row := map[string]any{
				"name":      chosenName,
				"edition":   edition,
				"version":   version,
				"bolt-port": boltPort,
				"http-port": httpPort,
				"uri":       uri,
				"username":  "neo4j",
				"password":  resolvedPassword,
			}
			fields := []string{"name", "edition", "version", "bolt-port", "http-port", "uri", "username", "password"}
			commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: row}, fields)

			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Container name. Also used as the dbms credential name.")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	cmd.Flags().StringVar(&version, versionFlag, "latest", "Neo4j version tag (e.g. 5.20, latest).")
	cmd.Flags().StringVar(&edition, editionFlag, "enterprise", `Neo4j edition. Must be one of "community" or "enterprise".`)
	cmd.Flags().BoolVar(&acceptLicense, acceptLicenseFlag, false, "Accept the Neo4j Commercial License (sets NEO4J_ACCEPT_LICENSE_AGREEMENT=yes; default is eval). Ignored for community edition.")
	cmd.Flags().IntVar(&boltPort, boltPortFlag, 7687, "Host port to publish for Bolt (container 7687). Auto-incremented along with --http-port if taken.")
	cmd.Flags().IntVar(&httpPort, httpPortFlag, 7474, "Host port to publish for the HTTP browser (container 7474). Auto-incremented along with --bolt-port if taken.")
	cmd.Flags().StringVar(&password, passwordFlag, "", "Neo4j password. When empty, a 16-byte base64 URL-safe password is generated.")
	cmd.Flags().BoolVar(&noStoreCredential, noStoreCredentialFlag, false, "Skip persisting a dbms credential for this container.")
	cmd.Flags().BoolVar(&ephemeral, ephemeralFlag, false, "Run with `docker run --rm`; skip credential persistence and emit a .env blob consumable by `query --env`.")
	cmd.Flags().StringVar(&envOutFile, envOutFileFlag, "", "When --ephemeral, write the .env blob to this path (mode 0600) instead of stdout. Writes via a temp file in the same directory and atomically renames; a pre-existing symlink at the path is replaced by a regular file.")
	cmd.Flags().StringVar(&dataDir, dataDirFlag, "", "Host directory to bind-mount at /data inside the container. Empty = no mount (data lives in the container layer and is lost on delete). Path supports `~` and environment-variable expansion; resolved to an absolute path; created at mode 0o755 if missing. Incompatible with --ephemeral.")
	cmd.Flags().StringVar(&logsDir, logsDirFlag, "", "Host directory to bind-mount at /logs inside the container. Empty = no mount. Same expansion + mkdir rules as --data-dir. Incompatible with --ephemeral.")
	cmd.Flags().StringVar(&importDir, importDirFlag, "", "Host directory to bind-mount at /import inside the container (used by Neo4j's LOAD CSV). Empty = no mount. Same expansion + mkdir rules as --data-dir. Incompatible with --ephemeral.")
	flags.RegisterWait(cmd, &wait, "Wait until Bolt is reachable before returning.")

	return cmd
}

// renderEnvFile builds the .env blob consumed by `neo4j-cli query --env <path>`
// (REQ-F-017). The variable names mirror neo4j-cli/query/connect.go so the
// blob is a drop-in for the existing flow. A trailing newline keeps `cat`-style
// inspection clean.
func renderEnvFile(name, image, uri, password string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# neo4j-cli docker — %s @ %s\n", name, image)
	fmt.Fprintf(&b, "NEO4J_URI=%s\n", uri)
	b.WriteString("NEO4J_USERNAME=neo4j\n")
	fmt.Fprintf(&b, "NEO4J_PASSWORD=%s\n", password)
	b.WriteString("NEO4J_DATABASE=neo4j\n")
	return b.String()
}

// writeEnvFile writes the blob to path via the cfg-supplied afero filesystem
// using a temp-file + atomic-rename strategy (REQ-F-017 / REQ-NF-004). This
// closes two interlocking issues vs. an OpenFile-then-Chmod flow:
//   - symlink follow on open: POSIX open() follows symlinks by default; an
//     attacker with write access to the containing dir could plant <path> as
//     a symlink to e.g. ~/.ssh/authorized_keys and have the generated Neo4j
//     password written there with O_TRUNC semantics.
//   - TOCTOU between OpenFile and Chmod: the window between syscalls plus a
//     swap-in symlink could land the credential on disk in the wrong place.
//
// afero.TempFile produces a fresh `.neo4j-cli-env-<rand>` path in the same
// directory as the final path; we chmod the temp while we still own it, then
// fs.Rename (atomic on POSIX) replaces whatever is at <path> — including a
// pre-existing symlink — with the regular temp file. Any error path after
// temp creation removes the temp file best-effort so a stray ^C does not
// leak. Routing through the afero seam keeps unit tests hermetic; production
// hits the real OS fs.
//
// Behaviour change documented in --env-out-file's flag Long: and the README /
// additions.md docker section: a pre-existing symlink at the target path is
// replaced by a regular file (the symlink is NOT followed).
func writeEnvFile(fs afero.Fs, path, contents string) error {
	dir := filepath.Dir(path)
	tmp, err := afero.TempFile(fs, dir, ".neo4j-cli-env-")
	if err != nil {
		return fmt.Errorf("docker create: create temp env-file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = fs.Remove(tmpPath) }

	if _, werr := tmp.WriteString(contents); werr != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("docker create: write temp env-file %s: %w", tmpPath, werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		cleanup()
		return fmt.Errorf("docker create: close temp env-file %s: %w", tmpPath, cerr)
	}
	// Chmod while we still own the temp. The temp path is fresh and O_EXCL-
	// guarded by afero.TempFile's random suffix, so there is no symlink-swap
	// TOCTOU window: any attacker-controlled path manipulation in dir would
	// have to win against the random suffix, which is cryptographically
	// infeasible per crypto/rand.
	if cerr := fs.Chmod(tmpPath, 0o600); cerr != nil {
		cleanup()
		return fmt.Errorf("docker create: chmod temp env-file %s: %w", tmpPath, cerr)
	}
	// Atomic rename (POSIX). Replaces whatever is at <path> — a regular file,
	// a symlink, anything — with our temp file. On Windows fs.Rename is
	// atomic on modern Go/NTFS. On failure cleanup runs so the temp does not
	// accumulate.
	if rerr := fs.Rename(tmpPath, path); rerr != nil {
		cleanup()
		return fmt.Errorf("docker create: rename env-file to %s: %w", path, rerr)
	}
	return nil
}

// singleRow adapts a single map[string]any into a commonoutput.ResponseData so
// PrintBodyMap can render it as a one-row table, a JSON array, or a TOON
// document. We marshal as a JSON array (matching credential dbms list's
// PrintableDbmsCredentials shape) so downstream consumers always see the same
// top-level type regardless of cardinality.
type singleRow struct {
	row map[string]any
}

// AsArray implements commonoutput.ResponseData. Always returns a one-element
// slice so PrintBodyMap renders a single row / object.
func (s singleRow) AsArray() []map[string]any {
	return []map[string]any{s.row}
}

// MarshalJSON returns the JSON array form, matching what AsArray emits, so
// PrintBodyMap's encoding/json path renders the row as `[{...}]` instead of
// the struct's default `{"row":{...}}` shape.
func (s singleRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.AsArray())
}

// portFree binds and immediately releases a TCP listener on the given host
// port via the listenerFactory seam. Returns true when the port is free
// (i.e. the listener bound successfully); false otherwise. On success the
// listener is closed before returning so the real `docker run` call can
// claim the port a moment later.
func portFree(port int) bool {
	ln, err := listenerFactory(port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// findFreePortPair walks the port-pair fallback loop (REQ-F-001..007).
// Starting from (boltStart, httpStart) it tries offsets 0..maxPortOffset-1,
// returning the first pair where BOTH ports bind successfully. The same
// offset is applied to both ports so the operator's bolt/http delta is
// preserved across the fallback. On exhaustion a clierr.UsageError points
// the operator at --bolt-port / --http-port so they can pin a free pair
// explicitly.
func findFreePortPair(boltStart, httpStart int) (int, int, error) {
	for offset := 0; offset < maxPortOffset; offset++ {
		bolt := boltStart + offset
		http := httpStart + offset
		if portFree(bolt) && portFree(http) {
			return bolt, http, nil
		}
	}
	return 0, 0, clierr.NewUsageError(
		"could not find a free port pair starting at %d/%d after %d attempts; pass --bolt-port / --http-port",
		boltStart, httpStart, maxPortOffset,
	)
}

// resolveContainerName implements the REQ-F-014 name-collision contract.
// It enumerates existing names from docker (all containers, managed or
// not — docker enforces global container-name uniqueness) and from the
// stored dbms credentials, then returns the requested name when free or
// the first non-colliding `<name>-<i>` suffix in 1..maxNameSuffix.
// Returns a clierr.UsageError when every suffix in that range is taken
// so the operator gets a clear "pick a different --name" hint.
func resolveContainerName(ctx context.Context, client dockerClient, cfg *clicfg.Config, requested string) (string, error) {
	used, err := collectUsedNames(ctx, client, cfg)
	if err != nil {
		return "", err
	}
	if _, taken := used[requested]; !taken {
		return requested, nil
	}
	for i := 1; i <= maxNameSuffix; i++ {
		candidate := fmt.Sprintf("%s-%d", requested, i)
		if _, taken := used[candidate]; !taken {
			return candidate, nil
		}
	}
	return "", clierr.NewUsageError(
		"could not find a free name for %q after trying %s-1 through %s-%d; pass --name <other>",
		requested, requested, requested, maxNameSuffix,
	)
}

// collectUsedNames merges docker container names (from PsAll, unfiltered so
// unmanaged containers count too) with stored dbms credential names into a
// single set used for collision detection. The set is conservative: any
// PsEntry.Names value gets split on `,` and trimmed because Docker emits
// multi-name entries as a comma-separated string.
func collectUsedNames(ctx context.Context, client dockerClient, cfg *clicfg.Config) (map[string]struct{}, error) {
	used := map[string]struct{}{}

	entries, err := client.PsAll(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		for _, n := range strings.Split(entry.Names, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				used[n] = struct{}{}
			}
		}
	}

	if cfg != nil && cfg.Credentials != nil && cfg.Credentials.Dbms != nil {
		for _, cred := range cfg.Credentials.Dbms.List() {
			if cred != nil && cred.Name != "" {
				used[cred.Name] = struct{}{}
			}
		}
	}
	return used, nil
}

// expandHostPath resolves a user-supplied host directory string into an
// absolute path. The expansion order:
//  1. `~` or `~/...` at the start of the path resolves to the operator's
//     home directory (via the homeDirFn seam).
//  2. Embedded environment variables are expanded via os.ExpandEnv (so
//     `$HOME/x` and `${HOME}/x` both work).
//  3. The result is run through filepath.Abs so docker never sees a relative
//     path.
//
// Empty input returns empty output and a nil error so callers can keep their
// "skip when empty" branches simple.
func expandHostPath(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	// Tilde expansion must happen before ExpandEnv so a value like `~/$FOO`
	// gets the HOME swap on the leading `~` while $FOO still resolves.
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		home, err := homeDirFn()
		if err != nil {
			return "", fmt.Errorf("expand ~: %w", err)
		}
		if s == "~" {
			s = home
		} else {
			s = filepath.Join(home, s[2:])
		}
	}
	s = os.ExpandEnv(s)
	abs, err := filepath.Abs(s)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	return abs, nil
}

// validateVersion enforces the `--version` allowlist (REQ-F-001..005) before
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
func validateVersion(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	if !versionPattern.MatchString(trimmed) {
		return "", clierr.NewUsageError(
			"invalid argument %q for \"--version\" flag: must match digits/dots with optional -enterprise suffix (e.g. 5.20, 5.20.0, 5.20-enterprise, latest)",
			version,
		)
	}
	return strings.TrimSuffix(trimmed, "-enterprise"), nil
}

// resolveHostDir expands a `--data-dir` / `--logs-dir` / `--import-dir` flag
// value into a docker-ready absolute path: it runs expandHostPath, ensures
// the resolved directory exists (creating at mode 0o755 if missing), and
// narrates a single `info: created host directory <path>` line to stderr
// when the directory was created. Routing through the supplied afero.Fs keeps
// unit tests hermetic; production passes cfg.Aura.Fs() which is backed by the
// real OS fs.
//
// flagName is only used for error rendering — it identifies which of the
// three volume flags failed so the operator can act.
func resolveHostDir(cmd *cobra.Command, fs afero.Fs, flagName, raw string) (string, error) {
	resolved, err := expandHostPath(raw)
	if err != nil {
		return "", clierr.NewUsageError("--%s: %s", flagName, err.Error())
	}
	_, statErr := fs.Stat(resolved)
	if statErr == nil {
		return resolved, nil
	}
	if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("docker create: stat --%s %s: %w", flagName, resolved, statErr)
	}
	// Missing directory — create it now. 0o755 (NOT 0o700) lets the
	// container's root-owned entrypoint chown the mounted dir to the neo4j
	// UID at startup; restricting to 0o700 would break that step on first
	// boot. The operator can chmod down later if they want to.
	if mkErr := fs.MkdirAll(resolved, 0o755); mkErr != nil {
		return "", fmt.Errorf("docker create: create --%s %s: %w", flagName, resolved, mkErr)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: created host directory %s\n", resolved)
	return resolved, nil
}
