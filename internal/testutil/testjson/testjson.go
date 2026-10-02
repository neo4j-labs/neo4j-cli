// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package testjson

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
