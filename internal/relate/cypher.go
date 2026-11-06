// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

// QuoteIdent renders a label or relationship type as a Cypher identifier,
// backticked with internal backticks doubled. Always backticking (not just
// when the name needs it) keeps the rendering uniform and safe for any
// spelling the engine can store.
func QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// CypherLiteral renders v as a self-contained Cypher literal preserving its
// type: scalars, strings, lists and maps recurse; the driver's temporal and
// spatial shapes render as their constructor forms (date('...'),
// datetime('...'), point({...}), duration({...})). It is the write path for
// pulled property values — the engine's typed parameter binder does not
// cover the temporal and spatial kinds from Go yet, so materialization
// inlines values as literals instead of binding them.
//
// An unsupported value returns an error naming its Go type; callers surface
// that instead of silently degrading the value.
func CypherLiteral(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "NULL", nil
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	case string:
		return quoteString(val), nil
	case int:
		return strconv.FormatInt(int64(val), 10), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case float64:
		return formatFloat(val), nil
	case float32:
		return formatFloat(float64(val)), nil
	case []any:
		parts := make([]string, len(val))
		for i, item := range val {
			lit, err := CypherLiteral(item)
			if err != nil {
				return "", err
			}
			parts[i] = lit
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]any:
		return literalMap(val)
	case dbtype.Date:
		return "date('" + val.Time().Format("2006-01-02") + "')", nil
	case dbtype.LocalTime:
		return "localtime('" + val.Time().Format("15:04:05.999999999") + "')", nil
	case dbtype.Time:
		return "time('" + val.Time().Format("15:04:05.999999999-07:00") + "')", nil
	case dbtype.LocalDateTime:
		return "localdatetime('" + val.Time().Format("2006-01-02T15:04:05.999999999") + "')", nil
	case time.Time:
		return "datetime('" + val.Format("2006-01-02T15:04:05.999999999-07:00") + "')", nil
	case dbtype.Duration:
		return fmt.Sprintf("duration({months: %d, days: %d, seconds: %d, nanoseconds: %d})",
			val.Months, val.Days, val.Seconds, val.Nanos), nil
	case dbtype.Point2D:
		return fmt.Sprintf("point({srid: %d, x: %s, y: %s})",
			val.SpatialRefId, formatFloat(val.X), formatFloat(val.Y)), nil
	case dbtype.Point3D:
		return fmt.Sprintf("point({srid: %d, x: %s, y: %s, z: %s})",
			val.SpatialRefId, formatFloat(val.X), formatFloat(val.Y), formatFloat(val.Z)), nil
	default:
		return "", fmt.Errorf("cannot render %T as a Cypher literal", v)
	}
}

// literalMap renders a property map as a Cypher map literal with its keys in
// sorted order (deterministic statements) and values as literals.
func literalMap(m map[string]any) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		lit, err := CypherLiteral(m[k])
		if err != nil {
			return "", err
		}
		parts = append(parts, quoteString(k)+": "+lit)
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}

// quoteString renders a Cypher string literal: backslashes and quotes are
// escaped, the common control characters use their short escapes, and any
// other control byte becomes its \uXXXX form so a hostile value can never
// break out of the literal.
func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// formatFloat renders a float as a Cypher literal: Infinity/NaN use their
// literal spellings, and exponent forms drop the '+' and leading zeros
// Cypher does not accept (1e+06 → 1e6).
func formatFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	if math.IsNaN(f) {
		return "NaN"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa, exp := s[:i], s[i+1:]
		neg := false
		exp = strings.TrimPrefix(exp, "+")
		if strings.HasPrefix(exp, "-") {
			neg = true
			exp = strings.TrimPrefix(exp, "-")
		}
		exp = strings.TrimLeft(exp, "0")
		if exp == "" {
			return mantissa
		}
		if neg {
			exp = "-" + exp
		}
		return mantissa + "e" + exp
	}
	return s
}
