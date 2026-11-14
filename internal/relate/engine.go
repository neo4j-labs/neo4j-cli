// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// This file is the engine implementation behind the package seam: it imports
// the cgo client (which links librelate_ffi at build time) and installs the
// real OpenStore / OpenStoreReadOnly hooks. It is compiled only under the
// `relate` build tag with CGO_ENABLED=1; every other file of this package
// compiles in every build and treats a nil hook as "not compiled in".

package relate

import (
	"errors"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
	relateclient "github.com/relatedb/relate/clients/go"
)

func init() {
	OpenStore = openClientStore(false)
	OpenStoreReadOnly = openClientStore(true)
}

// openClientStore returns the opener for one open mode. Opening a missing
// path creates the store in read-write mode; the read-only opener refuses a
// missing store (and later refuses writes and checkpoints).
func openClientStore(readOnly bool) func(path, namespace string) (Store, error) {
	return func(path, namespace string) (Store, error) {
		var db *relateclient.Database
		var err error
		if readOnly {
			db, err = relateclient.OpenReadOnly(path)
		} else {
			db, err = relateclient.Open(path)
		}
		if err != nil {
			return nil, convertError(err)
		}

		var conn *relateclient.Connection
		if namespace == "" {
			conn, err = db.Connect()
		} else {
			conn, err = db.ConnectNamespace(namespace)
		}
		if err != nil {
			db.Close()
			return nil, convertError(err)
		}
		return &clientStore{db: db, conn: conn, readOnly: readOnly}, nil
	}
}

// clientStore is the Store implementation over one client Database and its
// single Connection. Relate connections are sequential (one explicit
// transaction at a time, single-level), which matches the CLI's statement
// flow exactly.
type clientStore struct {
	db       *relateclient.Database
	conn     *relateclient.Connection
	readOnly bool
}

// Run implements Store. Non-atomic runs execute each statement in its own
// auto-commit transaction and fail fast on the first error (earlier
// statements stay committed, matching the CLI's default per-statement
// transaction mode). Atomic runs wrap the whole batch in one explicit
// transaction: any error rolls everything back.
func (s *clientStore) Run(statements []string, params map[string]any, atomic bool) ([]StatementResult, error) {
	if atomic {
		if err := s.conn.Begin(); err != nil {
			return nil, convertError(err)
		}
	}
	results := make([]StatementResult, 0, len(statements))
	for _, stmt := range statements {
		res, err := s.conn.QueryParams(stmt, params)
		if err != nil {
			if atomic {
				_ = s.conn.Rollback() //nolint:errcheck // rollback is best-effort; the original error is returned
			}
			return nil, convertError(err)
		}
		results = append(results, convertResult(res))
	}
	if atomic {
		if _, err := s.conn.Commit(); err != nil {
			return nil, convertError(err)
		}
	}
	return results, nil
}

func (s *clientStore) Checkpoint() error {
	return convertError(s.db.Checkpoint())
}

func (s *clientStore) Close() {
	s.conn.Close()
	s.db.Close()
}

// convertError maps a client failure onto the package boundary: the client's
// typed *relateclient.Error becomes the cgo-free *EngineError (fields copied,
// never the message reparsed); any other error (the pre-statement refusals:
// interior NULs, unsupported parameter types) passes through unchanged so the
// command layer can classify it as a validation error.
func convertError(err error) error {
	if err == nil {
		return nil
	}
	var ce *relateclient.Error
	if errors.As(err, &ce) {
		return &EngineError{
			Code:      ce.Code,
			GQLStatus: ce.GQLStatus,
			Stage:     string(ce.Stage),
			Retryable: ce.Retryable,
			Message:   ce.Message,
		}
	}
	return err
}

// convertResult snapshots one client Result into the driver-free
// StatementResult: columns as-is, rows converted cell by cell into the CLI's
// cell model (dbtype graph entities and containers; the query package's
// coercion layer owns display formatting), the query type and bookmark, the
// mutation counters (zeros included on every result), and the plan tree when
// the statement carried one.
func convertResult(res *relateclient.Result) StatementResult {
	out := StatementResult{
		Columns:   res.Columns,
		Rows:      make([][]any, len(res.Rows)),
		QueryType: QueryType(res.QueryType),
		Bookmark:  res.Bookmark,
		Plan:      convertPlan(res.Plan),
		Stats: StatementStats{
			NodesCreated:         res.Stats.NodesCreated,
			NodesDeleted:         res.Stats.NodesDeleted,
			RelationshipsCreated: res.Stats.RelationshipsCreated,
			RelationshipsDeleted: res.Stats.RelationshipsDeleted,
			PropertiesSet:        res.Stats.PropertiesSet,
			LabelsAdded:          res.Stats.LabelsAdded,
			LabelsRemoved:        res.Stats.LabelsRemoved,
			IndexesAdded:         res.Stats.IndexesAdded,
			IndexesRemoved:       res.Stats.IndexesRemoved,
			ConstraintsAdded:     res.Stats.ConstraintsAdded,
			ConstraintsRemoved:   res.Stats.ConstraintsRemoved,
		},
	}
	for i, row := range res.Rows {
		r := make([]any, len(row))
		for j, v := range row {
			r[j] = convertValue(v)
		}
		out.Rows[i] = r
	}
	return out
}

// convertPlan converts the engine's relate.plan/v1 document into the
// package's driver-free Plan tree. A nil document (a plain run) stays nil.
func convertPlan(doc *relateclient.PlanDocument) *Plan {
	if doc == nil {
		return nil
	}
	return convertPlanNode(doc.Root)
}

func convertPlanNode(node relateclient.PlanNode) *Plan {
	out := &Plan{
		Operator:    node.Operator,
		Details:     node.Details,
		Identifiers: node.Identifiers,
	}
	if node.EstimatedRows != nil {
		out.EstimatedRows = *node.EstimatedRows
	}
	if node.Actual != nil {
		out.Actual = &PlanActual{
			Rows:      node.Actual.Rows,
			DbHits:    node.Actual.DbHits,
			TimeMs:    node.Actual.TimeMs,
			NodeReads: node.Actual.NodeReads,
		}
	}
	for _, child := range node.Children {
		if c := convertPlanNode(child); c != nil {
			out.Children = append(out.Children, *c)
		}
	}
	return out
}

// convertValue converts one engine cell into the value model the CLI's Bolt
// path already renders: graph entities and temporals map onto the driver's
// dbtype shapes (field-for-field compatible), containers recurse, and scalars
// (nil, bool, int64, float64, string) pass through. The output therefore
// feeds the query package's existing coerceDriverValue/render layer unchanged.
func convertValue(v any) any {
	switch val := v.(type) {
	case relateclient.Node:
		return dbtype.Node{
			Id:        val.ID,
			ElementId: val.ElementID,
			Labels:    val.Labels,
			Props:     convertProps(val.Properties),
		}
	case relateclient.Relationship:
		return dbtype.Relationship{
			Id:             val.ID,
			ElementId:      val.ElementID,
			StartId:        val.StartID,
			StartElementId: val.StartElementID,
			EndId:          val.EndID,
			EndElementId:   val.EndElementID,
			Type:           val.Type,
			Props:          convertProps(val.Properties),
		}
	case relateclient.Path:
		path := dbtype.Path{}
		for _, el := range val {
			switch e := el.(type) {
			case relateclient.Node:
				path.Nodes = append(path.Nodes, convertValue(e).(dbtype.Node))
			case relateclient.Relationship:
				path.Relationships = append(path.Relationships, convertValue(e).(dbtype.Relationship))
			}
		}
		return path
	case relateclient.Date:
		return dbtype.Date(dateOf(int(val.Year), int(val.Month), int(val.Day)))
	case relateclient.LocalTime:
		t := time.Date(2000, time.January, 1, int(val.Hour), int(val.Minute), int(val.Second), int(val.Nanosecond), time.UTC)
		return dbtype.LocalTime(t)
	case relateclient.Time:
		t := time.Date(2000, time.January, 1, int(val.Hour), int(val.Minute), int(val.Second), int(val.Nanosecond),
			fixedZoneOf(val.OffsetSeconds))
		return dbtype.Time(t)
	case relateclient.LocalDateTime:
		t := time.Date(int(val.Year), time.Month(val.Month), int(val.Day),
			int(val.Hour), int(val.Minute), int(val.Second), int(val.Nanosecond), time.UTC)
		return dbtype.LocalDateTime(t)
	case relateclient.DateTime:
		// A zoned DATETIME renders as a native time.Time (the Bolt path does
		// the same); keep the IANA zone when the host tzdata resolves it and
		// fall back to the recorded offset otherwise.
		loc := locationOf(val.Zone, val.OffsetSeconds)
		return time.Date(int(val.Year), time.Month(val.Month), int(val.Day),
			int(val.Hour), int(val.Minute), int(val.Second), int(val.Nanosecond), loc)
	case relateclient.Duration:
		return dbtype.Duration{
			Months:  val.Months,
			Days:    val.Days,
			Seconds: val.Seconds,
			Nanos:   int(val.Nanos),
		}
	case relateclient.Point:
		if val.Dimension == 3 {
			return dbtype.Point3D{SpatialRefId: uint32(val.SRID), X: val.X, Y: val.Y, Z: val.Z}
		}
		return dbtype.Point2D{SpatialRefId: uint32(val.SRID), X: val.X, Y: val.Y}
	case relateclient.Vector:
		return convertVector(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = convertValue(item)
		}
		return out
	case map[string]any:
		return convertProps(val)
	default:
		return v
	}
}

// convertProps converts a property map's values in place-free fashion (a new
// map), recursing through convertValue.
func convertProps(props map[string]any) map[string]any {
	if props == nil {
		return nil
	}
	out := make(map[string]any, len(props))
	for k, v := range props {
		out[k] = convertValue(v)
	}
	return out
}

// convertVector maps an engine VECTOR cell onto the driver's dbtype vector
// shape of the matching family. The engine's coordinate dtypes within a
// family (INTEGER8/16/32/INTEGER, FLOAT16/BFLOAT16/FLOAT32/FLOAT) share one
// dbtype rendering per family, which is display-equivalent to the Bolt path.
func convertVector(v relateclient.Vector) any {
	if strings.HasPrefix(v.DType, "INTEGER") {
		elems := make([]int64, len(v.Ints))
		copy(elems, v.Ints)
		return dbtype.Vector[int64]{Elems: elems}
	}
	elems := make([]float64, len(v.Floats))
	copy(elems, v.Floats)
	return dbtype.Vector[float64]{Elems: elems}
}

// dateOf builds the midnight UTC instant a dbtype.Date wraps.
func dateOf(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// fixedZoneOf builds the fixed offset zone a dbtype.Time wraps.
func fixedZoneOf(offsetSeconds int64) *time.Location {
	return time.FixedZone("", int(offsetSeconds))
}

// locationOf resolves an IANA zone id, falling back to a fixed offset zone
// when the id is empty or the host's tzdata cannot resolve it (a stripped
// deployment, or an unknown zone).
func locationOf(zone string, offsetSeconds int64) *time.Location {
	if zone != "" {
		if loc, err := time.LoadLocation(zone); err == nil {
			return loc
		}
	}
	return fixedZoneOf(offsetSeconds)
}
