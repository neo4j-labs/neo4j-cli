// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import "github.com/neo4j/cli/internal/dbconn"

// categorizeBoltError is the shared Bolt-error classifier (dbconn owns it so
// the relate pull/push leaves classify remote failures identically).
func categorizeBoltError(err error) error { return dbconn.CategorizeBoltError(err) }

// Codes the classifier tests assert against.
const (
	forbiddenTxTypeCode = "Neo.ClientError.Transaction.ForbiddenDueToTransactionType"
	unauthorizedCode    = "Neo.ClientError.Security.Unauthorized"
)
