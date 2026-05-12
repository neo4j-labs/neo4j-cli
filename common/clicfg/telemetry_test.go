// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestShouldDisableTelemetry(t *testing.T) {
	tests := []struct {
		name              string
		telemetryValue    interface{} // nil = unset (default applies)
		envValue          string
		envSet            bool
		wantDisableResult bool
	}{
		{
			name:              "default (telemetry unset, no env) keeps telemetry enabled",
			telemetryValue:    nil,
			envSet:            false,
			wantDisableResult: false,
		},
		{
			name:              "telemetry=true, no env: enabled",
			telemetryValue:    true,
			envSet:            false,
			wantDisableResult: false,
		},
		{
			name:              "telemetry=false, no env: disabled",
			telemetryValue:    false,
			envSet:            false,
			wantDisableResult: true,
		},
		{
			name:              `telemetry=true, DO_NOT_TRACK="1": disabled (env wins)`,
			telemetryValue:    true,
			envValue:          "1",
			envSet:            true,
			wantDisableResult: true,
		},
		{
			name:              `telemetry=true, DO_NOT_TRACK="0": enabled (literal "1" only)`,
			telemetryValue:    true,
			envValue:          "0",
			envSet:            true,
			wantDisableResult: false,
		},
		{
			name:              `telemetry=true, DO_NOT_TRACK="": enabled`,
			telemetryValue:    true,
			envValue:          "",
			envSet:            true,
			wantDisableResult: false,
		},
		{
			name:              `telemetry=true, DO_NOT_TRACK="true": enabled (literal "1" only)`,
			telemetryValue:    true,
			envValue:          "true",
			envSet:            true,
			wantDisableResult: false,
		},
		{
			name:              `telemetry=false, DO_NOT_TRACK="1": disabled`,
			telemetryValue:    false,
			envValue:          "1",
			envSet:            true,
			wantDisableResult: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()
			v.SetDefault("telemetry", true)
			if tc.telemetryValue != nil {
				v.Set("telemetry", tc.telemetryValue)
			}

			getenv := func(key string) string {
				if key == "DO_NOT_TRACK" && tc.envSet {
					return tc.envValue
				}
				return ""
			}

			got := shouldDisableTelemetry(v, getenv)
			assert.Equal(t, tc.wantDisableResult, got)
		})
	}
}
