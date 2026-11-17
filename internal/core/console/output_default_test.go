// SPDX-License-Identifier: Apache-2.0

package console

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveDefaultOutputMode_NoAnnotation(t *testing.T) {
	cmd := &cobra.Command{Use: "leaf"}
	if got := resolveDefaultOutputMode(cmd); got != OutputRich {
		t.Errorf("no annotation should default to OutputRich, got %v", got)
	}
}

func TestSetGroupDefault_OnLeaf(t *testing.T) {
	cmd := &cobra.Command{Use: "leaf"}
	SetGroupDefault(cmd, OutputJSON)
	if got := resolveDefaultOutputMode(cmd); got != OutputJSON {
		t.Errorf("set on leaf, got %v want %v", got, OutputJSON)
	}
}

func TestSetGroupDefault_InheritsFromParent(t *testing.T) {
	llmGroup := &cobra.Command{Use: "llm"}
	SetGroupDefault(llmGroup, OutputJSON)

	tools := &cobra.Command{Use: "tools"}
	llmGroup.AddCommand(tools)

	list := &cobra.Command{Use: "list"}
	tools.AddCommand(list)

	if got := resolveDefaultOutputMode(list); got != OutputJSON {
		t.Errorf("descendant of llm should inherit OutputJSON, got %v", got)
	}
}

func TestSetGroupDefault_DescendantOverridesAncestor(t *testing.T) {
	llmGroup := &cobra.Command{Use: "llm"}
	SetGroupDefault(llmGroup, OutputJSON)

	special := &cobra.Command{Use: "special"}
	SetGroupDefault(special, OutputRaw)
	llmGroup.AddCommand(special)

	if got := resolveDefaultOutputMode(special); got != OutputRaw {
		t.Errorf("descendant override should win, got %v want %v", got, OutputRaw)
	}
}

// Regression test for the lazy parent-chain bug fixed 2026-05-23
// (SN-06). AddOutputFlag is typically called BEFORE the command is
// attached to its parent (the typical pattern: `child := buildCmd();
// parent.AddCommand(child)`). At construction time cmd.Parent() is
// nil, so the default must be resolved lazily at PreRunE time once
// the cobra tree is fully wired.
func TestAddOutputFlag_LazyResolveAfterAttach(t *testing.T) {
	// Build the leaf first — parent is nil here.
	leaf := &cobra.Command{Use: "list", RunE: func(c *cobra.Command, a []string) error { return nil }}
	mode := AddOutputFlag(leaf) // resolved here would see no parent → OutputRich

	// Now build the tree: root → llm (JSON-default) → tools → list.
	root := &cobra.Command{Use: "ktw-foo"}
	llm := &cobra.Command{Use: "llm"}
	SetGroupDefault(llm, OutputJSON)
	tools := &cobra.Command{Use: "tools"}
	tools.AddCommand(leaf)
	llm.AddCommand(tools)
	root.AddCommand(llm)

	// Execute the leaf with NO --out flag — the PreRunE hook should
	// walk the parent chain and apply OutputJSON.
	root.SetArgs([]string{"llm", "tools", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *mode != OutputJSON {
		t.Errorf("lazy-resolve should pick OutputJSON from llm ancestor, got %v", *mode)
	}
}

// Explicit --out override beats the group default — also via the lazy
// PreRunE path.
func TestAddOutputFlag_ExplicitOverrideBeatsDefault(t *testing.T) {
	leaf := &cobra.Command{Use: "list", RunE: func(c *cobra.Command, a []string) error { return nil }}
	mode := AddOutputFlag(leaf)

	root := &cobra.Command{Use: "ktw-foo"}
	llm := &cobra.Command{Use: "llm"}
	SetGroupDefault(llm, OutputJSON)
	llm.AddCommand(leaf)
	root.AddCommand(llm)

	root.SetArgs([]string{"llm", "list", "--out", "rich"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if *mode != OutputRich {
		t.Errorf("explicit --out rich must win over llm JSON default, got %v", *mode)
	}
}

func TestSetGroupDefault_SiblingsIndependent(t *testing.T) {
	root := &cobra.Command{Use: "ktw-foo"}

	llm := &cobra.Command{Use: "llm"}
	SetGroupDefault(llm, OutputJSON)
	root.AddCommand(llm)

	sc := &cobra.Command{Use: "sc"}
	root.AddCommand(sc)

	if got := resolveDefaultOutputMode(llm); got != OutputJSON {
		t.Errorf("llm should be JSON, got %v", got)
	}
	if got := resolveDefaultOutputMode(sc); got != OutputRich {
		t.Errorf("sc (no annotation) should default to OutputRich, got %v", got)
	}
}
