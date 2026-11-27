// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// OpenOptFlag is the name of the repeatable flag that carries engine open
// options on every command that opens a store.
const OpenOptFlag = "relate-opt"

// OpenOptFlagUsage is the shared help text for OpenOptFlag.
const OpenOptFlagUsage = "Relate engine open option as key=value (repeatable; the first '=' splits key from value, so values may contain ';' and '='). " +
	"Keys: snapshot-format (parquet|json|markdown; a new store only), markdown-body-properties, checkpoint-interval-secs, " +
	"durability-mode (full|everysecond|snapshot), logical-graph-size-limit, expression-memory-limit. " +
	"Same names and values as the standalone `relate` CLI flags; overrides the matching RELATE_* environment variable"

// openOpt describes one accepted --relate-opt key. The Go client's Open takes
// only a path, so the engine reads these from RELATE_* environment variables
// at open time; this table is the allow-list that maps a key onto its
// variable, so a caller cannot set arbitrary engine environment.
type openOpt struct {
	env      string
	validate func(value string) error
}

var openOpts = map[string]openOpt{
	"snapshot-format":          {"RELATE_SNAPSHOT_FORMAT", oneOf("parquet", "json", "markdown")},
	"markdown-body-properties": {"RELATE_MARKDOWN_BODY_PROPERTIES", nonEmpty},
	"checkpoint-interval-secs": {"RELATE_CHECKPOINT_INTERVAL_SECS", nonNegativeInt},
	"durability-mode":          {"RELATE_DURABILITY_MODE", oneOf("full", "everysecond", "snapshot")},
	"logical-graph-size-limit": {"RELATE_LOGICAL_GRAPH_SIZE_LIMIT", memorySize(false)},
	"expression-memory-limit":  {"RELATE_EXPRESSION_MEMORY_LIMIT", memorySize(true)},
}

// OpenOptKeys returns the accepted keys, sorted.
func OpenOptKeys() []string {
	keys := make([]string, 0, len(openOpts))
	for k := range openOpts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func nonEmpty(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("must not be empty")
	}
	return nil
}

// memorySizePattern is the grammar the engine accepts for its memory limits
// (checked against the standalone CLI): whole bytes, or a whole number with a
// mb, mib, gb or gib suffix — case-insensitive, no decimals, no kb.
var memorySizePattern = regexp.MustCompile(`(?i)^\d+\s*(mb|mib|gb|gib)?$`)

func memorySize(allowNone bool) func(string) error {
	return func(v string) error {
		v = strings.TrimSpace(v)
		if allowNone && strings.EqualFold(v, "none") {
			return nil
		}
		if !memorySizePattern.MatchString(v) {
			want := "bytes or a whole number with an mb, mib, gb or gib suffix (for example 512mb)"
			if allowNone {
				want += ", or none"
			}
			return fmt.Errorf("must be %s", want)
		}
		return nil
	}
}

func nonNegativeInt(v string) error {
	if n, err := strconv.Atoi(v); err != nil || n < 0 {
		return fmt.Errorf("must be a non-negative integer")
	}
	return nil
}

func oneOf(allowed ...string) func(string) error {
	return func(v string) error {
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s", strings.Join(allowed, ", "))
	}
}

// ApplyOpenOpts validates the raw key=value pairs and exports them for the
// engine, which reads them when the store is opened — call it BEFORE
// OpenStore. Nothing is exported unless every pair is valid. An explicit
// option wins over an ambient RELATE_* variable. The process is a one-shot
// CLI, so the variables are left set for its lifetime.
func ApplyOpenOpts(raw []string) error {
	if len(raw) == 0 {
		return nil
	}
	resolved := make(map[string]string, len(raw))
	for _, pair := range raw {
		key, value, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return clierr.NewUsageError("--%s %q: expected key=value (keys: %s)", OpenOptFlag, pair, strings.Join(OpenOptKeys(), ", "))
		}
		opt, known := openOpts[key]
		if !known {
			return clierr.NewUsageError("--%s: unknown key %q (keys: %s)", OpenOptFlag, key, strings.Join(OpenOptKeys(), ", "))
		}
		if _, dup := resolved[opt.env]; dup {
			return clierr.NewUsageError("--%s: key %q given more than once", OpenOptFlag, key)
		}
		if err := opt.validate(value); err != nil {
			return clierr.NewUsageError("--%s %s: %s", OpenOptFlag, key, err)
		}
		resolved[opt.env] = value
	}
	for env, value := range resolved {
		if err := os.Setenv(env, value); err != nil {
			return clierr.NewFatalError("cannot set %s: %w", env, err)
		}
	}
	return nil
}
