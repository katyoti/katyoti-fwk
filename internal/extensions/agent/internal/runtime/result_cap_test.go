// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// : a tool result over the tool's MaxResultSize reaches the model
// as its start, its size and what to do, the whole result kept in a file; a
// result within the limit, or of a tool without one, is untouched; a tool the
// loop does not know gets the default limit.
func TestAToolResultOverItsLimitIsPreviewed(t *testing.T) {
	type in struct{}
	tool := func(opts ...toolsapi.ToolOption) toolsapi.Tool {
		return toolsapi.NewFunctionTool[in, string]("entities-list", "lists entities",
			func(context.Context, in) (string, error) { return "", nil }, opts...)
	}
	dir := filepath.Join(t.TempDir(), "tool-results")
	huge := strings.Repeat("é", 300*1024) // 600 KB, two bytes a rune

	got := capToolResult(tool(), "toolu_01/A", huge, dir)
	if len(got) > toolsapi.MaxResultSizeDefault/8 {
		t.Fatalf("the model got %d bytes of a %d-byte result", len(got), len(huge))
	}
	if !strings.HasPrefix(got, strings.Repeat("é", 1000)) || !strings.Contains(got, "[Result cut: 614400 bytes") || !strings.Contains(got, "Ask for less") {
		t.Fatalf("the preview lacks the result's start, its size or what to do:\n%s", got[len(got)-400:])
	}
	if !strings.Contains(got, "toolu_01_A.txt") {
		t.Fatalf("the preview does not say where the whole result is: %s", got[len(got)-400:])
	}
	kept, err := os.ReadFile(filepath.Join(dir, "toolu_01_A.txt"))
	if err != nil || string(kept) != huge {
		t.Fatalf("the whole result is not kept: %v (%d bytes)", err, len(kept))
	}
	if strings.ContainsRune(strings.TrimSuffix(got[:strings.Index(got, "\n\n[Result cut")], "é"), '�') {
		t.Fatal("the preview cuts a character in two")
	}

	// A tool's own limit; a result within it is untouched.
	small := capToolResult(tool(toolsapi.WithMaxResultSize(1000)), "t2", strings.Repeat("x", 1500), "")
	if !strings.HasPrefix(small, strings.Repeat("x", 1000)+"\n\n[Result cut: 1500 bytes, over this tool's limit of 1000") {
		t.Fatalf("the tool's own limit is not applied: %q", small[:80])
	}
	if r := strings.Repeat("y", 900); capToolResult(tool(toolsapi.WithMaxResultSize(1000)), "t3", r, dir) != r {
		t.Fatal("a result within its limit was changed")
	}
	// Unlimited, and a tool the loop does not know (the default limit).
	if capToolResult(tool(toolsapi.WithUnlimitedResult()), "t4", huge, dir) != huge {
		t.Fatal("an unlimited tool's result was cut")
	}
	if got := capToolResult(nil, "t5", huge, ""); len(got) >= len(huge) || strings.Contains(got, "kept in") {
		t.Fatalf("an unknown tool's huge result passed whole, or claims a file it has not (%d bytes)", len(got))
	}
}
