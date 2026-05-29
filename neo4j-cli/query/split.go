// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import "strings"

// splitStatements splits a Cypher string into individual statements on a
// semicolon that ends a line (";" followed by optional trailing whitespace then
// a newline, or ";" at the very end of the input). A ";" that is not at end of
// line is kept verbatim within the statement. The terminating ";" is stripped,
// each statement is whitespace-trimmed, and empty fragments are dropped. CRLF
// line endings are handled identically to LF. Non-empty input always yields at
// least one statement.
func splitStatements(cypher string) []string {
	var statements []string
	var buf strings.Builder

	flush := func(trimmedLine string) {
		buf.WriteString(trimmedLine)
		if stmt := strings.TrimSpace(buf.String()); stmt != "" {
			statements = append(statements, stmt)
		}
		buf.Reset()
	}

	lines := strings.Split(cypher, "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasSuffix(strings.TrimRight(line, " \t"), ";") {
			withoutSemi := strings.TrimRight(line, " \t")
			withoutSemi = withoutSemi[:len(withoutSemi)-1]
			flush(withoutSemi)
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}

	if stmt := strings.TrimSpace(buf.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}
