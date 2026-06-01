// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import "strings"

// splitStatements splits a Cypher string into individual statements on a
// semicolon that ends a line (";" followed by optional trailing whitespace then
// a newline, or ";" at the very end of the input). A ";" that is not at end of
// line is kept verbatim within the statement. The terminating ";" is stripped,
// comments (// to end-of-line and /* ... */ blocks) outside string literals are
// removed, each statement is whitespace-trimmed, and empty fragments are
// dropped. CRLF line endings are handled identically to LF. Non-empty input
// always yields at least one statement.
//
// The end-of-line ";" split decision is deliberately comment- and
// string-unaware: a ";" ending a line splits even inside a /* ... */ block or a
// multi-line string literal. Comment stripping runs only after a fragment is
// accumulated, so a block comment must not contain a line-final ";".
func splitStatements(cypher string) []string {
	var statements []string
	var buf strings.Builder

	flush := func(trimmedLine string) {
		buf.WriteString(trimmedLine)
		if stmt := strings.TrimSpace(stripComments(buf.String())); stmt != "" {
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

	if stmt := strings.TrimSpace(stripComments(buf.String())); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// stripComments removes Cypher line comments (// to end-of-line) and block
// comments (/* ... */, possibly spanning lines) from s, leaving comment markers
// that appear inside '...', "..." or `...` string literals untouched. Single-
// and double-quoted literals honour backslash escapes; backtick literals do
// not. An unterminated string literal or block comment runs to end of input.
func stripComments(s string) string {
	var out strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch c {
		case '\'', '"', '`':
			quote := c
			out.WriteRune(c)
			i++
			for ; i < len(runes); i++ {
				r := runes[i]
				out.WriteRune(r)
				if (quote == '\'' || quote == '"') && r == '\\' && i+1 < len(runes) {
					i++
					out.WriteRune(runes[i])
					continue
				}
				if r == quote {
					break
				}
			}
		case '/':
			if i+1 < len(runes) && runes[i+1] == '/' {
				for i+1 < len(runes) && runes[i+1] != '\n' {
					i++
				}
			} else if i+1 < len(runes) && runes[i+1] == '*' {
				i += 2
				for i+1 < len(runes) && (runes[i] != '*' || runes[i+1] != '/') {
					i++
				}
				i++
			} else {
				out.WriteRune(c)
			}
		default:
			out.WriteRune(c)
		}
	}
	return out.String()
}
