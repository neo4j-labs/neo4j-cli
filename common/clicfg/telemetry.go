// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import "github.com/spf13/viper"

// shouldDisableTelemetry returns true when telemetry emission must be disabled,
// either because the persisted "telemetry" config key is false or because the
// caller's environment has DO_NOT_TRACK set to the literal string "1".
//
// Only the literal "1" disables via env — values like "0", "", "true", or "yes"
// keep telemetry enabled. The env-var lever wins over the persisted config:
// if DO_NOT_TRACK=1, telemetry is disabled regardless of the config value.
func shouldDisableTelemetry(v *viper.Viper, getenv func(string) string) bool {
	if !v.GetBool("telemetry") {
		return true
	}
	if getenv("DO_NOT_TRACK") == "1" {
		return true
	}
	return false
}
