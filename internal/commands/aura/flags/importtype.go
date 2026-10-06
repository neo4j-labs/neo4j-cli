// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import "errors"

type ImportType string

// String is used both by fmt.Print and by Cobra in help text
func (e *ImportType) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *ImportType) Set(v string) error {
	switch v {
	case "online", "bulk":
		*e = ImportType(v)
		return nil
	default:
		return errors.New(`must be either "online" or "bulk"`)
	}
}

// Type is only used in help text
func (e *ImportType) Type() string {
	return "import-type"
}
