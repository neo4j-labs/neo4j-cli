// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd_test

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestLeaves_WriteAnnotations pins the --rw enforcement surface: every write
// leaf is annotated write=true, the read leaf is not.
func TestLeaves_WriteAnnotations(t *testing.T) {
	spec := baseSpec(t)
	for _, leaf := range []string{"create", "update", "delete"} {
		if got := findLeaf(t, spec, leaf).Annotations["write"]; got != "true" {
			t.Fatalf("%s must be annotated write=true; got %v", leaf, got)
		}
	}
	if got := findLeaf(t, spec, "list").Annotations["write"]; got != "" {
		t.Fatalf("list must not be write-annotated; got %q", got)
	}
}

// TestLeaves_UseStrings pins the generated Use strings (the positional is
// composed from the spec noun).
func TestLeaves_UseStrings(t *testing.T) {
	spec := baseSpec(t)
	want := map[string]string{
		"list":   "list",
		"create": "create <name>",
		"update": "update <widget>",
		"delete": "delete <widget>",
	}
	for leaf, use := range want {
		if got := findLeaf(t, spec, leaf).Use; got != use {
			t.Fatalf("%s Use = %q, want %q", leaf, got, use)
		}
	}
}

// TestLeaves_Examples mirrors the whole-tree example gate: every generated
// leaf carries a flush-left Example with >=2 invocations; writes carry --rw
// and every leaf shows --format json.
func TestLeaves_Examples(t *testing.T) {
	spec := baseSpec(t)
	for _, leaf := range []string{"list", "create", "update", "delete"} {
		cmd := findLeaf(t, spec, leaf)
		if cmd.Example == "" {
			t.Fatalf("%s Example must be non-empty", leaf)
		}
		firstLine := strings.SplitN(cmd.Example, "\n", 2)[0]
		if strings.HasPrefix(firstLine, "  ") {
			t.Fatalf("%s Example first line must be flush-left; got %q", leaf, firstLine)
		}
		if c := strings.Count(cmd.Example, "neo4j-cli desktop widget "+leaf); c < 2 {
			t.Fatalf("%s Example must contain >=2 invocations; got %d", leaf, c)
		}
		if !strings.Contains(cmd.Example, "--format json") {
			t.Fatalf("%s Example must contain --format json; got:\n%s", leaf, cmd.Example)
		}
		if leaf != "list" && !strings.Contains(cmd.Example, "--rw") {
			t.Fatalf("%s Example must contain --rw; got:\n%s", leaf, cmd.Example)
		}
	}
}

// TestUpdate_NameFlagRequired pins the flag surface: --name exists, is marked
// required, and its usage string is composed from the spec noun.
func TestUpdate_NameFlagRequired(t *testing.T) {
	spec := baseSpec(t)
	cmd := findLeaf(t, spec, "update")
	flag := cmd.LocalFlags().Lookup("name")
	if flag == nil {
		t.Fatalf("update must register --name")
	}
	if flag.Usage != "(required) New name for the widget" {
		t.Fatalf("--name usage = %q", flag.Usage)
	}
	required := flag.Annotations[cobra.BashCompOneRequiredFlag]
	if len(required) == 0 || required[0] != "true" {
		t.Fatalf("--name must be marked required; annotations: %v", flag.Annotations)
	}
}

// TestColorSpec_MountsColorFlag pins that --color appears on create/update
// exactly when the spec carries a Color block.
func TestColorSpec_MountsColorFlag(t *testing.T) {
	plain := baseSpec(t)
	for _, leaf := range []string{"create", "update"} {
		if f := findLeaf(t, plain, leaf).LocalFlags().Lookup("color"); f != nil {
			t.Fatalf("%s must not register --color on a colorless spec", leaf)
		}
	}
	colored := withColor(baseSpec(t))
	for _, leaf := range []string{"create", "update"} {
		cmd := findLeaf(t, colored, leaf)
		f := cmd.LocalFlags().Lookup("color")
		if f == nil {
			t.Fatalf("%s must register --color on a color spec", leaf)
		}
		want := colored.Color.CreateUsage
		if leaf == "update" {
			want = colored.Color.UpdateUsage
		}
		if f.Usage != want {
			t.Fatalf("%s --color usage = %q, want %q", leaf, f.Usage, want)
		}
	}
}

// TestDelete_ConfirmFlags pins that the delete leaf carries the shared
// --yes/--force confirm flags.
func TestDelete_ConfirmFlags(t *testing.T) {
	spec := baseSpec(t)
	cmd := findLeaf(t, spec, "delete")
	for _, name := range []string{"yes", "force"} {
		if f := cmd.LocalFlags().Lookup(name); f == nil {
			t.Fatalf("delete must register --%s", name)
		}
	}
}
