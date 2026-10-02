// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import "errors"

type AuthProviderType string

// String is used both by fmt.Print and by Cobra in help text
func (e *AuthProviderType) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *AuthProviderType) Set(v string) error {
	switch v {
	case "api-key", "jwks":
		*e = AuthProviderType(v)
		return nil
	default:
		return errors.New(`must be one of "api-key" or "jwks"`)
	}
}

// Type is only used in help text
func (e *AuthProviderType) Type() string {
	return "type"
}
