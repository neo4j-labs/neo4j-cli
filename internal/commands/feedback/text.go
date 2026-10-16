// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

const (
	groupShort  = "Send feedback about neo4j-cli to the Neo4j team"
	submitShort = "Send a feedback report about neo4j-cli"

	// dataPolicyBlock is appended to the submit Long text.
	dataPolicyBlock = "Describe product behavior in your own words. Do not include query text (Cypher), parameters, " +
		"schema/label/property names, result rows, pasted command output or logs, file paths, connection URIs, hostnames, " +
		"credentials, emails, names, or account/organization/database identifiers. Reports that appear to contain these are " +
		"rejected locally and never sent; the check cannot catch everything, so leave such details out. This is product feedback, not a support request; no response is guaranteed. " +
		"Keep expected and observed each under 255 characters."

	// enableDisclosure is shown by `feedback enable` without --accept.
	// <endpoint> and <date> are substituted at runtime.
	enableDisclosure = "Enabling neo4j-cli feedback. When enabled, an agent or user can send short feedback reports to: <endpoint>. " +
		"Each report contains only: category, expected, observed, optional command name, optional error code, optional public agent harness/model label, CLI version, operating system and CPU architecture, how the CLI was run (agent, script or person), a random request id. " +
		"Reports are checked locally and rejected if they appear to contain query text, parameters, schema names, output, file paths, URIs, hostnames, credentials, emails, names, or identifiers. " +
		"Authorization expires on <date>. Revoke any time: neo4j-cli feedback disable. Re-run with --accept to enable."
)
