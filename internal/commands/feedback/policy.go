// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

const (
	schemaVersion   = "1"
	surfaceCLI      = "cli"
	maxPayloadBytes = 16384
	maxFieldBytes   = 255 // expected/observed cap (Mixpanel's string property limit); over-limit text is rejected, never truncated
)

// Detector codes plus the structural codes.
const (
	detEmail         = "email"
	detURL           = "url"
	detHostPort      = "host_port"
	detIPAddress     = "ip_address"
	detFilePath      = "file_path"
	detDataFilename  = "data_filename"
	detSecret        = "secret"
	detIdentifier    = "identifier"
	detCypher        = "cypher"
	detParameter     = "parameter"
	detPastedOutput  = "pasted_output"
	detControlChars  = "control_chars"
	detInvalidInput  = "invalid_input"
	detTooLarge      = "too_large"
	detTooLong       = "too_long"
	fieldPayload     = "payload"
	fieldCategory    = "category"
	fieldExpected    = "expected"
	fieldObserved    = "observed"
	fieldRequestID   = "request_id"
	droppedCommandID = "command_id"
	droppedErrorCode = "error_code"
	droppedModel     = "agent.model"
)

var validCategories = []string{"unclear_error", "unexpected_behavior", "missing_capability", "other_friction"}

// Input is what the caller (flags plus environment) hands to Validate.
type Input struct {
	Category   string
	Expected   string
	Observed   string
	Command    string
	ErrorCode  string
	AgentModel string
	RequestID  string
	CLIVersion string
	Harness    string
	// Set by the sender from the running process (never from the report text):
	OS      string // runtime.GOOS: linux, darwin or windows
	OSArch  string // runtime.GOARCH: arm64 or amd64
	Invoker string // agent, script or human, as the CLI's own telemetry classifies it
}

// Agent is the optional agent block of a payload.
type Agent struct {
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
}

// Payload is the only thing that may leave the machine.
type Payload struct {
	SchemaVersion string `json:"schema_version"`
	Surface       string `json:"surface"`
	CLIVersion    string `json:"cli_version"`
	OS            string `json:"os"`
	OSArch        string `json:"os_arch"`
	Invoker       string `json:"invoker"`
	RequestID     string `json:"request_id"`
	Category      string `json:"category"`
	Expected      string `json:"expected"`
	Observed      string `json:"observed"`
	CommandID     string `json:"command_id,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	Agent         *Agent `json:"agent,omitempty"`
}

// Violation names the field and detector that rejected a report. It never
// carries the matched text.
type Violation struct {
	Field    string `json:"field"`
	Detector string `json:"detector"`
}

// Validate applies the field rules and detectors. tree is the root command,
// used for the --command allow-list. The returned payload is always populated
// (so a request id exists), but must only be sent when violations is empty.
func Validate(in Input, tree *cobra.Command) (Payload, []Violation, []string) {
	var violations []Violation
	var dropped []string
	add := func(field, det string) {
		for _, v := range violations {
			if v.Field == field && v.Detector == det {
				return
			}
		}
		violations = append(violations, Violation{Field: field, Detector: det})
	}

	rid, ridOK := resolveRequestID(in.RequestID)
	if !ridOK {
		add(fieldRequestID, detInvalidInput)
	}

	p := Payload{
		SchemaVersion: schemaVersion,
		Surface:       surfaceCLI,
		CLIVersion:    in.CLIVersion,
		OS:            in.OS,
		OSArch:        in.OSArch,
		Invoker:       in.Invoker,
		RequestID:     rid,
		Category:      in.Category,
		Expected:      in.Expected,
		Observed:      in.Observed,
	}

	valid := false
	for _, c := range validCategories {
		if in.Category == c {
			valid = true
		}
	}
	if !valid {
		add(fieldCategory, detInvalidInput)
	}

	for _, f := range []struct{ name, text string }{{fieldExpected, in.Expected}, {fieldObserved, in.Observed}} {
		if strings.TrimSpace(f.text) == "" {
			add(f.name, detInvalidInput)
			continue
		}
		if len(f.text) > maxFieldBytes {
			add(f.name, detTooLong)
		}
		for _, d := range detect(f.text) {
			add(f.name, d)
		}
	}

	if in.Command != "" {
		if commandAllowed(tree, in.Command) {
			p.CommandID = in.Command
		} else {
			dropped = append(dropped, droppedCommandID)
		}
	}
	if in.ErrorCode != "" {
		if errorCodeValid(in.ErrorCode) {
			p.ErrorCode = in.ErrorCode
		} else {
			dropped = append(dropped, droppedErrorCode)
		}
	}

	agent := Agent{Harness: in.Harness}
	if in.AgentModel != "" {
		if modelValid(in.AgentModel) {
			agent.Model = in.AgentModel
		} else {
			dropped = append(dropped, droppedModel)
		}
	}
	if agent != (Agent{}) {
		p.Agent = &agent
	}

	if b, err := json.Marshal(p); err != nil || len(b) > maxPayloadBytes {
		add(fieldPayload, detTooLarge)
	}
	return p, violations, dropped
}

// resolveRequestID returns the caller's UUIDv4 when valid, otherwise a fresh
// one. ok is false only when the caller supplied an invalid value.
func resolveRequestID(s string) (id string, ok bool) {
	if s == "" {
		return newUUIDv4(), true
	}
	if uuidV4.MatchString(s) {
		return strings.ToLower(s), true
	}
	return newUUIDv4(), false
}

func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var uuidV4 = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func errorCodeValid(s string) bool {
	for _, name := range clierr.CodeNames {
		if s == name {
			return true
		}
	}
	return false
}

var (
	modelPrefixes = []string{"claude", "gpt", "gemini", "codex", "glm", "llama", "qwen", "mistral", "deepseek"}
	modelO        = regexp.MustCompile(`^o[0-9]`)
	modelChars    = regexp.MustCompile(`^[a-z0-9][a-z0-9._:\-]*$`)
)

func modelValid(s string) bool {
	if len(s) > 40 || s != strings.ToLower(s) || !modelChars.MatchString(s) {
		return false
	}
	if modelO.MatchString(s) {
		return true
	}
	for _, p := range modelPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// commandPaths collects the visible command paths of the CLI tree (root name
// stripped), excluding the feedback subtree.
func commandPaths(root *cobra.Command) map[string]bool {
	out := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden {
				continue
			}
			path := strings.TrimSpace(strings.TrimPrefix(sub.CommandPath(), root.Name()))
			if path == "feedback" || strings.HasPrefix(path, "feedback ") {
				continue
			}
			out[path] = true
			walk(sub)
		}
	}
	if root != nil {
		walk(root)
	}
	return out
}

func commandAllowed(root *cobra.Command, s string) bool {
	return commandPaths(root)[s]
}

// ---- detectors -----------------------------------------------------------

var (
	reEmail    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reURL      = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://\S+`)
	reHostPort = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9.\-]*:([0-9]{2,5})\b`)
	reIPv4     = regexp.MustCompile(`\b[0-9]{1,3}(?:\.[0-9]{1,3}){3}\b`)
	reIPv6Cand = regexp.MustCompile(`[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*`)
	// A POSIX path needs two segments or a file extension, so an agent's own
	// slash command ("/mcp", "/login") is not mistaken for one.
	rePosixPath = regexp.MustCompile(`(?:^|[\s"'(=,])/[A-Za-z0-9._\-~]+(?:/|\.[A-Za-z0-9]+\b)`)
	reHomePath  = regexp.MustCompile(`(?:^|[\s"'(=,])~[A-Za-z0-9._\-]*/`)
	reRelPath   = regexp.MustCompile(`(?:^|[\s"'(=,])\.{1,2}/`)
	reWinPath   = regexp.MustCompile(`\b[A-Za-z]:[\\/]`)
	reDataFile  = regexp.MustCompile(`(?i)\b[\w\-]+\.(?:csv|tsv|json|jsonl|ndjson|cypher|cql|txt|xlsx?|xml|ya?ml|parquet|sql|log|py|js|ts|go|java|sh|md|zip|gz|tar|pdf|db|dump|backup|toml|ini|conf|properties)\b`)
	reSecretKV  = regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|secret|token|api[_-]?key|apikey|auth|credential)s?(\s*=\s*|\s*:\s*)(\S+)`)
	reSecretKey = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_\-]{8,}|ghp_[A-Za-z0-9]{8,}|xox[a-z]-[A-Za-z0-9\-]{6,}|AKIA[0-9A-Z]{8,}|nams_[A-Za-z0-9_\-]{6,})`)
	reLongToken = regexp.MustCompile(`[A-Za-z0-9+/]{20,}`)
	reUUID      = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	reHexRun    = regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`)
	reParameter = regexp.MustCompile(`--param(?:\s+|=)[^\s=]+=\S*`)
	// Node and relationship patterns: the label follows the colon directly, as
	// in (p:Person) or [:KNOWS], unlike prose such as "(default: false)".
	reCypherNode = regexp.MustCompile(`\(\s*\w*\s*:[A-Za-z_]\w*`)
	reCypherRel  = regexp.MustCompile(`\[\s*\w*\s*:[A-Za-z_]\w*`)
	// Lower-case Cypher, recognized by its structure rather than its keywords.
	reCypherLowerClause = regexp.MustCompile(`(?i)\b(?:optional\s+match|match|merge|create)\s*\(\s*\w*\s*[:)]`)
	reCypherLowerProp   = regexp.MustCompile(`(?i)\b(?:return|where|with|order\s+by)\s+[A-Za-z_]\w*\.[A-Za-z_]\w*`)
	// Names of Cypher features that a report may mention without containing a
	// query; they are removed before keywords are counted.
	reCypherFeature = regexp.MustCompile(`\bCALL\s+(?:\{[^}]*\}\s+)?IN\s+(?:CONCURRENT\s+)?TRANSACTIONS\b`)
	reCypherEdge    = regexp.MustCompile(`-\[[^\]]*\]-`)
	reCypherMap     = regexp.MustCompile(`\{\s*\w+\s*:\s*`)
	reCypherVar     = regexp.MustCompile(`\$[A-Za-z_]\w*`)
	reCypherKW      = regexp.MustCompile(`\b(?:OPTIONAL MATCH|LOAD CSV|MATCH|MERGE|CREATE|RETURN|UNWIND|WITH|WHERE|SET|DELETE|CALL)\b`)
	reOutputJSON    = regexp.MustCompile(`\{\s*"`)
	reOutputArr     = regexp.MustCompile(`\[\s*\{`)
	reOutputPipe    = regexp.MustCompile(`\|[^|\n]*\|[^|\n]*\|`)
	reOutputErr     = regexp.MustCompile(`Error: .*\(exit \d+\)`)
)

// allowedURLPath limits the path of an allowed documentation link to plain
// characters, so nothing else can ride along in it.
var allowedURLPath = regexp.MustCompile(`^[A-Za-z0-9/._\-]{0,160}$`)

// allowedURL reports whether raw is a link to Neo4j's documentation or to the
// neo4j-cli repository: https, the exact host, no user, port, query or
// fragment, and a plain path under the allowed prefix.
func allowedURL(raw string) bool {
	u, err := url.Parse(strings.TrimRight(raw, ".,;:!?)'\""))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || !allowedURLPath.MatchString(u.Path) {
		return false
	}
	switch u.Host {
	case "neo4j.com":
		return strings.HasPrefix(u.Path, "/docs/")
	case "github.com":
		return u.Path == "/neo4j-labs/neo4j-cli" || strings.HasPrefix(u.Path, "/neo4j-labs/neo4j-cli/")
	}
	return false
}

// allowedFilenames are file names that identify neo4j-cli, its documentation
// or well-known tools rather than a user's data.
var allowedFilenames = map[string]bool{
	"config.json": true, "node.js": true, "next.js": true, "vue.js": true, "package.json": true,
	"skill.md": true, "readme.md": true, "agents.md": true, "claude.md": true, "changelog.md": true,
	"install.sh": true, "install-neo4j-cli.sh": true, "install-neo4j-cli.ps1": true, "neo4j.conf": true,
}

// detect returns the detector codes that fire on text, in a stable order.
func detect(text string) []string {
	var out []string
	hit := func(code string, cond bool) {
		if cond {
			out = append(out, code)
		}
	}

	hit(detControlChars, hasControl(text))
	// The detectors run on the compatibility-normalized text, so full-width
	// and other look-alike forms ("ｕｓｅｒ＠ｅｘａｍｐｌｅ．ｃｏｍ") are
	// matched as their plain equivalents.
	text = norm.NFKC.String(text)
	hit(detEmail, reEmail.MatchString(text))

	// URLs: report disallowed ones. An allowed link is replaced by the words of
	// its path, so the other detectors still see what the path carries but
	// not the slashes and host that would make it look like a path or address.
	urlBad := false
	rest := reURL.ReplaceAllStringFunc(text, func(m string) string {
		if allowedURL(m) {
			u, _ := url.Parse(strings.TrimRight(m, ".,;:!?)'\""))
			return " " + strings.ReplaceAll(u.Path, "/", " ") + " "
		}
		urlBad = true
		return m
	})
	hit(detURL, urlBad)

	hit(detHostPort, hasHostPort(rest))
	hit(detIPAddress, hasIP(rest))
	hit(detFilePath, rePosixPath.MatchString(rest) || reHomePath.MatchString(rest) || reRelPath.MatchString(rest) || reWinPath.MatchString(rest))
	hit(detDataFilename, hasDataFilename(rest))
	hit(detSecret, hasSecret(rest))
	hit(detIdentifier, hasIdentifier(rest))
	hit(detCypher, hasCypher(rest))
	hit(detParameter, reParameter.MatchString(rest))
	hit(detPastedOutput, reOutputJSON.MatchString(rest) || reOutputArr.MatchString(rest) || reOutputPipe.MatchString(rest) || reOutputErr.MatchString(rest))
	return out
}

// hasControl reports invalid UTF-8, control characters including newlines (a
// tab is allowed), and
// invisible format characters (zero-width and bidirectional marks). Newlines
// and bidi marks would let report text pose as other lines of the consent
// dialog; zero-width characters can split a detected pattern.
func hasControl(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if (unicode.IsControl(r) && r != '\t') || unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func hasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// hasHostPort matches host:port where the host contains a letter, so that
// clock times like 10:30 are not flagged (numeric hosts are caught as IPs),
// and where the host has a dot, is localhost, or the port has 4 or 5 digits,
// so that "retry:10" is not flagged either.
func hasHostPort(s string) bool {
	for _, m := range reHostPort.FindAllStringSubmatch(s, -1) {
		host := m[0][:strings.LastIndex(m[0], ":")]
		if !hasLetter(host) {
			continue
		}
		if strings.Contains(host, ".") || strings.EqualFold(host, "localhost") || len(m[1]) >= 4 {
			return true
		}
	}
	return false
}

func hasIP(s string) bool {
	for _, m := range reIPv4.FindAllString(s, -1) {
		if net.ParseIP(m) != nil {
			return true
		}
	}
	for _, loc := range reIPv6Cand.FindAllStringIndex(s, -1) {
		// Part of a longer word, as in "std::string", is not an address.
		if loc[0] > 0 && isWordByte(s[loc[0]-1]) || loc[1] < len(s) && isWordByte(s[loc[1]]) {
			continue
		}
		m := s[loc[0]:loc[1]]
		if net.ParseIP(strings.TrimRight(m, ":.")) != nil || net.ParseIP(m) != nil {
			return true
		}
	}
	return false
}

func hasDataFilename(s string) bool {
	for _, m := range reDataFile.FindAllString(s, -1) {
		if !allowedFilenames[strings.ToLower(m)] {
			return true
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// hasSecret flags known key formats, long random-looking runs, and a secret
// keyword followed by a value. With "=" any value counts (password=x is code,
// not prose); with ":" only a value-like one does, so "token: expired" is
// prose but "password: Hunter2" is not.
func hasSecret(s string) bool {
	if reSecretKey.MatchString(s) {
		return true
	}
	for _, m := range reSecretKV.FindAllStringSubmatch(s, -1) {
		if strings.Contains(m[1], "=") || valueLike(m[2]) {
			return true
		}
	}
	// A run of 20 or more letters and digits with no separator; hyphenated or
	// underscored names (x86_64-unknown-linux-gnu) are words, not tokens.
	for _, m := range reLongToken.FindAllString(s, -1) {
		if hasLetter(m) && hasDigit(m) {
			return true
		}
	}
	return false
}

// valueLike reports whether v looks like a credential rather than a word: it
// has a digit, a symbol, or a capital letter after the first character.
func valueLike(v string) bool {
	v = strings.TrimRight(v, ".,;:!?)'\"")
	for i, r := range v {
		switch {
		case unicode.IsDigit(r), !unicode.IsLetter(r):
			return true
		case i > 0 && unicode.IsUpper(r):
			return true
		}
	}
	return false
}

func hasIdentifier(s string) bool {
	if reUUID.MatchString(s) {
		return true
	}
	for _, m := range reHexRun.FindAllString(s, -1) {
		if hasLetter(m) && hasDigit(m) {
			return true
		}
	}
	return false
}

func hasCypher(s string) bool {
	for _, re := range []*regexp.Regexp{reCypherNode, reCypherRel, reCypherEdge, reCypherMap, reCypherLowerClause, reCypherLowerProp} {
		if re.MatchString(s) {
			return true
		}
	}
	// $name is a query parameter; an all-capitals $NAME is an environment
	// variable such as $NEO4J_URI.
	for _, m := range reCypherVar.FindAllString(s, -1) {
		if strings.ToUpper(m) != m {
			return true
		}
	}
	distinct := map[string]bool{}
	for _, m := range reCypherKW.FindAllString(reCypherFeature.ReplaceAllString(s, " "), -1) {
		distinct[m] = true
	}
	return len(distinct) >= 2
}
