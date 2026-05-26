// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// Lowercase wire status values; the table renderer uppercases for display.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusSkip = "skip"
	StatusInfo = "info"
)

// Stable check identifiers used as the `name` field in JSON / toon output
// and as dependency keys for skip semantics. Names are deliberately neutral
// — no "secret"/"salt"/"JWT"/"key" wording.
const (
	CheckInstallPresent   = "install_present"
	CheckStandardProbe    = "standard_probe"
	CheckInfoApp          = "info_app"
	CheckDataDir          = "data_dir"
	CheckAuthDataReadable = "auth_data_readable"
	CheckAuthenticated    = "authenticated_probe"
)

// User-visible labels. The `Auth data readable` label intentionally says
// nothing about secrets / keys / JWTs / salts.
const (
	LabelInstallPresent   = "Install present"
	LabelStandardProbe    = "Standard port probe"
	LabelInfoApp          = "Desktop info"
	LabelDataDir          = "Data directory"
	LabelAuthDataReadable = "Auth data readable"
	LabelAuthenticated    = "Authenticated probe"
)

// CheckResult is one row in the doctor report. `Version`, `AppPath`, and
// `DataPath` are populated only on the `info_app` row when /info/app
// succeeds; `omitempty` keeps the JSON / toon shape minimal elsewhere.
type CheckResult struct {
	Name     string `json:"name"`
	Label    string `json:"-"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Version  string `json:"version,omitempty"`
	AppPath  string `json:"app_path,omitempty"`
	DataPath string `json:"data_path,omitempty"`
}

// DoctorSummary is the single-shot verdict appended to the report. `Port`
// is nil-able to distinguish "reachable on port X" from "no probe succeeded".
type DoctorSummary struct {
	Reachable         bool   `json:"reachable"`
	Port              *int   `json:"port,omitempty"`
	StandardPortRange bool   `json:"standard_port_range"`
	NextStep          string `json:"next_step,omitempty"`
}

// DoctorReport is the full `--format json` / `--format toon` envelope.
type DoctorReport struct {
	Checks  []CheckResult `json:"checks"`
	Summary DoctorSummary `json:"summary"`
}

// newDoctorCmd returns the read-only `desktop doctor` leaf. The leaf always
// exits 0; gating consumers parse `summary.reachable` instead.
func newDoctorCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose a local Neo4j Desktop 2 install end-to-end",
		Long: "Run an ordered sequence of six health checks against the local Neo4j Desktop 2 install: " +
			"(1) install present, (2) standard port probe, (3) Desktop info (version, app path, data path), (4) data directory present, (5) auth data readable, (6) authenticated probe. " +
			"Each check produces a `{name, status, detail, hint?}` record; when a check FAILs, dependent later checks render as `skip` with a `(depends on …)` detail. " +
			"The Desktop-info check is purely diagnostic: an unavailable `/info/app` endpoint (older Desktop) renders as INFO and never blocks subsequent checks. " +
			"`--format json` (or `toon`) emits a single `{checks: [...], summary: {reachable, port?, standard_port_range, next_step?}}` document for agent consumption. " +
			"Default-TTY table format renders aligned name / status / detail columns and a trailing one-line summary. " +
			"Inherits `--port <n>` from the `desktop` parent: when set, the standard-port probe tries only that port instead of scanning 44222..44232. " +
			"The leaf is read-only and always exits 0 — parse `summary.reachable` to gate downstream actions.",
		Example: `# Run all six checks against the local Desktop install (default table output)
neo4j-cli desktop doctor

# Pin the probe to a specific port instead of scanning 44222..44232
neo4j-cli desktop doctor --port 44222

# Emit a structured JSON report suitable for agent consumption
neo4j-cli desktop doctor --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			port, _ := cmd.Flags().GetInt(portFlag)
			report := runChecks(ctx, cfg, port)
			return renderReport(cmd, cfg, report)
		},
	}
}
