// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// Receipt is one line of feedback-receipts.jsonl, the local record of every
// report attempt. Payload is null for local rejections so rejected text is never
// stored.
type Receipt struct {
	Time        string   `json:"time"`
	Outcome     string   `json:"outcome"`
	Reason      string   `json:"reason"`
	RequestID   string   `json:"request_id"`
	ReportID    string   `json:"report_id"`
	Destination string   `json:"destination"`
	ElapsedMS   int64    `json:"elapsed_ms"`
	Payload     *Payload `json:"payload"`
}

// appendReceipt appends one receipt line with file mode 0600. It returns the
// path written, or "" on error; callers treat receipts as best-effort.
func appendReceipt(fs afero.Fs, path string, r Receipt) string {
	if fs == nil {
		return ""
	}
	if r.Time == "" {
		r.Time = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	if err := fs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ""
	}
	f, err := fs.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return ""
	}
	_, werr := f.Write(append(line, '\n'))
	cerr := f.Close()
	_ = fs.Chmod(path, 0o600)
	if werr != nil || cerr != nil {
		return ""
	}
	return path
}
