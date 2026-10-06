// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"encoding/json"
	"fmt"
	"github.com/neo4j/cli/internal/clicfg/credentials"
	engine "github.com/neo4j/cli/internal/docker"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/debug"
	"github.com/neo4j/cli/internal/flags"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// homeDirFn is the injectable seam for resolving the operator's home dir when
// expanding a leading `~` in a `--data-dir` / `--logs-dir` / `--import-dir`
// flag value. Production wires os.UserHomeDir; tests can swap a deterministic
// stub. Kept at package scope so the test seam matches the rest of this
// package's seams (clientFactory, randSource, listenerFactory, waitForBoltFn).
var homeDirFn = os.UserHomeDir

// clientFactory is the injectable seam for the engine.Client used by leaves.
// Production wires the exec-backed client (engine.NewClient); tests swap in an
// engine.FakeClient without touching the leaf code.
var clientFactory = engine.NewClient

// storeCredentialFn records the new container's credential. It is a seam so
// tests can force the storage failure that can only happen after the container
// has started.
var storeCredentialFn = func(dbms *credentials.DbmsCredentials, name, password, uri string) error {
	return dbms.Add(name, "neo4j", password, "neo4j", uri)
}

// waitTimeout is the fixed budget for the post-`docker run` Bolt readiness
// probe when --wait is passed (REQ-F-018). The contract pins this at 60s for
// v1 — there is intentionally no --wait-timeout flag. Exposed as a package
// var so tests can shrink it to keep the timeout path fast.
var waitTimeout = 60 * time.Second

// waitForBoltFn is the injectable seam create.go uses to perform the readiness
// probe when --wait is set. Production wires WaitForBolt directly; tests swap
// in a deterministic fake so the wait path can be exercised without standing
// up a real Bolt endpoint.
var waitForBoltFn = engine.WaitForBolt

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
		noPrintPassword   bool
		wait              bool
		ephemeral         bool
		envOutFile        string
		dataDir           string
		logsDir           string
		importDir         string
		plugins           []string
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
		noPrintPasswordFlag   = "no-print-password"
		ephemeralFlag         = "ephemeral"
		envOutFileFlag        = "env-out-file"
		dataDirFlag           = "data-dir"
		logsDirFlag           = "logs-dir"
		importDirFlag         = "import-dir"
		pluginFlag            = "plugin"
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
			"--ephemeral. " +
			"Pass --plugin (repeatable) to install Neo4j plugins such as apoc or graph-data-science; they are set through NEO4J_PLUGINS and installed by the image at startup, so a misspelt name shows up in `docker logs <name>` rather than here. " +
			"Pass --no-print-password to omit the generated password from stdout output. " +
			"The stored credential still connects via `--credential <name>` (no plaintext needed). " +
			"If credential storage is unavailable, or the name is reserved or already stored, the command fails before any container is created. " +
			"If storing the credential fails after the container has started, a warning is printed and the password is still shown; " +
			"with --no-print-password a generated password would be unrecoverable, so the container is removed and the command fails. " +
			"The password is not readable through the CLI. " +
			"To set a known password, run " +
			"`admin user set-password neo4j --new-password <s> --credential <name> --rw`, " +
			"then resync the stored credential with `credential dbms remove <name>` plus `credential dbms add`.",
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

# Create a container with the APOC and Graph Data Science plugins installed
neo4j-cli docker create --name gds --plugin apoc --plugin graph-data-science --rw

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
			canonicalVersion, err := engine.ValidateVersion(version)
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
			// --no-print-password incompatibilities. The flag is meaningful only
			// when the password remains recoverable through some other channel
			// (stored dbms credential, operator-supplied --password, or the
			// ephemeral .env blob via --env-out-file). Reject combos that would
			// leave NO recovery path.
			if noPrintPassword && ephemeral {
				return clierr.NewUsageError(
					"--%s is incompatible with --%s (ephemeral emits a .env blob; use --%s to write it to a file)",
					noPrintPasswordFlag, ephemeralFlag, envOutFileFlag,
				)
			}
			if noPrintPassword && noStoreCredential && password == "" {
				return clierr.NewUsageError(
					"--%s with --%s would discard the generated password unrecoverably; supply --%s explicitly or drop one of the flags",
					noPrintPasswordFlag, noStoreCredentialFlag, passwordFlag,
				)
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

			// Fail on everything that can be known up front BEFORE any docker
			// side effect, so a rejected invocation never leaves a running
			// container behind.
			//
			// Credential storage: a created container whose password cannot be
			// recorded is a running container nobody can log in to. Check that
			// the store exists first (storing can still fail later; see below).
			persistCredential := !noStoreCredential && !ephemeral
			if persistCredential && cfg.DbmsCredentials() == nil {
				return clierr.NewUsageError("credential storage is not available; use --%s to skip storing credentials locally", noStoreCredentialFlag)
			}

			validPlugins, err := engine.ValidatePlugins(plugins)
			if err != nil {
				return err
			}

			// Port-conflict and name-collision pre-flight (REQ-F-013, REQ-F-014),
			// plus the password. Equal-ports fires first so we don't walk the
			// fallback loop with a pair that can never be valid.
			if boltPort == httpPort {
				return clierr.NewUsageError("--%s and --%s must be different (got %d for both)", boltPortFlag, httpPortFlag, boltPort)
			}
			client := clientFactory(debug.Resolve(cmd))
			ctx := cmd.Context()
			res, err := engine.Reserve(ctx, client, cfg.DbmsCredentials(), engine.Want{
				Name:     name,
				BoltPort: boltPort,
				HTTPPort: httpPort,
				Password: password,
			}, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			chosenName, resolvedPassword := res.Name, res.Password
			boltPort, httpPort = res.BoltPort, res.HTTPPort
			if persistCredential {
				// A name Add is certain to reject (reserved, or already stored)
				// must fail now, before the container exists.
				if err := cfg.DbmsCredentials().CheckName(chosenName); err != nil {
					return err
				}
			}

			// Resolve and mount host directories. Each path goes through
			// expand-home + ExpandEnv + filepath.Abs + mkdir-if-missing via
			// resolveHostDir; errors are fail-loud so the operator sees a bad path
			// before the container starts.
			var mounts []engine.Mount
			for _, vol := range volumeFlags {
				if vol.value == "" {
					continue
				}
				resolved, err := resolveHostDir(cmd, cfg.Fs(), vol.flag, vol.value)
				if err != nil {
					return err
				}
				mounts = append(mounts, engine.Mount{Source: resolved, Target: vol.container})
			}

			// The server container. engine.ServerSpec owns the argv shape, the
			// image tag scheme and the labels; the password reaches the container
			// through the docker process environment, never argv.
			spec := engine.ServerSpec{
				Name:          chosenName,
				Edition:       engine.Edition(edition),
				Version:       version,
				BoltPort:      boltPort,
				HTTPPort:      httpPort,
				AcceptLicense: acceptLicense,
				Ephemeral:     ephemeral,
				Mounts:        mounts,
				Plugins:       validPlugins,
				Password:      resolvedPassword,
			}
			image := spec.Image()
			if err := engine.StartServer(ctx, client, spec); err != nil {
				// Client.Run already wraps stderr verbatim (REQ-F-061) in a
				// clierr.UsageError, so we surface as-is.
				cmd.SilenceUsage = true
				return err
			}

			uri := fmt.Sprintf("neo4j://localhost:%d", boltPort)

			// Persist a matching dbms credential unless opted out or ephemeral
			// (an ephemeral container leaves no on-disk footprint; its credential
			// travels in the env-file blob below). The container is already
			// running, so a failure here must not lose the password:
			//   - the password is going to be printed (or the operator supplied
			//     it): warn, keep going, and the operator has it;
			//   - the password is generated AND hidden (--no-print-password): it
			//     would be unrecoverable, so remove the container we just created
			//     and fail instead of leaving an orphan nobody can log in to.
			if persistCredential {
				if addErr := storeCredentialFn(cfg.DbmsCredentials(), chosenName, resolvedPassword, uri); addErr != nil {
					if noPrintPassword && password == "" {
						rmErr := client.RemoveForce(ctx, chosenName)
						cmd.SilenceUsage = true
						msg := "could not store the credential for %q (%s). The generated password cannot be shown with --%s, so the container was removed. Fix credential storage or rerun without --%s"
						if rmErr != nil {
							msg = "could not store the credential for %q (%s) and could not remove the container (%s). Remove it with `neo4j-cli docker delete %[1]s --rw`"
							return clierr.NewFatalError(msg, chosenName, addErr, rmErr)
						}
						return clierr.NewFatalError(msg, chosenName, addErr, noPrintPasswordFlag, noPrintPasswordFlag)
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to store credentials locally (%s). Save the password now — it cannot be retrieved later.\n", addErr)
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
			// With --env-out-file we write to disk via cfg.Fs() with 0600
			// perms and stay silent on stdout (so callers can pipe). Without
			// --env-out-file we emit the blob to stdout.
			if ephemeral {
				blob := renderEnvFile(chosenName, image, uri, resolvedPassword)
				if envOutFile != "" {
					if err := writeEnvFile(cfg.Fs(), envOutFile, blob); err != nil {
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
			// When --no-print-password is set, the password is omitted from
			// both the map and the fields slice so every format (JSON, table,
			// TOON) suppresses it at a single point.
			row := map[string]any{
				"name":      chosenName,
				"edition":   edition,
				"version":   version,
				"bolt_port": boltPort,
				"http_port": httpPort,
				"uri":       uri,
				"username":  "neo4j",
			}
			fields := []string{"name", "edition", "version", "bolt_port", "http_port", "uri", "username"}
			if !noPrintPassword {
				row["password"] = resolvedPassword
				fields = append(fields, "password")
			}
			if len(validPlugins) > 0 {
				row["plugins"] = pluginsForOutput(validPlugins)
				fields = append(fields, "plugins")
			}
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
	cmd.Flags().BoolVar(&noPrintPassword, noPrintPasswordFlag, false, "Omit the generated password from stdout output. The stored credential still connects via --credential <name> (the password is not CLI-readable).")
	cmd.Flags().BoolVar(&ephemeral, ephemeralFlag, false, "Run with `docker run --rm`; skip credential persistence and emit a .env blob consumable by `query --env`.")
	cmd.Flags().StringVar(&envOutFile, envOutFileFlag, "", "When --ephemeral, write the .env blob to this path (mode 0600) instead of stdout. Writes via a temp file in the same directory and atomically renames; a pre-existing symlink at the path is replaced by a regular file.")
	cmd.Flags().StringVar(&dataDir, dataDirFlag, "", "Host directory to bind-mount at /data inside the container. Empty = no mount (data lives in the container layer and is lost on delete). Path supports `~` and environment-variable expansion; resolved to an absolute path; created at mode 0o755 if missing. Incompatible with --ephemeral.")
	cmd.Flags().StringVar(&logsDir, logsDirFlag, "", "Host directory to bind-mount at /logs inside the container. Empty = no mount. Same expansion + mkdir rules as --data-dir. Incompatible with --ephemeral.")
	cmd.Flags().StringVar(&importDir, importDirFlag, "", "Host directory to bind-mount at /import inside the container (used by Neo4j's LOAD CSV). Empty = no mount. Same expansion + mkdir rules as --data-dir. Incompatible with --ephemeral.")
	cmd.Flags().StringSliceVar(&plugins, pluginFlag, nil, "Neo4j plugin to install in the container via NEO4J_PLUGINS, e.g. apoc or graph-data-science. Repeat the flag or comma-separate for several. The image installs it at startup and rejects unknown names (see `docker logs <name>`).")
	flags.RegisterWait(cmd, &wait, "Wait until Bolt is reachable before returning.")

	return cmd
}

// renderEnvFile builds the .env blob consumed by `neo4j-cli query --env <path>`
// (REQ-F-017). The variable names mirror internal/commands/query/connect.go so the
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

// resolveHostDir expands a `--data-dir` / `--logs-dir` / `--import-dir` flag
// value into a docker-ready absolute path: it runs expandHostPath, ensures
// the resolved directory exists (creating at mode 0o755 if missing), and
// narrates a single `info: created host directory <path>` line to stderr
// when the directory was created. Routing through the supplied afero.Fs keeps
// unit tests hermetic; production passes cfg.Fs() which is backed by the
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
