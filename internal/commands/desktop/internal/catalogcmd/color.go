// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd

import (
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// validPaletteColors are the only `--color` values Desktop's TagSchema
// accepts: the palette indices "1".."12" (NOT hex colours).
var validPaletteColors = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}

// validatePaletteColor rejects any --color value outside Desktop's "1".."12"
// palette. The empty string means "unset" and passes — the client then omits
// the color key from the request body.
func validatePaletteColor(color string) error {
	if color == "" {
		return nil
	}
	for _, v := range validPaletteColors {
		if color == v {
			return nil
		}
	}
	return clierr.NewUsageError(
		"invalid --color %q: must be one of %s (Desktop palette index, not a hex colour)",
		color, strings.Join(validPaletteColors, ", "))
}
