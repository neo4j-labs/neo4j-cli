// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"encoding/json"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists the current configuration of the Aura CLI subcommand",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if cfg.Global.Output() == "table" {
				printAuraConfigTable(cmd, cfg)
			} else {
				cfg.Aura.PrintAuraConfig(cmd)
			}
		},
	}
}

func printAuraConfigTable(cmd *cobra.Command, cfg *clicfg.Config) {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"key", "value"})
	for _, key := range cfg.Aura.ValidConfigKeys {
		value := cfg.Aura.Get(key)
		var displayValue string
		if value == nil {
			displayValue = ""
		} else {
			displayValue = fmt.Sprintf("%v", value)
		}
		t.AppendRow(table.Row{key, displayValue})
	}
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}

func printConfigKeyValueAsJSON(cmd *cobra.Command, key string, value interface{}) {
	m := map[string]interface{}{key: value}
	bytes, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		panic(err)
	}
	cmd.Println(string(bytes))
}

func printConfigKeyValueAsTable(cmd *cobra.Command, key string, value interface{}) {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"key", "value"})
	var displayValue string
	if value == nil {
		displayValue = ""
	} else {
		displayValue = fmt.Sprintf("%v", value)
	}
	t.AppendRow(table.Row{key, displayValue})
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}
