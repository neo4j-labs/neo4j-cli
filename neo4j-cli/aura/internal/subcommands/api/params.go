// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// Flag names the request-shaping parsers report in their usage errors.
const (
	flagMethod   = "method"
	flagField    = "field"
	flagRawField = "raw-field"
	flagInput    = "input"
	flagHeader   = "header"
)

// allowedMethods is the accepted --method set, in the order the usage error
// lists them.
var allowedMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// queryFieldMethods are the methods whose fields travel in the query string
// instead of a request body. The v2beta1 spec puts most of its filtering and
// paging options (include_deleted, page_limit, …) on GET operations, so this
// mapping is what makes those reachable at all.
var queryFieldMethods = []string{http.MethodGet, http.MethodHead, http.MethodDelete}

// requestFlags holds the raw values of the flags that shape the outgoing
// request. An empty method means --method was not passed, which is what lets a
// body-bearing flag infer POST.
type requestFlags struct {
	method    string
	fields    []string
	rawFields []string
	input     string
	headers   []string
}

// registerRequestFlags declares the request-shaping flags on cmd and binds them
// to f, so their names, shorthands, and defaults stay next to the parsing they
// drive. --method deliberately defaults to the empty string rather than "GET":
// registering "GET" as the default would make an explicit `--method GET` with a
// field turn into a POST.
func registerRequestFlags(cmd *cobra.Command, f *requestFlags) {
	cmd.Flags().StringVarP(&f.method, flagMethod, "X", "",
		fmt.Sprintf("HTTP method to use, from a choice of [%s]. Defaults to GET, or to POST when --field, --raw-field or --input is passed.", strings.Join(allowedMethods, ", ")))
	cmd.Flags().StringArrayVarP(&f.fields, flagField, "F", nil,
		"Repeatable key=value pair. 'true', 'false', 'null' and integers become JSON literals, '@<file>' ('@-' for stdin) reads the value from a file, anything else is a string. Sent as query parameters for GET, HEAD and DELETE, and as a JSON body otherwise.")
	cmd.Flags().StringArrayVarP(&f.rawFields, flagRawField, "f", nil,
		"Repeatable key=value pair whose value is always a string.")
	cmd.Flags().StringVar(&f.input, flagInput, "",
		"File holding the request body, sent verbatim; '-' reads it from stdin. Cannot be combined with --field or --raw-field.")
	cmd.Flags().StringArrayVarP(&f.headers, flagHeader, "H", nil,
		"Repeatable 'Name: value' request header, overlaid on the generated headers.")
}

// builtRequest is the request shape a requestFlags set describes. query is
// always non-nil so a caller can merge the endpoint's inline query into it.
type builtRequest struct {
	method  string
	body    []byte
	query   url.Values
	headers http.Header
}

// buildRequest resolves the HTTP method and turns the field, body, and header
// flags into the pieces of an Aura request.
//
// Fields become query parameters for GET, HEAD, and DELETE and a JSON object
// body for every other method; --input replaces that body with a verbatim
// document, so the two are mutually exclusive.
func buildRequest(cmd *cobra.Command, cfg *clicfg.Config, f *requestFlags) (*builtRequest, error) {
	method, err := resolveRequestMethod(f)
	if err != nil {
		return nil, err
	}

	headers, err := parseHeaders(f.headers)
	if err != nil {
		return nil, err
	}

	built := &builtRequest{method: method, query: url.Values{}, headers: headers}
	reader := &payloadReader{cmd: cmd, cfg: cfg}

	if f.input != "" {
		body, err := reader.read(flagInput, f.input)
		if err != nil {
			return nil, err
		}
		built.body = body
		return built, nil
	}

	fields, err := parseFields(reader, f.fields, f.rawFields)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return built, nil
	}

	if slices.Contains(queryFieldMethods, method) {
		for key, value := range fields {
			built.query.Set(key, fieldQueryValue(value))
		}
		return built, nil
	}

	body, err := json.Marshal(fields)
	if err != nil {
		return nil, clierr.NewUsageError("could not encode the --%s values as a JSON body: %s", flagField, err.Error())
	}
	built.body = body

	return built, nil
}

// resolveRequestMethod resolves the HTTP method from the flag values alone,
// touching neither the filesystem nor stdin. It is deliberately callable on its
// own and idempotent: the caller resolves the method up front so the --rw and
// confirm gates run before any payload is read, since a `--field key=@-` would
// otherwise drain the stdin the confirm prompt needs to read an answer from.
func resolveRequestMethod(f *requestFlags) (string, error) {
	hasFields := len(f.fields) > 0 || len(f.rawFields) > 0
	hasInput := f.input != ""

	if hasFields && hasInput {
		return "", clierr.NewUsageError("--%s cannot be combined with --%s or --%s", flagInput, flagField, flagRawField).
			WithSuggestion("Put every value in the --input document, or drop --input and pass each value as --field/--raw-field.")
	}

	return resolveMethod(f.method, hasFields || hasInput)
}

// resolveMethod upper-cases and validates the method, defaulting to GET and
// inferring POST when a body-bearing flag was passed without a --method
// (matching `gh api`). An explicit --method always wins, so
// `--method GET --field include_deleted=true` still sends a GET.
func resolveMethod(raw string, hasPayload bool) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(raw))
	if method == "" {
		if hasPayload {
			return http.MethodPost, nil
		}
		return http.MethodGet, nil
	}

	if !slices.Contains(allowedMethods, method) {
		return "", clierr.NewUsageError("unsupported --%s %q", flagMethod, raw).
			WithSuggestion(fmt.Sprintf("Supported methods are %s.", strings.Join(allowedMethods, ", ")))
	}

	return method, nil
}

// parseFields merges the typed --field and the always-string --raw-field
// entries into one set. Raw entries are applied last, so a key given to both
// keeps its raw string value.
func parseFields(reader *payloadReader, typed, raw []string) (map[string]any, error) {
	fields := make(map[string]any, len(typed)+len(raw))

	for _, entry := range typed {
		key, value, err := splitField(flagField, entry)
		if err != nil {
			return nil, err
		}
		converted, err := typedFieldValue(reader, key, value)
		if err != nil {
			return nil, err
		}
		fields[key] = converted
	}

	for _, entry := range raw {
		key, value, err := splitField(flagRawField, entry)
		if err != nil {
			return nil, err
		}
		fields[key] = value
	}

	return fields, nil
}

// splitField splits one field entry on its first "=". Only the key is ever
// quoted back in an error: the value may be a secret, and an error message
// reaches both stderr and the on-disk tee file, neither of which redacts a value
// whose key does not look secret-shaped.
func splitField(flag, entry string) (string, string, error) {
	key, value, found := strings.Cut(entry, "=")
	if !found {
		return "", "", clierr.NewUsageError("invalid --%s %q: expected key=value", flag, entry)
	}
	if key == "" {
		return "", "", clierr.NewUsageError("invalid --%s entry: empty key", flag)
	}
	return key, value, nil
}

// typedFieldValue applies --field's type inference: an "@" prefix reads the
// value from that file ("@-" from stdin) as a string, "true"/"false"/"null" and
// integer-looking values become JSON literals, and everything else stays a
// string.
//
// Inference is textual, so a numeric-looking string loses its leading zeros
// ("0123" becomes 123) and a decimal ("1.5") stays a string; --raw-field is the
// escape hatch for both.
func typedFieldValue(reader *payloadReader, key, value string) (any, error) {
	if source, isFile := strings.CutPrefix(value, "@"); isFile {
		if source == "" {
			return nil, clierr.NewUsageError("invalid --%s %q: expected @<file>, or @- to read stdin", flagField, key)
		}
		contents, err := reader.read(flagField, source)
		if err != nil {
			return nil, err
		}
		return string(contents), nil
	}

	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}

	if number, err := strconv.ParseInt(value, 10, 64); err == nil {
		return number, nil
	}

	return value, nil
}

// fieldQueryValue renders a parsed field value as a query parameter.
func fieldQueryValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	}

	// nil, the only other shape a field can hold. A query string cannot express
	// it, so it becomes an empty value.
	return ""
}

// payloadReader reads the file and stdin payloads referenced by --input and
// --field. It refuses to consume stdin twice: the second read would silently
// yield an empty value rather than failing.
type payloadReader struct {
	cmd       *cobra.Command
	cfg       *clicfg.Config
	stdinRead bool
}

// read returns the contents of source verbatim, treating "-" as stdin, so any
// JSON shape including a top-level array survives.
func (r *payloadReader) read(flag, source string) ([]byte, error) {
	if source == "-" {
		if r.stdinRead {
			return nil, clierr.NewUsageError("stdin can only be read once, but --%s reads it again", flag).
				WithSuggestion("Read stdin for a single value and pass the others as literals or files.")
		}
		r.stdinRead = true

		contents, err := io.ReadAll(r.cmd.InOrStdin())
		if err != nil {
			return nil, clierr.NewUsageError("could not read --%s from stdin: %s", flag, err.Error())
		}
		return contents, nil
	}

	contents, err := afero.ReadFile(r.cfg.Aura.Fs(), source)
	if err != nil {
		return nil, clierr.NewUsageError("could not read --%s file %q: %s", flag, source, err.Error())
	}
	return contents, nil
}

// headerNameSpecials are the non-alphanumeric characters RFC 7230 allows in a
// header name.
const headerNameSpecials = "!#$%&'*+-.^_`|~"

// parseHeaders turns repeatable "Name: value" entries into a header set built
// through Add, so each name settles on one canonical spelling — the api
// package's header overlay picks an arbitrary winner otherwise.
//
// Only the header name is quoted back in an error, never the value: an entry may
// carry a bearer token, and an error message reaches both stderr and the on-disk
// tee file, neither of which redacts a value whose name does not look
// secret-shaped.
func parseHeaders(entries []string) (http.Header, error) {
	header := http.Header{}

	for _, entry := range entries {
		name, value, found := strings.Cut(entry, ":")
		if !found {
			return nil, clierr.NewUsageError("invalid --%s %q: expected 'Name: value'", flagHeader, firstWord(entry))
		}
		if !validHeaderName(name) {
			return nil, clierr.NewUsageError("invalid --%s: %q is not a valid header name", flagHeader, name)
		}

		value = strings.Trim(value, " \t")
		if !validHeaderValue(value) {
			return nil, clierr.NewUsageError("invalid --%s %q: value must not contain control characters, including a carriage return or a newline", flagHeader, name)
		}

		header.Add(name, value)
	}

	return header, nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(headerNameSpecials, r):
		default:
			return false
		}
	}
	return true
}

// validHeaderValue applies the same rule as net/http, which rejects a control
// character other than tab. Screening it here keeps a bad -H a usage error
// instead of a transport failure reported as a retryable upstream error.
func validHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if b := value[i]; b != '\t' && (b < ' ' || b == 0x7f) {
			return false
		}
	}
	return true
}

// firstWord identifies a malformed entry without echoing what follows the first
// space, which in "Authorization Bearer <token>" is a credential.
func firstWord(entry string) string {
	word, _, _ := strings.Cut(strings.TrimSpace(entry), " ")
	return word
}
