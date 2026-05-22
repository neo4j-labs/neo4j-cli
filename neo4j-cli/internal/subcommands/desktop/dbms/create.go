// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
	"golang.org/x/term"
)

var dbmsCreateFields = []string{"id", "name", "version", "status", "connectionUri"}

// 1s cadence matches `docker.WaitForBolt`; 30s ceiling matches `docker create --wait`.
const (
	createPollInterval = 1 * time.Second
	createPollTimeout  = 30 * time.Second
)

var createPollSleepFn = func(d time.Duration) { time.Sleep(d) }

// SetCreatePollSleepFnForTest overrides the poll-loop sleep for tests.
func SetCreatePollSleepFnForTest(fn func(time.Duration)) func() {
	prev := createPollSleepFn
	createPollSleepFn = fn
	return func() { createPollSleepFn = prev }
}

var createNowFn = func() time.Time { return time.Now() }

// SetCreateNowFnForTest overrides the poll-loop clock for tests.
func SetCreateNowFnForTest(fn func() time.Time) func() {
	prev := createNowFn
	createNowFn = fn
	return func() { createNowFn = prev }
}

var createStdinIsTTYFn = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// SetCreateStdinIsTTYFnForTest overrides the TTY detector for tests.
func SetCreateStdinIsTTYFnForTest(fn func() bool) func() {
	prev := createStdinIsTTYFn
	createStdinIsTTYFn = fn
	return func() { createStdinIsTTYFn = prev }
}

var createPasswordReaderFn = func() (string, error) {
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

// SetCreatePasswordReaderFnForTest overrides the password reader for tests.
func SetCreatePasswordReaderFnForTest(fn func() (string, error)) func() {
	prev := createPasswordReaderFn
	createPasswordReaderFn = fn
	return func() { createPasswordReaderFn = prev }
}

// promptCreatePassword reads the new DBMS password from the controlling terminal
// with no echo so the value never lands in argv (visible via `ps aux` /
// `/proc/<pid>/cmdline` to other local users). Non-TTY returns a usage error.
func promptCreatePassword(cmd *cobra.Command) (string, error) {
	if !createStdinIsTTYFn() {
		return "", clierr.NewUsageError(
			"--password is required when stdin is not a terminal; pass --password <value> or run interactively")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := createPasswordReaderFn()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if pw == "" {
		return "", clierr.NewUsageError("empty password is not allowed")
	}
	return pw, nil
}

// dbmsCreateResult adapts a `*DbmsInfo` to the output.ResponseData contract.
type dbmsCreateResult struct {
	Item *desktopclient.DbmsInfo
}

func (r dbmsCreateResult) AsArray() []map[string]any {
	if r.Item == nil {
		return nil
	}
	return []map[string]any{
		{
			"id":            r.Item.ID,
			"name":          r.Item.Name,
			"version":       r.Item.Version,
			"status":        r.Item.Status,
			"connectionUri": r.Item.ConnectionURI,
		},
	}
}

// MarshalJSON emits the full DbmsInfo so `--format json` matches `desktop dbms list`.
func (r dbmsCreateResult) MarshalJSON() ([]byte, error) {
	if r.Item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.Item)
}

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name     string
		version  string
		password string
		wait     bool
		force    bool
	)

	const (
		nameFlag     = "name"
		versionFlag  = "version"
		passwordFlag = "password"
		waitFlag     = "wait"
		forceFlag    = "force"
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new DBMS under the local Neo4j Desktop 2 install",
		Long: "Create a new DBMS managed by the local Neo4j Desktop 2 install and start it. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Desktop 2 ships enterprise-only and the create route defaults to enterprise; the CLI does NOT expose an `--edition` flag because there is no choice. " +
			"`--version` is optional: when omitted, the CLI queries Desktop's `GET /dbmss/versions` catalog and auto-picks the highest stable enterprise version (preferring already-cached entries on ties), emitting a stderr breadcrumb naming the picked version + origin. " +
			"Desktop owns the credential lifecycle — the initial password is stored via Desktop's safeStorage and is NOT written to `~/.neo4j/cli/credentials.json`. " +
			"Use `credential dbms add` separately if you want a persisted neo4j-cli profile pointing at this DBMS. " +
			"A pre-flight check refuses to create+start when another DBMS is already running, since Neo4j Desktop 2 runs one DBMS at a time on port 7687; pass `--force` to stop the conflicting DBMS first and then proceed. " +
			"By default the command returns as soon as Desktop's `POST /start` call resolves (the DBMS is created and the start request has been issued, but may still be transitioning). Pass `--wait` to block while the CLI polls every 1s for up to 30s for `status=started`, exiting non-zero if that threshold is exceeded.",
		Example: `# Create a DBMS using the latest stable enterprise version Desktop knows about (returns once start is issued)
neo4j-cli desktop dbms create --name my-dbms --password supersecret --rw

# Create a DBMS pinned to a specific version
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --rw

# Create a DBMS and block until it reports status=started
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --wait --rw

# Stop any other running DBMS first to free port 7687, then create+start this one
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --force --rw

# Create a DBMS and emit the full DbmsInfo as JSON for scripting
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return clierr.NewUsageError("--%s is required", nameFlag)
			}
			if password == "" {
				// Prompt on TTY so the password never lands in argv
				// (visible via `ps aux` / `/proc/<pid>/cmdline`). Non-TTY
				// errors out with a usage hint.
				pw, perr := promptCreatePassword(cmd)
				if perr != nil {
					return perr
				}
				password = pw
			}
			cmd.SilenceUsage = true

			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}
			if version == "" {
				// Pick the highest stable enterprise entry from Desktop's catalog
				// and emit a stderr breadcrumb so the user knows what was chosen.
				versions, verr := client.ListDbmsVersions(ctx)
				if verr != nil {
					return verr
				}
				picked, perr := pickLatestStableEnterprise(versions)
				if perr != nil {
					return perr
				}
				version = picked.Version
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Using Neo4j enterprise %s (%s)\n", picked.Version, picked.Origin)
			}
			if !force {
				// Pre-flight runs BEFORE POST /dbmss so a blocked request
				// leaves no orphan on disk. selfID empty (no DBMS yet).
				if err := assertNoOtherRunning(ctx, client, "", name, "create+start", cmd.ErrOrStderr()); err != nil {
					return err
				}
			} else {
				// Stop conflicting DBMS first (if any) so the subsequent
				// create+start succeeds on port 7687. Runs BEFORE POST /dbmss
				// so a failed stop leaves no orphan on disk.
				if _, err := resolveConflicting(ctx, client, "", cmd.ErrOrStderr()); err != nil {
					return err
				}
			}

			created, err := client.CreateDbms(ctx, desktopclient.CreateDbmsRequest{
				Name:     name,
				Version:  version,
				Password: password,
			})
			if err != nil {
				return err
			}
			if err := client.StartDbms(ctx, created.ID); err != nil {
				return err
			}

			final := created
			if wait {
				started, perr := pollUntilStarted(ctx, client, created.ID)
				if perr != nil {
					return perr
				}
				final = started
			}

			output.PrintBodyMap(cmd, cfg, dbmsCreateResult{Item: final}, dbmsCreateFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name of the new DBMS")
	cmd.Flags().StringVar(&version, versionFlag, "", "Neo4j version (e.g. 2026.04.0 or 5.26.1). When omitted, picks the latest stable enterprise version Desktop knows about.")
	cmd.Flags().StringVar(&password, passwordFlag, "", "Initial password for the `neo4j` user (stored by Desktop's safeStorage; not persisted in neo4j-cli credentials). Prefer the interactive TTY prompt (omit --password) so the value does not land in argv (`ps aux` / Task Manager) or in shell history. Required on non-TTY callers.")
	cmd.Flags().BoolVar(&wait, waitFlag, false, "Block until Desktop reports `status=started` for the new DBMS (polled every 1s, 30s ceiling). Without --wait the command returns as soon as the start request resolves.")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Stop any other running Desktop DBMS first to free port 7687, then proceed. Without --force, the command refuses when another DBMS is running.")

	return cmd
}

func pollUntilStarted(ctx context.Context, client *desktopclient.Client, id string) (*desktopclient.DbmsInfo, error) {
	return pollUntilStatus(ctx, client, id, dbmsStatusStarted)
}

// PickLatestStableEnterpriseForTest exposes pickLatestStableEnterprise to the external test package.
func PickLatestStableEnterpriseForTest(versions []desktopclient.DbmsVersion) (desktopclient.DbmsVersion, error) {
	return pickLatestStableEnterprise(versions)
}

// pickLatestStableEnterprise filters to enterprise + stable (no pre-release suffix)
// and picks the highest semver. Tie-break: prefer `origin == "cached"` to avoid a
// download wait.
//
// `golang.org/x/mod/semver` rejects calendar versions like `2026.04.0` (strict semver
// forbids leading zeros). Normalise leading zeros so both 5.x and YYYY.MM series sort
// correctly.
func pickLatestStableEnterprise(versions []desktopclient.DbmsVersion) (desktopclient.DbmsVersion, error) {
	candidates := make([]desktopclient.DbmsVersion, 0, len(versions))
	for _, v := range versions {
		if v.Edition != "enterprise" {
			continue
		}
		tagged := normaliseSemver(v.Version)
		if !semver.IsValid(tagged) {
			continue
		}
		if semver.Prerelease(tagged) != "" {
			continue
		}
		candidates = append(candidates, v)
	}
	if len(candidates) == 0 {
		return desktopclient.DbmsVersion{}, clierr.NewFatalError(
			"Desktop has no stable enterprise versions to choose from. " +
				"Pass --version <vX> explicitly. See Desktop's UI or 'neo4j-cli desktop list' for the catalog.")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		cmp := semver.Compare(normaliseSemver(candidates[i].Version), normaliseSemver(candidates[j].Version))
		if cmp != 0 {
			return cmp > 0
		}
		// Prefer cached over online to avoid a dist.neo4j.org download wait.
		return candidates[i].Origin == "cached" && candidates[j].Origin != "cached"
	})
	return candidates[0], nil
}

// normaliseSemver converts a Neo4j version (`5.26.1` or `2026.04.0`) into a
// `golang.org/x/mod/semver`-compatible tagged form: prepend `v` and strip leading
// zeros on each dotted numeric component (strict semver forbids leading zeros, and
// Desktop's calendar versions use them). Pre-release / build metadata preserved.
func normaliseSemver(v string) string {
	suffix := ""
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		suffix = v[idx:]
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	for i, p := range parts {
		parts[i] = stripLeadingZeros(p)
	}
	return "v" + strings.Join(parts, ".") + suffix
}

// stripLeadingZeros drops leading zeros from a semver component (`"04"` → `"4"`,
// `"000"` → `"0"`).
func stripLeadingZeros(p string) string {
	i := 0
	for i < len(p)-1 && p[i] == '0' {
		i++
	}
	return p[i:]
}
