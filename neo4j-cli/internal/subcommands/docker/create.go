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

// maxNameSuffix caps the auto-suffix walk for name-collision resolution
// (REQ-F-014). The contract is `<name>-1` … `<name>-99`; exceeding it is
// almost certainly operator error (stale containers piling up) and we
// surface that explicitly rather than spinning forever.
const maxNameSuffix = 99

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

// newCreateCmd builds the `neo4j-cli docker create` leaf. The leaf performs
// the port-conflict pre-flight (REQ-F-013) and the name-collision auto-suffix
// (REQ-F-014) before touching docker so a clash never leaves a half-created
// container behind. --wait, --ephemeral, and --env-file land in later tasks.
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
		envFile           string
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
		envFileFlag           = "env-file"
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
			"--env-file <path>, written to that path (mode 0600) while stdout stays silent so it can be piped into " +
			"`neo4j-cli query --env <path>`.",
		Example: `# Create an enterprise container with auto-generated password and store a dbms credential
neo4j-cli docker create --name dev --rw

# Create a community container on a non-default bolt port; emit JSON for scripting
neo4j-cli docker create --name local --edition community --bolt-port 7688 --http-port 7475 --rw --format json

# Create an enterprise container and block until Bolt is reachable before returning
neo4j-cli docker create --name dev --wait --rw

# Create an ephemeral container and emit an env-file blob to stdout for piping into another tool
neo4j-cli docker create --name tmp --ephemeral --rw

# Create an ephemeral container and write the env-file to a path that 'query --env' can consume
neo4j-cli docker create --name tmp --ephemeral --env-file /tmp/n.env --rw

# Create an enterprise container with the commercial license accepted and a custom password (no credential stored)
neo4j-cli docker create --name licensed --edition enterprise --accept-license --password mysecret --no-store-credential --rw`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --edition. Cobra has no built-in enum validator that
			// surfaces a clierr.UsageError; we do the check manually so the
			// error rendering matches the rest of the docker subtree.
			if edition != "community" && edition != "enterprise" {
				return clierr.NewUsageError(`invalid argument %q for "--%s" flag: must be one of "community" or "enterprise"`, edition, editionFlag)
			}

			// --env-file / --ephemeral compatibility (REQ-F-017). --env-file
			// is a child of --ephemeral (it only changes WHERE the env blob
			// goes); rejecting it standalone keeps the contract honest.
			// --no-store-credential + --ephemeral is redundant: ephemeral
			// already skips persistence — error out so the operator notices.
			if envFile != "" && !ephemeral {
				return clierr.NewUsageError("--%s requires --%s", envFileFlag, ephemeralFlag)
			}
			if ephemeral && noStoreCredential {
				return clierr.NewUsageError("--%s is incompatible with --%s (ephemeral already skips credential persistence)", noStoreCredentialFlag, ephemeralFlag)
			}

			// Port-conflict pre-flight (REQ-F-013). Run BEFORE any docker
			// side effect so a port clash never leaves a half-created
			// container behind. Equal-ports check fires first so we don't
			// confusingly succeed on the first Listen and then fail on the
			// second with the same port number in both error messages.
			if boltPort == httpPort {
				return clierr.NewUsageError("--%s and --%s must be different (got %d for both)", boltPortFlag, httpPortFlag, boltPort)
			}
			if err := checkPortFree(boltPort, boltPortFlag); err != nil {
				return err
			}
			if err := checkPortFree(httpPort, httpPortFlag); err != nil {
				return err
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

			// Resolve image: community → neo4j:<version>; enterprise →
			// neo4j:<version>-enterprise (REQ-F-011).
			image := "neo4j:" + version
			if edition == "enterprise" {
				image = "neo4j:" + version + "-enterprise"
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
			// With --env-file we write to disk via cfg.Aura.Fs() with 0600
			// perms and stay silent on stdout (so callers can pipe). Without
			// --env-file we emit the blob to stdout.
			if ephemeral {
				blob := renderEnvFile(chosenName, image, uri, resolvedPassword)
				if envFile != "" {
					if err := writeEnvFile(cfg.Aura.Fs(), envFile, blob); err != nil {
						cmd.SilenceUsage = true
						return err
					}
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: wrote credentials to %s\n", envFile)
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
	cmd.Flags().IntVar(&boltPort, boltPortFlag, 7687, "Host port to publish for Bolt (container 7687).")
	cmd.Flags().IntVar(&httpPort, httpPortFlag, 7474, "Host port to publish for the HTTP browser (container 7474).")
	cmd.Flags().StringVar(&password, passwordFlag, "", "Neo4j password. When empty, a 16-byte base64 URL-safe password is generated.")
	cmd.Flags().BoolVar(&noStoreCredential, noStoreCredentialFlag, false, "Skip persisting a dbms credential for this container.")
	cmd.Flags().BoolVar(&ephemeral, ephemeralFlag, false, "Run with `docker run --rm`; skip credential persistence and emit a .env blob consumable by `query --env`.")
	cmd.Flags().StringVar(&envFile, envFileFlag, "", "When --ephemeral, write the .env blob to this path (mode 0600) instead of stdout.")
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
// using O_WRONLY|O_CREATE|O_TRUNC with mode 0600 (REQ-F-017 / REQ-NF-004) so
// the file is only readable by the calling user. Routing through the afero
// seam keeps unit tests hermetic — production hits the real OS fs, tests hit
// the memfs from testfs.GetTestFs.
func writeEnvFile(fs afero.Fs, path, contents string) error {
	f, err := fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("docker create: write env-file %s: %w", path, err)
	}
	if _, werr := f.WriteString(contents); werr != nil {
		_ = f.Close()
		return fmt.Errorf("docker create: write env-file %s: %w", path, werr)
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("docker create: write env-file %s: %w", path, cerr)
	}
	// Defense-in-depth (REQ-NF-004): OpenFile only applies the mode arg on
	// create; a pre-existing file at this path keeps its prior (potentially
	// permissive) mode. Chmod unconditionally so the on-disk file ends up
	// at 0o600 regardless of who owned it before.
	if cerr := fs.Chmod(path, 0o600); cerr != nil {
		return fmt.Errorf("docker create: chmod env-file %s: %w", path, cerr)
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

// checkPortFree binds and immediately releases a TCP listener on the given
// host port via the listenerFactory seam. On bind failure it returns a
// usage error naming both the port and the flag the operator can override
// (REQ-F-013). On success the listener is closed before returning so the
// real `docker run` call can claim the port a moment later.
func checkPortFree(port int, flagName string) error {
	ln, err := listenerFactory(port)
	if err != nil {
		return clierr.NewUsageError("port %d is in use on the host. Pass --%s <other> to pick a free port.", port, flagName)
	}
	_ = ln.Close()
	return nil
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
