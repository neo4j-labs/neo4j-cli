// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

// ExportedUserExecFn exposes the package-level userExecFn seam for use by
// the external test package (package user_test). Only compiled during tests.
var ExportedUserExecFn = &userExecFn
