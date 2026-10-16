// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

// MCP server instructions are sent in the initialize reply. Claude Code keeps
// them in the agent's context even when it defers the tool's own description,
// so they are the place for a short pointer to feedback_submit; without them
// agents rarely report unprompted. The tool description itself stays fixed.
const (
	// instructionsEnv selects the variant: "none", "basic" or "optin". Unset
	// means defaultInstructions.
	instructionsEnv = "NEO4J_CLI_FEEDBACK_INSTRUCTIONS"

	instructionsBasic = "If neo4j-cli prevented or complicated your task (a missing capability, an unclear or unhelpful error, " +
		"behavior that differs from its help, or impractical performance), report it once with the feedback_submit tool " +
		"after you finish the task, even if you found a workaround. Never include queries, parameters, schema names, " +
		"results, file paths, connection details, credentials, names or identifiers."

	instructionsOptIn = instructionsBasic +
		" The operator has enabled feedback on this machine, and the client asks the user to approve the first call, " +
		"so you do not need to ask the user separately before calling the tool."
)

// defaultInstructions is the variant used when the environment variable is
// unset. optin is a research setting for controlled tests only; every build
// other people run uses basic.
const defaultInstructions = "basic"

// serverInstructions returns the instructions text for a variant. "" selects
// defaultInstructions, "none" sends no instructions, and unknown values fall
// back to basic (the conservative text).
func serverInstructions(variant string) string {
	if variant == "" {
		variant = defaultInstructions
	}
	switch variant {
	case "none":
		return ""
	case "optin":
		return instructionsOptIn
	default:
		return instructionsBasic
	}
}

// enableInstructions is the variant `feedback enable` registers by default.
const enableInstructions = "basic"

func validInstructionsVariant(v string) bool {
	return v == "none" || v == "basic" || v == "optin"
}

// endpointEnv is an operator-only override for the feedback endpoint, used when
// --endpoint is not given (the install script calls plain `feedback enable`).
// It accepts only the destinations endpointAllowed permits: a loopback test
// server or a Mixpanel ingestion host.
const endpointEnv = "NEO4J_CLI_FEEDBACK_ENDPOINT"
