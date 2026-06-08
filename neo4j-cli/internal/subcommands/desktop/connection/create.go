// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// connectionCreateFields is the default column order for table / toon output;
// JSON output emits the full Connection wire payload.
var connectionCreateFields = []string{"id", "name", "connection_uri"}

// newDesktopClientFn is the test seam for desktop client construction.
var newDesktopClientFn = newDesktopClient

func newDesktopClient(ctx context.Context, fs afero.Fs, port int) (*desktopclient.Client, error) {
	// Discover runs first so its origin can be threaded into ResolveDataDir.
	probe, err := desktopclient.Discover(ctx, port)
	if err != nil {
		if errors.Is(err, desktopclient.ErrNoDesktop) {
			return nil, desktopclient.UnreachableError()
		}
		return nil, clierr.NewFatalError("desktop: probe failed: %s", err.Error())
	}
	dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return nil, clierr.NewFatalError("desktop: could not resolve relate data dir: %s", err.Error())
	}
	salt, err := desktopclient.LoadSalt(fs, dataDir)
	if err != nil {
		// Missing/unreadable salt = Desktop has not finished first-run auth
		// setup. Route to the same canonical hint as a probe miss.
		return nil, desktopclient.UnreachableError()
	}
	return desktopclient.NewClient(probe, salt)
}

// SetNewDesktopClientFnForTest overrides the client constructor for tests and returns a restore func.
func SetNewDesktopClientFnForTest(fn func(context.Context, afero.Fs, int) (*desktopclient.Client, error)) func() {
	prev := newDesktopClientFn
	newDesktopClientFn = fn
	return func() { newDesktopClientFn = prev }
}

// stdinIsTTYFn is the test seam for stdin TTY detection.
var stdinIsTTYFn = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// SetStdinIsTTYFnForTest overrides the TTY detector for tests and returns a restore func.
func SetStdinIsTTYFnForTest(fn func() bool) func() {
	prev := stdinIsTTYFn
	stdinIsTTYFn = fn
	return func() { stdinIsTTYFn = prev }
}

// passwordReaderFn is the test seam for the no-echo TTY password prompt.
var passwordReaderFn = func() (string, error) {
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

// SetPasswordReaderFnForTest overrides the password reader for tests and returns a restore func.
func SetPasswordReaderFnForTest(fn func() (string, error)) func() {
	prev := passwordReaderFn
	passwordReaderFn = fn
	return func() { passwordReaderFn = prev }
}

// promptPassword reads a password from the TTY with no echo, or returns a
// usage error when stdin is not a TTY. Prompt is written to stderr so a
// `--format json` pipeline's stdout stays clean.
func promptPassword(cmd *cobra.Command) (string, error) {
	if !stdinIsTTYFn() {
		return "", clierr.NewUsageError(
			"--password is required when stdin is not a terminal; pass --password <value> or run interactively")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := passwordReaderFn()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return pw, nil
}

// connectionResult adapts a single `*Connection` to `output.ResponseData`.
type connectionResult struct {
	Item *desktopclient.Connection
}

func (r connectionResult) AsArray() []map[string]any {
	if r.Item == nil {
		return nil
	}
	return []map[string]any{
		{
			"id":             r.Item.ID,
			"name":           r.Item.Name,
			"connection_uri": r.Item.ConnectionURI,
		},
	}
}

func (r connectionResult) MarshalJSON() ([]byte, error) {
	if r.Item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.Item.ToOutput())
}

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name        string
		uri         string
		username    string
		password    string
		description string
	)

	const (
		nameFlag        = "name"
		uriFlag         = "uri"
		usernameFlag    = "username"
		passwordFlag    = "password"
		descriptionFlag = "description"
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Register a saved remote DB connection with Neo4j Desktop 2",
		Long: "Register a saved remote DB connection profile with the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Saved connections appear under `Remote connections` in `neo4j-cli desktop list` and can be selected at query time via `--credential desktop-connection:<uuid>`. " +
			"The password is stored by Desktop via its safeStorage mechanism on the `connection:<id>` key and is NOT written to `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle. " +
			"`--password` is mandatory at runtime: pass it as a flag, or omit it on an interactive terminal to be prompted with no echo; non-TTY callers without `--password` fail with a usage error.",
		Example: `# Create a saved connection against an Aura instance, passing the password as a flag
neo4j-cli desktop connection create --name aura-prod --uri neo4j+s://abc123.databases.neo4j.io --username neo4j --password supersecret --rw

# Create a saved connection and be prompted for the password interactively (TTY only)
neo4j-cli desktop connection create --name local-bolt --uri neo4j://localhost:7687 --username neo4j --rw

# Create a saved connection with a description, emitting the full Connection as JSON
neo4j-cli desktop connection create --name aura-dev --uri neo4j+s://xyz789.databases.neo4j.io --username neo4j --password supersecret --description "dev tier" --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt("port")

			// Password is optional on the flag surface so we can prompt on a
			// TTY, but mandatory at runtime. Prompt before building the client
			// so non-TTY callers fail fast without touching Desktop.
			if password == "" {
				pw, err := promptPassword(cmd)
				if err != nil {
					return err
				}
				password = pw
			}

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}

			args2 := desktopclient.ConnectionCreateArgs{
				Name:          name,
				ConnectionURI: uri,
				Username:      username,
				Password:      password,
			}
			// Only forward --description when the user set it; empty-default
			// would be indistinguishable from "field not provided" on the wire.
			if cmd.Flag(descriptionFlag).Changed {
				args2.Description = description
			}

			clievents.RegisterSecretValue(password)

			created, err := client.CreateConnection(ctx, args2)
			if err != nil {
				return err
			}

			output.PrintBodyMap(cmd, cfg, connectionResult{Item: created}, connectionCreateFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Human-readable name for the saved connection")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&uri, uriFlag, "", "(required) Bolt URI of the remote DB (e.g. neo4j+s://abc.databases.neo4j.io)")
	cmd.MarkFlagRequired(uriFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&username, usernameFlag, "", "(required) Username used to authenticate against the remote DB")
	cmd.MarkFlagRequired(usernameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&password, passwordFlag, "", "Password for the remote DB. Prefer the interactive TTY prompt (omit --password) so the value does not land in argv (`ps aux` / Task Manager) or in shell history. Required on non-TTY callers")
	cmd.Flags().StringVar(&description, descriptionFlag, "", "Optional human-readable description for the saved connection")

	return cmd
}
