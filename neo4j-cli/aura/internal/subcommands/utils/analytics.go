// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import (
	"github.com/neo4j/cli/common/analytics"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// sensitiveFlags are flag names whose values must not appear in analytics.
var sensitiveFlags = map[string]bool{
	"client-secret": true,
	"password":      true,
	"token":         true,
}

// BuildActiveFlags returns an ActiveFlags map of every flag the user
// explicitly set on cmd. Values of sensitive flags are replaced with
// "REDACTED".
func BuildActiveFlags(cmd *cobra.Command) analytics.ActiveFlags {
	flags := make(analytics.ActiveFlags)
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if sensitiveFlags[f.Name] {
			flags[f.Name] = "REDACTED"
		} else {
			flags[f.Name] = f.Value.String()
		}
	})
	return flags
}

// WrapRunE wraps a cobra RunE function so that a COMMAND_USED analytics
// event is emitted after the inner function returns, capturing the command
// path, success/failure, and any flags the user explicitly set.
func WrapRunE(cfg *clicfg.Config, inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := inner(cmd, args)
		cfg.Events.EmitCommandEvent(cmd.CommandPath(), err == nil, BuildActiveFlags(cmd))
		return err
	}
}
