// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import "errors"

type Memory string

// String is used both by fmt.Print and by Cobra in help text
func (e *Memory) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *Memory) Set(v string) error {
	switch v {
	case "1GB", "2GB", "4GB", "8GB", "16GB", "24GB", "32GB", "48GB", "64GB", "128GB", "192GB", "256GB", "384GB", "512GB":
		*e = Memory(v)
		return nil
	default:
		return errors.New(`must be one of "1GB", "2GB", "4GB", "8GB", "16GB", "24GB", "32GB", "48GB", "64GB", "128GB", "192GB", "256GB", "384GB", or "512GB"`)
	}
}

// Type is only used in help text
func (e *Memory) Type() string {
	return "memory"
}
