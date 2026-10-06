// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/log"
)

// dbconnLogTimeFormat is the timestamp format shared by both the query and
// admin Bolt driver loggers. Factored out here so both users stay in sync.
const dbconnLogTimeFormat = "2006-01-02 15:04:05.000"

// stderrLoggerImpl is the shared implementation of neo4j/log.Logger that routes
// all Bolt driver log output to stderr (or a configurable writer) so it never
// contaminates stdout (which carries the CLI's machine-readable output). A
// minimum level filter lets callers suppress noisier levels: only messages at
// or above minLevel are written.
type stderrLoggerImpl struct {
	w        io.Writer
	minLevel log.Level
}

// NewStderrLogger constructs a neo4j/log.Logger that writes timestamped log
// lines to os.Stderr at or above minLevel.  Passing log.DEBUG enables all four
// levels (ERROR, WARNING, INFO, DEBUG), which is the correct choice when
// conn.Debug is true.
func NewStderrLogger(minLevel log.Level) log.Logger {
	return &stderrLoggerImpl{w: os.Stderr, minLevel: minLevel}
}

func (l *stderrLoggerImpl) Error(name, id string, err error) {
	if l.minLevel < log.ERROR {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s  ERROR  [%s %s] %s\n", time.Now().Format(dbconnLogTimeFormat), name, id, err.Error())
}

func (l *stderrLoggerImpl) Warnf(name, id, msg string, args ...any) {
	if l.minLevel < log.WARNING {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s   WARN  [%s %s] %s\n", time.Now().Format(dbconnLogTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

func (l *stderrLoggerImpl) Infof(name, id, msg string, args ...any) {
	if l.minLevel < log.INFO {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s   INFO  [%s %s] %s\n", time.Now().Format(dbconnLogTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

func (l *stderrLoggerImpl) Debugf(name, id, msg string, args ...any) {
	if l.minLevel < log.DEBUG {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s  DEBUG  [%s %s] %s\n", time.Now().Format(dbconnLogTimeFormat), name, id, fmt.Sprintf(msg, args...))
}
