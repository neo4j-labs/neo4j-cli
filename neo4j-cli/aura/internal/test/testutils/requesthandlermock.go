// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package testutils

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

type call struct {
	Method      string
	Path        string
	Body        map[string]interface{}
	QueryParams url.Values
}

type response struct {
	body    string
	status  int
	headers map[string]string
}

type requestHandlerMock struct {
	Calls     []call
	Responses []response
	t         *testing.T
}

func (mock *requestHandlerMock) AddResponse(status int, body string) *requestHandlerMock {
	mock.Responses = append(mock.Responses, response{
		body:   body,
		status: status,
	})

	return mock
}

// WithResponseHeader sets an HTTP response header on the most-recently-added
// response. The configured headers are written before WriteHeader when the
// response is served.
func (mock *requestHandlerMock) WithResponseHeader(key, value string) *requestHandlerMock {
	last := len(mock.Responses) - 1
	if last < 0 {
		return mock
	}
	if mock.Responses[last].headers == nil {
		mock.Responses[last].headers = map[string]string{}
	}
	mock.Responses[last].headers[key] = value

	return mock
}

func (mock *requestHandlerMock) AssertCalledTimes(times int) {
	calls := len(mock.Calls)

	assert.Equal(mock.t, times, calls, "Request handler mock not called the expected number of times")
}

func (mock *requestHandlerMock) AssertCalledWithMethod(method string) {
	methods := ""

	for _, call := range mock.Calls {
		if call.Method == method {
			return
		}

		methods += call.Method
	}

	assert.Fail(mock.t, fmt.Sprintf("Handler not called with method:\nexpected: %s, actual: %s", method, methods))
}

// CalledWithMethod reports whether the handler was invoked at least once with
// the given HTTP method. Used by confirmtest.AssertLeafGate to detect whether
// the destructive sink (DELETE) fired during a gating scenario.
func (mock *requestHandlerMock) CalledWithMethod(method string) bool {
	for _, call := range mock.Calls {
		if call.Method == method {
			return true
		}
	}
	return false
}

func (mock *requestHandlerMock) AssertCalledWithQueryParam(param string, value string) {
	for _, call := range mock.Calls {
		if call.QueryParams.Has(param) && call.QueryParams.Get(param) == value {
			return
		}
	}

	assert.Fail(mock.t, fmt.Sprintf("Handler not called with query param:\nexpected: %s:%s", param, value))
}

func (mock *requestHandlerMock) AssertCalledWithBody(body string) {
	unmarshalled, err := UmarshalJson([]byte(body))
	assert.Nil(mock.t, err)

	bodies := ""

	for _, call := range mock.Calls {
		if cmp.Equal(call.Body, unmarshalled) {
			return
		}
		data, err := MarshalJson(call.Body)
		assert.Nil(mock.t, err)

		bodies += data + "\n"
	}

	assert.Fail(mock.t, fmt.Sprintf("Handler not called with body:\nexpected: %s\nactual: %s", body, bodies))
}
