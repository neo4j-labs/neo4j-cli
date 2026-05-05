// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clierr"
)

// formatAgentErr converts skill-package sentinel errors into user-facing
// usage errors that include the valid agent names. Other errors pass
// through unchanged. Used by install + remove leaves.
func formatAgentErr(err error) error {
	switch {
	case errors.Is(err, ErrUnknownAgent):
		return clierr.NewUsageError("%v\nvalid agents: %s", err, strings.Join(agentNames(), ", "))
	case errors.Is(err, ErrAgentNotDetected):
		return clierr.NewUsageError("%v", err)
	case errors.Is(err, ErrNoAgentsDetected):
		return clierr.NewUsageError("%v\nvalid agents: %s", err, strings.Join(agentNames(), ", "))
	default:
		return err
	}
}

func agentNames() []string {
	names := make([]string, 0, len(AGENTS))
	for i := range AGENTS {
		names = append(names, AGENTS[i].Name)
	}
	return names
}

// printJSON marshals v as indented JSON and prints it to cmd's stdout.
// Used by install/remove/list/check leaves.
func printJSON(cmd *cobra.Command, v any) {
	bytes, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		// Marshalling our own structs cannot fail in practice; mirror the
		// existing output package's posture (panic on impossible-state).
		panic(err)
	}
	cmd.Println(string(bytes))
}
