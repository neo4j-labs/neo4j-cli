// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMethod(t *testing.T) {
	for _, tt := range []struct {
		name       string
		raw        string
		hasPayload bool
		want       string
	}{
		{name: "unset is GET", raw: "", want: http.MethodGet},
		{name: "whitespace only is unset", raw: "  ", want: http.MethodGet},
		{name: "payload infers POST", raw: "", hasPayload: true, want: http.MethodPost},
		{name: "explicit GET beats inference", raw: "get", hasPayload: true, want: http.MethodGet},
		{name: "explicit DELETE beats inference", raw: "delete", hasPayload: true, want: http.MethodDelete},
		{name: "lower case is upper cased", raw: "post", want: http.MethodPost},
		{name: "mixed case is upper cased", raw: "PaTcH", want: http.MethodPatch},
		{name: "surrounding whitespace trimmed", raw: "  put  ", want: http.MethodPut},
		{name: "head allowed", raw: "head", want: http.MethodHead},
		{name: "options allowed", raw: "options", want: http.MethodOptions},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMethod(tt.raw, tt.hasPayload)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveMethod_UsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
	}{
		{name: "off allowlist", raw: "TRACE"},
		{name: "connect rejected", raw: "connect"},
		{name: "not a method", raw: "instances"},
		{name: "method with a space", raw: "GET POST"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMethod(tt.raw, false)
			assert.Empty(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported --method")

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 2, ce.Code)
		})
	}
}

func TestBuildRequest_Fields(t *testing.T) {
	for _, tt := range []struct {
		name       string
		flags      requestFlags
		wantMethod string
		wantBody   string
		wantQuery  url.Values
	}{
		{
			name:       "no flags is a bare GET",
			flags:      requestFlags{},
			wantMethod: http.MethodGet,
			wantQuery:  url.Values{},
		},
		{
			name: "typed fields become a JSON body on the inferred POST",
			flags: requestFlags{
				fields: []string{"name=my-db", "memory=2GB"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"memory":"2GB","name":"my-db"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "type inference covers literals and integers",
			flags: requestFlags{
				fields: []string{"yes=true", "no=false", "nothing=null", "count=10", "below=-3"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"below":-3,"count":10,"no":false,"nothing":null,"yes":true}`,
			wantQuery:  url.Values{},
		},
		{
			name: "non integer numerics and empty values stay strings",
			flags: requestFlags{
				fields: []string{"ratio=1.5", "version=1.2.3", "padded=0123", "empty="},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"empty":"","padded":123,"ratio":"1.5","version":"1.2.3"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "raw fields are always strings",
			flags: requestFlags{
				rawFields: []string{"yes=true", "count=10", "nothing=null", "at=@payload.json"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"at":"@payload.json","count":"10","nothing":"null","yes":"true"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "a value may contain further equals signs",
			flags: requestFlags{
				rawFields: []string{"filter=a=b=c"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"filter":"a=b=c"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "raw field wins over a typed field with the same key",
			flags: requestFlags{
				fields:    []string{"count=10"},
				rawFields: []string{"count=10"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"count":"10"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "the last of a repeated key wins",
			flags: requestFlags{
				fields: []string{"count=1", "count=2"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"count":2}`,
			wantQuery:  url.Values{},
		},
		{
			name: "an at sign is only special as a prefix",
			flags: requestFlags{
				fields: []string{"email=someone@example.com"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"email":"someone@example.com"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "an integer beyond int64 stays a string",
			flags: requestFlags{
				fields: []string{"big=99999999999999999999"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"big":"99999999999999999999"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "file field reads its value verbatim",
			flags: requestFlags{
				fields: []string{"query=@" + testPayloadFile},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"query":"MATCH (n) RETURN n\n"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "stdin field reads its value verbatim",
			flags: requestFlags{
				fields: []string{"query=@-"},
			},
			wantMethod: http.MethodPost,
			wantBody:   `{"query":"piped text"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "fields are query params on an explicit GET",
			flags: requestFlags{
				method: "get",
				fields: []string{"include_deleted=true", "page_limit=10", "nothing=null", "name=my db"},
			},
			wantMethod: http.MethodGet,
			wantQuery: url.Values{
				"include_deleted": []string{"true"},
				"page_limit":      []string{"10"},
				"nothing":         []string{""},
				"name":            []string{"my db"},
			},
		},
		{
			name: "fields are query params on HEAD",
			flags: requestFlags{
				method: "head",
				fields: []string{"include_deleted=false"},
			},
			wantMethod: http.MethodHead,
			wantQuery:  url.Values{"include_deleted": []string{"false"}},
		},
		{
			name: "fields are query params on DELETE",
			flags: requestFlags{
				method:    "delete",
				rawFields: []string{"database_username=neo4j"},
			},
			wantMethod: http.MethodDelete,
			wantQuery:  url.Values{"database_username": []string{"neo4j"}},
		},
		{
			name: "fields are a body on PUT",
			flags: requestFlags{
				method: "put",
				fields: []string{"memory=4GB"},
			},
			wantMethod: http.MethodPut,
			wantBody:   `{"memory":"4GB"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "fields are a body on PATCH",
			flags: requestFlags{
				method: "patch",
				fields: []string{"memory=4GB"},
			},
			wantMethod: http.MethodPatch,
			wantBody:   `{"memory":"4GB"}`,
			wantQuery:  url.Values{},
		},
		{
			name: "fields are a body on OPTIONS",
			flags: requestFlags{
				method: "options",
				fields: []string{"memory=4GB"},
			},
			wantMethod: http.MethodOptions,
			wantBody:   `{"memory":"4GB"}`,
			wantQuery:  url.Values{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newParamsTestConfig(t, "MATCH (n) RETURN n\n")
			cmd := newParamsTestCmd("piped text")

			got, err := buildRequest(cmd, cfg, &tt.flags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMethod, got.method)
			assert.Equal(t, tt.wantQuery, got.query)

			if tt.wantBody == "" {
				assert.Nil(t, got.body)
				return
			}
			assert.Equal(t, tt.wantBody, string(got.body))
		})
	}
}

func TestBuildRequest_Input(t *testing.T) {
	for _, tt := range []struct {
		name       string
		payload    string
		stdin      string
		flags      requestFlags
		wantMethod string
		wantBody   string
	}{
		{
			name:       "file body infers POST",
			payload:    `{"name":"my-db"}`,
			flags:      requestFlags{input: testPayloadFile},
			wantMethod: http.MethodPost,
			wantBody:   `{"name":"my-db"}`,
		},
		{
			name:       "stdin body infers POST",
			stdin:      `{"name":"piped"}`,
			flags:      requestFlags{input: "-"},
			wantMethod: http.MethodPost,
			wantBody:   `{"name":"piped"}`,
		},
		{
			name:       "explicit method beats inference",
			payload:    `{"memory":"4GB"}`,
			flags:      requestFlags{method: "patch", input: testPayloadFile},
			wantMethod: http.MethodPatch,
			wantBody:   `{"memory":"4GB"}`,
		},
		{
			name:       "top level array survives verbatim",
			payload:    "[{\"op\":\"add\"},\n {\"op\":\"remove\"}]",
			flags:      requestFlags{input: testPayloadFile},
			wantMethod: http.MethodPost,
			wantBody:   "[{\"op\":\"add\"},\n {\"op\":\"remove\"}]",
		},
		{
			name:       "body is not reformatted or validated",
			payload:    "  not json at all  ",
			flags:      requestFlags{input: testPayloadFile},
			wantMethod: http.MethodPost,
			wantBody:   "  not json at all  ",
		},
		{
			name:       "body may accompany a GET",
			payload:    `{"name":"my-db"}`,
			flags:      requestFlags{method: "get", input: testPayloadFile},
			wantMethod: http.MethodGet,
			wantBody:   `{"name":"my-db"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newParamsTestConfig(t, tt.payload)
			cmd := newParamsTestCmd(tt.stdin)

			got, err := buildRequest(cmd, cfg, &tt.flags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMethod, got.method)
			assert.Equal(t, tt.wantBody, string(got.body))
			assert.Equal(t, url.Values{}, got.query)
		})
	}
}

func TestBuildRequest_UsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		flags    requestFlags
		contains string
		// absent is text the message must not carry, so a value that may be a
		// secret cannot creep back into it.
		absent string
	}{
		{
			name:     "input with a typed field",
			flags:    requestFlags{input: testPayloadFile, fields: []string{"name=my-db"}},
			contains: "--input cannot be combined with --field or --raw-field",
		},
		{
			name:     "input with a raw field",
			flags:    requestFlags{input: testPayloadFile, rawFields: []string{"name=my-db"}},
			contains: "--input cannot be combined with --field or --raw-field",
		},
		{
			name:     "unsupported method",
			flags:    requestFlags{method: "TRACE"},
			contains: "unsupported --method",
		},
		{
			name:     "field without an equals sign",
			flags:    requestFlags{fields: []string{"name"}},
			contains: `invalid --field "name": expected key=value`,
		},
		{
			name:     "field with an empty key does not echo its value",
			flags:    requestFlags{fields: []string{"=s3cret"}},
			contains: "invalid --field entry: empty key",
			absent:   "s3cret",
		},
		{
			name:     "raw field without an equals sign",
			flags:    requestFlags{rawFields: []string{"name"}},
			contains: `invalid --raw-field "name": expected key=value`,
		},
		{
			name:     "raw field with an empty key does not echo its value",
			flags:    requestFlags{rawFields: []string{"=s3cret"}},
			contains: "invalid --raw-field entry: empty key",
			absent:   "s3cret",
		},
		{
			name:     "field file reference without a path",
			flags:    requestFlags{fields: []string{"query=@"}},
			contains: "expected @<file>, or @- to read stdin",
		},
		{
			name:     "field file that does not exist",
			flags:    requestFlags{fields: []string{"query=@missing.json"}},
			contains: `could not read --field file "missing.json"`,
		},
		{
			name:     "input file that does not exist",
			flags:    requestFlags{input: "missing.json"},
			contains: `could not read --input file "missing.json"`,
		},
		{
			name:     "stdin read twice",
			flags:    requestFlags{fields: []string{"a=@-", "b=@-"}},
			contains: "stdin can only be read once",
		},
		{
			name:     "header without a colon does not echo what follows the name",
			flags:    requestFlags{headers: []string{"Authorization Bearer s3cret"}},
			contains: `invalid --header "Authorization": expected 'Name: value'`,
			absent:   "s3cret",
		},
		{
			name:     "header value with a control character does not echo the value",
			flags:    requestFlags{headers: []string{"X-Session: s3cret\x00"}},
			contains: `invalid --header "X-Session": value must not contain control characters`,
			absent:   "s3cret",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newParamsTestConfig(t, "{}")
			cmd := newParamsTestCmd("piped text")

			got, err := buildRequest(cmd, cfg, &tt.flags)
			assert.Nil(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
			if tt.absent != "" {
				assert.NotContains(t, err.Error(), tt.absent)
			}

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 2, ce.Code)
		})
	}
}

func TestParseHeaders(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []string
		want    http.Header
	}{
		{
			name: "no entries",
			want: http.Header{},
		},
		{
			name:    "single header",
			entries: []string{"Accept: application/json"},
			want:    http.Header{"Accept": []string{"application/json"}},
		},
		{
			name:    "no space after the colon",
			entries: []string{"Accept:application/json"},
			want:    http.Header{"Accept": []string{"application/json"}},
		},
		{
			name:    "surrounding whitespace is not part of the value",
			entries: []string{"Accept: \tapplication/json \t"},
			want:    http.Header{"Accept": []string{"application/json"}},
		},
		{
			name:    "empty value allowed",
			entries: []string{"Accept:"},
			want:    http.Header{"Accept": []string{""}},
		},
		{
			name:    "an inner tab is part of the value",
			entries: []string{"X-Tag: a\tb"},
			want:    http.Header{"X-Tag": []string{"a\tb"}},
		},
		{
			name:    "a non ascii value is allowed",
			entries: []string{"X-Tag: café"},
			want:    http.Header{"X-Tag": []string{"café"}},
		},
		{
			name:    "value may contain colons",
			entries: []string{"X-Trace: a:b:c"},
			want:    http.Header{"X-Trace": []string{"a:b:c"}},
		},
		{
			name:    "several headers",
			entries: []string{"Accept: application/json", "X-Request-Id: abc"},
			want: http.Header{
				"Accept":       []string{"application/json"},
				"X-Request-Id": []string{"abc"},
			},
		},
		{
			name:    "repeated name keeps both values",
			entries: []string{"X-Tag: a", "X-Tag: b"},
			want:    http.Header{"X-Tag": []string{"a", "b"}},
		},
		{
			name:    "differing spellings collapse onto one canonical key",
			entries: []string{"accept: application/json", "ACCEPT: text/plain"},
			want:    http.Header{"Accept": []string{"application/json", "text/plain"}},
		},
		{
			name:    "token characters allowed in a name, which is canonicalized",
			entries: []string{"X-Weird_Name.1: ok"},
			want:    http.Header{"X-Weird_name.1": []string{"ok"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHeaders(tt.entries)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseHeaders_UsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		entry    string
		contains string
	}{
		{name: "no colon", entry: "Accept application/json", contains: "expected 'Name: value'"},
		{name: "empty", entry: "", contains: "expected 'Name: value'"},
		{name: "empty name", entry: ": application/json", contains: "is not a valid header name"},
		{name: "space in name", entry: "X Trace: abc", contains: "is not a valid header name"},
		{name: "trailing space in name", entry: "Accept : application/json", contains: "is not a valid header name"},
		{name: "non token character in name", entry: "X-Tr@ce: abc", contains: "is not a valid header name"},
		{name: "newline in value", entry: "X-Tag: a\nX-Injected: b", contains: "must not contain control characters"},
		{name: "carriage return in value", entry: "X-Tag: a\r\nX-Injected: b", contains: "must not contain control characters"},
		{name: "nul in value", entry: "X-Tag: a\x00b", contains: "must not contain control characters"},
		{name: "escape in value", entry: "X-Tag: a\x1b[31mb", contains: "must not contain control characters"},
		{name: "delete in value", entry: "X-Tag: a\x7fb", contains: "must not contain control characters"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHeaders([]string{tt.entry})
			assert.Nil(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 2, ce.Code)
		})
	}
}

func TestBuildRequest_Headers(t *testing.T) {
	cfg := newParamsTestConfig(t, "{}")
	cmd := newParamsTestCmd("")

	got, err := buildRequest(cmd, cfg, &requestFlags{
		headers: []string{"Accept: application/json", "X-Tag: a", "x-tag: b"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.Header{
		"Accept": []string{"application/json"},
		"X-Tag":  []string{"a", "b"},
	}, got.headers)
}

// TestRegisterRequestFlags drives buildRequest through real parsed argv, which
// is what pins the flag defaults to the inference rules: registering --method
// with a "GET" default would silently turn `--method GET --field x=1` into a
// POST, and no requestFlags literal could catch that.
func TestRegisterRequestFlags(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		wantMethod  string
		wantBody    string
		wantQuery   url.Values
		wantHeaders http.Header
	}{
		{
			name:       "no flags is a GET",
			wantMethod: http.MethodGet,
			wantQuery:  url.Values{},
		},
		{
			name:       "a field infers POST",
			args:       []string{"--field", "name=my-db"},
			wantMethod: http.MethodPost,
			wantBody:   `{"name":"my-db"}`,
			wantQuery:  url.Values{},
		},
		{
			name:       "an explicit GET keeps its method and takes query params",
			args:       []string{"--method", "GET", "--field", "include_deleted=true"},
			wantMethod: http.MethodGet,
			wantQuery:  url.Values{"include_deleted": []string{"true"}},
		},
		{
			name:       "an input body infers POST",
			args:       []string{"--input", testPayloadFile},
			wantMethod: http.MethodPost,
			wantBody:   `{"seeded":true}`,
			wantQuery:  url.Values{},
		},
		{
			name:        "shorthands",
			args:        []string{"-X", "patch", "-F", "count=2", "-H", "Accept: text/plain"},
			wantMethod:  http.MethodPatch,
			wantBody:    `{"count":2}`,
			wantQuery:   url.Values{},
			wantHeaders: http.Header{"Accept": []string{"text/plain"}},
		},
		{
			name:       "raw field shorthand",
			args:       []string{"-f", "count=2"},
			wantMethod: http.MethodPost,
			wantBody:   `{"count":"2"}`,
			wantQuery:  url.Values{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newParamsTestConfig(t, `{"seeded":true}`)

			var flags requestFlags
			var got *builtRequest

			cmd := newParamsTestCmd("")
			registerRequestFlags(cmd, &flags)
			cmd.RunE = func(cmd *cobra.Command, _ []string) error {
				var err error
				got, err = buildRequest(cmd, cfg, &flags)
				return err
			}
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())

			require.NotNil(t, got)
			assert.Equal(t, tt.wantMethod, got.method)
			assert.Equal(t, tt.wantBody, string(got.body))
			assert.Equal(t, tt.wantQuery, got.query)

			wantHeaders := tt.wantHeaders
			if wantHeaders == nil {
				wantHeaders = http.Header{}
			}
			assert.Equal(t, wantHeaders, got.headers)
		})
	}
}

// TestRegisterRequestFlags_Shorthands pins the shorthands clievents.RedactArgs
// already scrubs by name, so renaming one here cannot silently stop redacting
// the values it carries.
func TestRegisterRequestFlags_Shorthands(t *testing.T) {
	cmd := &cobra.Command{Use: "api"}
	registerRequestFlags(cmd, &requestFlags{})

	for name, shorthand := range map[string]string{
		flagMethod:   "X",
		flagField:    "F",
		flagRawField: "f",
		flagHeader:   "H",
		flagInput:    "",
	} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, name)
		assert.Equal(t, shorthand, flag.Shorthand, name)
	}
}
