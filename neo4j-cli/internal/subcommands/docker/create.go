// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// clientFactory is the injectable seam for the dockerClient used by leaves.
// Production wires the exec-backed client (client.go newClient); tests swap
// in a fakeDockerClient (helpers_test.go) without touching the leaf code.
var clientFactory = newClient

// randSource is the injectable seam for crypto-grade random bytes used to
// generate a default Neo4j password (REQ-F-015). Tests swap in a deterministic
// reader so the rendered password is assertable.
var randSource io.Reader = rand.Reader

// generatedPasswordBytes is the byte length consumed from randSource before
// base64-URL-safe encoding without padding. 16 bytes → 22 base64 characters,
// which is well above the entropy floor for a local Bolt password.
const generatedPasswordBytes = 16

// newCreateCmd builds the `neo4j-cli docker create` leaf — the minimum-viable
// happy path (no port-conflict pre-flight, no name-collision auto-suffix, no
// --wait, no --ephemeral, no --env-file; those land in tasks 4–7).
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
	)

	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a local Neo4j Docker container",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a local Neo4j Docker container via `docker run -d` and (unless --no-store-credential) " +
			"store a matching dbms credential so `neo4j-cli query --credential <name>` can connect immediately. " +
			"The container carries `org.neo4j.cli.managed=true` plus a small set of metadata labels — " +
			"Docker itself is the source of truth, no separate state file is maintained. " +
			"When --password is omitted, a 16-byte base64 URL-safe password is generated and surfaced in the output.",
		Example: `# Create an enterprise container with auto-generated password and store a dbms credential
neo4j-cli docker create --name dev --rw

# Create a community container on a non-default bolt port; emit JSON for scripting
neo4j-cli docker create --name local --edition community --bolt-port 7688 --http-port 7475 --rw --format json

# Create an enterprise container with the commercial license accepted and a custom password (no credential stored)
neo4j-cli docker create --name licensed --edition enterprise --accept-license --password mysecret --no-store-credential --rw`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --edition. Cobra has no built-in enum validator that
			// surfaces a clierr.UsageError; we do the check manually so the
			// error rendering matches the rest of the docker subtree.
			if edition != "community" && edition != "enterprise" {
				return clierr.NewUsageError(`invalid argument %q for "--%s" flag: must be one of "community" or "enterprise"`, edition, editionFlag)
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
			// shape; keep ports → env → labels → image (last).
			argv := []string{"--name", name}
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
			argv = append(argv, "--label", LabelManaged+"=true")
			argv = append(argv, "--label", LabelEdition+"="+edition)
			argv = append(argv, "--label", LabelVersion+"="+version)
			argv = append(argv, "--label", LabelBoltPort+"="+strconv.Itoa(boltPort))
			argv = append(argv, "--label", LabelHTTPPort+"="+strconv.Itoa(httpPort))
			argv = append(argv, "--label", LabelEphemeral+"=false")
			argv = append(argv, image)

			client := clientFactory()
			ctx := cmd.Context()
			if _, err := client.Run(ctx, argv); err != nil {
				// dockerClient.Run already wraps stderr verbatim (REQ-F-061)
				// in a clierr.UsageError, so we surface as-is.
				cmd.SilenceUsage = true
				return err
			}

			uri := fmt.Sprintf("neo4j://localhost:%d", boltPort)

			// Persist a matching dbms credential unless explicitly opted out.
			// Database name defaults to "neo4j" for local containers per
			// existing credential conventions (mirrors credential dbms add).
			if !noStoreCredential {
				if cfg.Credentials == nil || cfg.Credentials.Dbms == nil {
					return clierr.NewUsageError("credential storage is not available; use --%s to skip storing credentials locally", noStoreCredentialFlag)
				}
				if err := cfg.Credentials.Dbms.Add(name, "neo4j", resolvedPassword, "neo4j", uri); err != nil {
					return err
				}
			}

			// Render the result. Field order mirrors what an operator wants
			// at a glance: identity, image identity, ports, connection details.
			row := map[string]any{
				"name":      name,
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

	return cmd
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
