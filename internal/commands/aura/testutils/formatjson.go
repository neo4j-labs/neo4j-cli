// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package testutils

import (
	"bytes"
	"encoding/json"
	"strings"
)

func FormatJson(unformatted string, indent string) (string, error) {
	var pretty bytes.Buffer
	err := json.Indent(&pretty, []byte(strings.TrimSpace(unformatted)), "", indent)
	if err != nil {
		return "", err
	}
	return pretty.String() + "\n", nil
}

func UmarshalJson(data []byte) (map[string]interface{}, error) {
	var result map[string]interface{}

	err := json.Unmarshal(data, &result)
	return result, err
}

func MarshalJson(v map[string]interface{}) (string, error) {
	data, err := json.Marshal(v)

	return string(data), err
}
