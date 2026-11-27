// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package envfile

import (
	"errors"
	"strings"
)

// doubleQuoteEscaper escapes the characters that Parse treats as special
// inside double quotes.
var doubleQuoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`, "\r", `\r`)

// Quote returns value in a form that Parse reads back unchanged.
//
// Parse reads a single-quoted value literally, so Quote uses single quotes
// whenever it can. A value that contains a single quote or a line ending
// needs double quotes instead, with backslashes, double quotes, dollar signs
// and line endings escaped.
//
// Quote returns an error for the two kinds of value that Parse would change:
//   - a value that ends in a backslash, because Parse reads the closing quote
//     after it as escaped;
//   - a value that needs double quotes and also contains the text \n or \r,
//     because inside double quotes Parse turns that text into a line ending.
func Quote(value string) (string, error) {
	if strings.HasSuffix(value, `\`) {
		return "", errors.New("a value that ends in a backslash cannot be written to a dotenv file")
	}
	if !strings.ContainsAny(value, "'\n\r") {
		return "'" + value + "'", nil
	}
	if strings.Contains(value, `\n`) || strings.Contains(value, `\r`) {
		return "", errors.New(`a value with a single quote or line ending cannot also contain the text \n or \r`)
	}
	return `"` + doubleQuoteEscaper.Replace(value) + `"`, nil
}
