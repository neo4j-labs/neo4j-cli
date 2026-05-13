// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/neo4j/cli/common/agent"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// detectAgent is the test seam for agent-harness detection. Production calls
// agent.Detect (env-var driven); tests override to drive the gate matrix
// without mutating real process state.
var detectAgent = agent.Detect

// stdoutIsTerminal is the test seam for TTY detection on stdout. Production
// calls term.IsTerminal on os.Stdout's file descriptor; tests override to
// simulate interactive vs. piped contexts. Mirrors stdinIsTTY in
// neo4j-cli/query/run.go.
var stdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// RegisterOutputFlag adds a persistent --format/-f flag to cmd and installs a
// PersistentPreRunE hook that validates the value and binds it to cfg.Global.
func RegisterOutputFlag(cmd *cobra.Command, cfg *clicfg.Config) {
	cmd.PersistentFlags().StringP(
		"format",
		"f",
		"",
		fmt.Sprintf("Format to print console output in, from a choice of [%s]. (agents: prefer toon)", strings.Join(clicfg.ValidFormatValues[:], ", ")),
	)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return silenceUsageOnError(cmd, BindFormatFromFlag(cmd, cfg))
	}
}

// silenceUsageOnError sets cmd.SilenceUsage=true when err is non-nil so
// cobra prints the focused error without appending the full --help block.
// Returns err unchanged so call sites can `return silenceUsageOnError(cmd, x())`.
func silenceUsageOnError(cmd *cobra.Command, err error) error {
	if err != nil {
		cmd.SilenceUsage = true
	}
	return err
}

// RegisterRwFlag adds a persistent --rw flag to cmd.
func RegisterRwFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(
		"rw",
		false,
		"Allow write operations. Auto-applied in interactive terminals; required when running under an agent harness or non-interactive script.",
	)
}

// BindFormatFromFlag validates the --format flag and binds it to cfg.Global.
func BindFormatFromFlag(cmd *cobra.Command, cfg *clicfg.Config) error {
	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag != nil && formatFlag.Value.String() != "" {
		formatValue := formatFlag.Value.String()
		valid := false
		for _, v := range clicfg.ValidFormatValues {
			if v == formatValue {
				valid = true
				break
			}
		}
		if !valid {
			return clierr.NewUsageError("invalid format value specified: %s", formatValue)
		}
	}

	cfg.Global.BindFormat(cmd.Flags().Lookup("format"))

	return nil
}

// EnforceWriteGate rejects write-annotated commands unless --rw is true, an
// interactive terminal is detected on stdout, or the caller is running under
// a known agent harness (in which case --rw must be explicit). Precedence:
//  1. --rw set         → allow
//  2. agent detected   → require --rw (gate fires)
//  3. stdout is a TTY  → allow (interactive human)
//  4. otherwise        → require --rw (CI, piped script, nohup, …)
func EnforceWriteGate(cmd *cobra.Command) error {
	if cmd.Annotations["write"] != "true" {
		return nil
	}

	rwFlag := cmd.Flag("rw")
	if rwFlag != nil {
		rw, err := strconv.ParseBool(rwFlag.Value.String())
		if err != nil {
			return err
		}
		if rw {
			return nil
		}
	}

	if detectAgent() {
		return clierr.NewUsageError("this command writes; pass --rw to allow it")
	}

	if stdoutIsTerminal() {
		return nil
	}

	return clierr.NewUsageError("this command writes; pass --rw to allow it")
}

// ComposeRootPersistentPreRunE returns a root hook that binds format before enforcing --rw.
func ComposeRootPersistentPreRunE(cfg *clicfg.Config) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := BindFormatFromFlag(cmd, cfg); err != nil {
			return silenceUsageOnError(cmd, err)
		}
		return silenceUsageOnError(cmd, EnforceWriteGate(cmd))
	}
}

// RegisterAuraCredentialFlag adds a persistent --credential/-c flag to cmd and
// wraps any existing PersistentPreRunE with a hook that resolves the named
// credential from the store and stores it via cfg.Aura.SetActiveCredential.
//
// Hook execution order:
//  1. Run the pre-existing PersistentPreRunE (if any); abort on error.
//  2. If --credential was set, look up the credential by name.
//     On failure, return a usage error hinting the correct credential list command.
//     On success, call cfg.Aura.SetActiveCredential.
//  3. If --credential was not set, cfg is left unchanged (GetDefault fallback applies).
func RegisterAuraCredentialFlag(cmd *cobra.Command, cfg *clicfg.Config) {
	cmd.PersistentFlags().StringP(
		"credential",
		"c",
		"",
		"Name of a stored Aura credential to use for the command (see 'neo4j-cli credential aura-client list')",
	)

	prior := cmd.PersistentPreRunE

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if prior != nil {
			if err := prior(cmd, args); err != nil {
				return silenceUsageOnError(cmd, err)
			}
		}

		credFlag := cmd.Flag("credential")
		if credFlag == nil || !credFlag.Changed {
			return nil
		}

		name := credFlag.Value.String()
		cred, err := cfg.Credentials.Aura.Get(name)
		if err != nil {
			rootUse := cmd.Root().Use
			var hint string
			if rootUse == "neo4j-cli" {
				hint = "neo4j-cli aura credential list"
			} else {
				hint = "aura-cli credential list"
			}
			return silenceUsageOnError(cmd, clierr.NewUsageError("credential %q not found, run `%s` to see available credentials", name, hint))
		}

		cfg.Aura.SetActiveCredential(cred)
		return nil
	}
}
