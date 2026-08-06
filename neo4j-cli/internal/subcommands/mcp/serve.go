// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"os"
	"strconv"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

func newServeCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP stdio server",
		Long: "Start the MCP stdio server, exposing neo4j-cli to MCP clients such as Claude Desktop. " +
			"The server reads JSON-RPC frames from stdin and writes responses to stdout. " +
			"Once the server is running, an MCP client connects to discover and interact with " +
			"Neo4j databases through the five neo4j_cli_* tools.",
		Example: `# Start the MCP server with read-only access (default)
neo4j-cli mcp serve

# Start the MCP server with write access allowed
neo4j-cli mcp serve --rw

# Start the MCP server with JSON output format for executed commands
neo4j-cli mcp serve --format json

# Start the MCP server with Aura provisioning allowed
neo4j-cli mcp serve --allow-aura`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveRun(cfg, cmd)
		},
	}

	// serve is NOT annotated write:"true" (REQ-NF-006): stdout is never a TTY
	// under MCP, so EnforceWriteGate would demand --rw merely to start the
	// server, destroying the read-only default.
	addServeFlags(cmd)

	return cmd
}

// probeKeyringFn is the injectable seam for credential store availability at
// server start. Production wires credentials.ProbeKeyringAvailability; tests
// swap in a stub so the test does not depend on the real OS keyring.
var probeKeyringFn = credentials.ProbeKeyringAvailability

// envManifestMarker is the env key that unlocks the gate env-var fallback.
// Set unconditionally in the .mcpb manifest's Env block so Claude Desktop
// settings-UI toggles work. Without it, only CLI flags grant capability.
const envManifestMarker = "NEO4J_CLI_MCP_MANIFEST"

// Gate env-var names. Read in serveRun when the manifest marker is present.
const envMCPAllowWrites = "NEO4J_CLI_MCP_ALLOW_WRITES"
const envMCPAllowAura = "NEO4J_CLI_MCP_ALLOW_AURA"
const envMCPAllowCredentialWrite = "NEO4J_CLI_MCP_ALLOW_CREDENTIAL_WRITE"

// envBool reads a boolean env var, returning false if unset or unparseable.
func envBool(key string) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return false
}

// checkCredentialStore returns an actionable error when the credential storage
// mode is "keyring" but the OS keyring daemon is unreachable. A locked or
// unavailable keyring would hang on the first credential read during a tool
// call; checking at server start catches it early.
func checkCredentialStore(cfg *clicfg.Config) error {
	if cfg.Global.CredentialStorage() != credentials.StorageModeKeyring {
		return nil
	}
	if err := probeKeyringFn(); err != nil {
		return clierr.NewFatalError(
			"OS keyring is locked or unavailable (%v).\n"+
				"To store credentials in plaintext instead, run:\n"+
				"  neo4j-cli config set credential-storage insecure --rw\n"+
				"\n...or unlock your keyring.",
			err,
		)
	}
	return nil
}

// addServeFlags registers the serve command's flags. The gate flags were previously
// persistent flags on the `mcp` parent (task-005 parked them there because `serve`
// did not exist yet).
func addServeFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("rw", false,
		"Allow write operations through neo4j_cli_run_write")
	cmd.Flags().Bool(AllowAuraFlag, false,
		"Let MCP clients provision, modify and delete Aura resources, which costs money")
	cmd.Flags().Bool(AllowCredentialWriteFlag, false,
		"Let MCP clients add, remove and select stored credentials in the OS keyring")
	cmd.Flags().String("format", "toon",
		"Default output format for executed commands (json, toon, table)")
	cmd.Flags().Int("max-output-chars", DefaultMaxOutputChars,
		"Maximum characters of output to return in tool results")
}

// resolveGates reads gate flags and applies the manifest-gated env fallback.
//
// The gate flags (--rw, --allow-aura, --allow-credential-write) are the primary
// controls. Each also has an env fallback guarded by manifestMarker: the env
// var is only consulted when the flag is false AND the marker is present.
func resolveGates(writeFlag, auraFlag, credFlag string, manifestMarker bool) Gates {
	writeAllowed, _ := strconv.ParseBool(writeFlag)
	if !writeAllowed && manifestMarker {
		writeAllowed = envBool(envMCPAllowWrites)
	}
	allowAura, _ := strconv.ParseBool(auraFlag)
	if !allowAura && manifestMarker {
		allowAura = envBool(envMCPAllowAura)
	}
	allowCredWrite, _ := strconv.ParseBool(credFlag)
	if !allowCredWrite && manifestMarker {
		allowCredWrite = envBool(envMCPAllowCredentialWrite)
	}
	return Gates{
		AllowAura:            allowAura,
		AllowCredentialWrite: allowCredWrite,
		WriteAllowed:         writeAllowed,
	}
}

// serveRun is the implementation of the serve command.
//
// The gate flags (--rw, --allow-aura, --allow-credential-write) are the primary
// controls. Each has a manifest-gated env fallback (see resolveGates).
//
// KNOWN LIMITATION: the env fallback is gated behind NEO4J_CLI_MCP_MANIFEST=1,
// which the .mcpb manifest's Env block sets unconditionally so Claude Desktop's
// settings-UI toggles keep working. This stops ACCIDENTAL leakage: a stale gate
// var left in a shell rc or inherited environment is ignored, because a hand-run
// `mcp serve` carries no marker. It does NOT stop a deliberate attacker running
// as the same OS user (a dotfile, an npm postinstall, a poisoned shell rc), who
// can set the marker alongside the gate vars. Only flag-only gating would close
// that, and it would break the Desktop UI. The operator's start-up intent is not
// authoritative while both marker and gate var are set.
//
// Layer 3 (flags.EnforceWriteGate) limits this for write-ANNOTATED commands:
// it still demands --rw in the model's own execArgs. It does NOT cover a leaf
// that carries no annotation because its write-ness is resolved at runtime —
// `aura api` is the live example, which is why gated policies are refused by
// neo4j_cli_run and reachable only through neo4j_cli_run_write. Do not restate
// the Layer 3 guarantee without that carve-out.
//
// Audit these vars if the read-only posture matters.
func serveRun(cfg *clicfg.Config, cmd *cobra.Command) error {
	manifestMarker := envBool(envManifestMarker)
	gates := resolveGates(
		cmd.Flag("rw").Value.String(),
		cmd.Flag(AllowAuraFlag).Value.String(),
		cmd.Flag(AllowCredentialWriteFlag).Value.String(),
		manifestMarker,
	)
	defaultFormat := cmd.Flag("format").Value.String()
	maxOutputChars, _ := strconv.Atoi(cmd.Flag("max-output-chars").Value.String())

	if err := checkCredentialStore(cfg); err != nil {
		return err
	}

	exec, err := NewExecutor(cfg, storedRootFactory)
	if err != nil {
		return clierr.NewFatalError("cannot start MCP server: %s", err.Error())
	}

	server, err := NewServer(cfg, exec, gates, defaultFormat, maxOutputChars)
	if err != nil {
		return clierr.NewFatalError("cannot create MCP server: %s", err.Error())
	}

	in, out, restore, err := ClaimStdio()
	if err != nil {
		return clierr.NewFatalError("cannot claim stdio for MCP server: %s", err.Error())
	}
	// ClaimStdio points the os.Stdout variable at stderr, which would make the
	// write gate's TTY probe read stderr and treat a shell-launched server as
	// interactive — skipping the --rw requirement. An MCP caller is never
	// interactive, so pin the probe off.
	flags.ForceNonInteractive()
	// restore is safe to call more than once; defer so even a panic or
	// recover leaves the process usable after the serve command exits.
	defer restore()

	// Use IOTransport with the claimed files, NOT StdioTransport, which
	// reads the os.Stdin/os.Stdout variables at Connect time — those have
	// been swapped to /dev/null and stderr by ClaimStdio.
	transport := &mcpsdk.IOTransport{
		Reader: in,
		Writer: out,
	}

	if err := server.Run(cmd.Context(), transport); err != nil {
		return clierr.NewFatalError("MCP server exited: %s", err.Error())
	}
	return nil
}
