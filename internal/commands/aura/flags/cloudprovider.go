// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import "errors"

type CloudProvider string

// String is used both by fmt.Print and by Cobra in help text
func (e *CloudProvider) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *CloudProvider) Set(v string) error {
	switch v {
	case "aws", "azure", "gcp":
		*e = CloudProvider(v)
		return nil
	default:
		return errors.New(`must be one of "aws", "azure", or "gcp"`)
	}
}

// Type is only used in help text
func (e *CloudProvider) Type() string {
	return "cloud-provider"
}
