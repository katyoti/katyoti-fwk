// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type sampleInput struct {
	Path string `json:"path" jsonschema:"File path"`
}

type sampleOutput struct {
	OK bool `json:"ok"`
}

func sampleHandler(_ context.Context, in sampleInput) (sampleOutput, error) {
	return sampleOutput{OK: in.Path != ""}, nil
}

// --- Defaults (fail-closed per spec FR-01) ---

func TestFunctionTool_FailClosedDefaults(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test tool", sampleHandler)

	if tool.IsReadOnly() {
		t.Error("default IsReadOnly should be false (fail-closed)")
	}
	if tool.IsConcurrencySafe() {
		t.Error("default IsConcurrencySafe should be false (fail-closed)")
	}
	if tool.IsDestructive() {
		t.Error("default IsDestructive should be false")
	}
	if tool.InterruptBehavior() != InterruptBlock {
		t.Errorf("default InterruptBehavior should be InterruptBlock, got %v", tool.InterruptBehavior())
	}
	if tool.MaxResultSize() != MaxResultSizeDefault {
		t.Errorf("default MaxResultSize should be %d, got %d", MaxResultSizeDefault, tool.MaxResultSize())
	}
	if tool.ShouldDefer() {
		t.Error("default ShouldDefer should be false")
	}
	if !tool.IsEnabled() {
		t.Error("default IsEnabled should be true")
	}
	if tool.Aliases() != nil {
		t.Errorf("default Aliases should be nil, got %v", tool.Aliases())
	}
	if tool.Prompt() != "" {
		t.Errorf("default Prompt should be empty, got %q", tool.Prompt())
	}
	if tool.SearchHint() != "" {
		t.Errorf("default SearchHint should be empty, got %q", tool.SearchHint())
	}
}

// --- Default permission policy chain ---

func TestFunctionTool_DefaultPermission_ReadOnly(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	got := tool.CheckPermissions(context.Background(), json.RawMessage(`{}`))
	if got.Behavior != PermissionAllow {
		t.Errorf("read-only tool should auto-allow, got %v", got.Behavior)
	}
}

func TestFunctionTool_DefaultPermission_MutatingDefault(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler) // no opts
	got := tool.CheckPermissions(context.Background(), json.RawMessage(`{}`))
	if got.Behavior != PermissionAsk {
		t.Errorf("mutating-default tool should ask, got %v", got.Behavior)
	}
}

func TestFunctionTool_DefaultPermission_DestructiveEscalates(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithDestructive())
	got := tool.CheckPermissions(context.Background(), json.RawMessage(`{}`))
	if got.Behavior != PermissionAsk {
		t.Errorf("destructive tool should ask, got %v", got.Behavior)
	}
	if !strings.Contains(got.Message, "irreversible") {
		t.Errorf("destructive prompt should mention irreversibility, got %q", got.Message)
	}
}

func TestFunctionTool_CustomPermissionOverrides(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler,
		WithReadOnly(),                        // would normally allow
		WithPermissionCheck(DenyFunc("nope")), // override
	)
	got := tool.CheckPermissions(context.Background(), json.RawMessage(`{}`))
	if got.Behavior != PermissionDeny {
		t.Errorf("explicit WithPermissionCheck should override, got %v", got.Behavior)
	}
	if got.Message != "nope" {
		t.Errorf("custom deny message lost, got %q", got.Message)
	}
}

// --- Options composition ---

func TestFunctionTool_WithDestructiveImpliesMutating(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithDestructive())
	if tool.IsReadOnly() {
		t.Error("WithDestructive should imply !ReadOnly")
	}
	if tool.IsConcurrencySafe() {
		t.Error("WithDestructive should imply !ConcurrencySafe")
	}
	if !tool.IsDestructive() {
		t.Error("WithDestructive should set Destructive=true")
	}
}

func TestFunctionTool_WithReadOnlyImpliesConcurrent(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	if !tool.IsReadOnly() {
		t.Error("WithReadOnly should set ReadOnly=true")
	}
	if !tool.IsConcurrencySafe() {
		t.Error("WithReadOnly should imply ConcurrencySafe=true")
	}
}

func TestFunctionTool_WithUnlimitedResult(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithUnlimitedResult())
	if tool.MaxResultSize() != 0 {
		t.Errorf("WithUnlimitedResult should yield MaxResultSize=0, got %d", tool.MaxResultSize())
	}
}

func TestFunctionTool_WithMaxResultSizeOverridesDefault(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithMaxResultSize(1024))
	if tool.MaxResultSize() != 1024 {
		t.Errorf("WithMaxResultSize(1024) should yield 1024, got %d", tool.MaxResultSize())
	}
}

func TestFunctionTool_WithCancelOnInterrupt(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithCancelOnInterrupt())
	if tool.InterruptBehavior() != InterruptCancel {
		t.Errorf("WithCancelOnInterrupt should yield InterruptCancel, got %v", tool.InterruptBehavior())
	}
}

func TestFunctionTool_WithDeferred(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithDeferred("search code"))
	if !tool.ShouldDefer() {
		t.Error("WithDeferred should set ShouldDefer=true")
	}
	if tool.SearchHint() != "search code" {
		t.Errorf("WithDeferred should set SearchHint, got %q", tool.SearchHint())
	}
}

func TestFunctionTool_WithAliases(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithAliases("foo", "bar"))
	got := tool.Aliases()
	if len(got) != 2 || got[0] != "foo" || got[1] != "bar" {
		t.Errorf("WithAliases failed, got %v", got)
	}
}

func TestFunctionTool_WithEnabledFunc(t *testing.T) {
	enabled := false
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler,
		WithEnabledFunc(func() bool { return enabled }),
	)
	if tool.IsEnabled() {
		t.Error("expected disabled")
	}
	enabled = true
	if !tool.IsEnabled() {
		t.Error("expected enabled after toggle")
	}
}

// --- Describe ---

func TestFunctionTool_DefaultDescribe(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler)
	got := tool.Describe(json.RawMessage(`{"path":"a.txt"}`))
	if !strings.HasPrefix(got, "sample ") {
		t.Errorf("default Describe should start with name, got %q", got)
	}
	if !strings.Contains(got, "a.txt") {
		t.Errorf("default Describe should include input, got %q", got)
	}
}

func TestFunctionTool_DefaultDescribe_EmptyInput(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler)
	if got := tool.Describe(nil); got != "sample" {
		t.Errorf("nil input should yield bare name, got %q", got)
	}
}

func TestFunctionTool_DefaultDescribe_TruncatesLongInput(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler)
	long := `{"path":"` + strings.Repeat("x", 500) + `"}`
	got := tool.Describe(json.RawMessage(long))
	if !strings.HasSuffix(got, "…") {
		t.Errorf("long input should be truncated with ellipsis, got %q (len %d)", got, len(got))
	}
}

func TestFunctionTool_CustomDescriber(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler,
		WithDescriber(func(_ json.RawMessage) string { return "custom!" }),
	)
	if got := tool.Describe(json.RawMessage(`{}`)); got != "custom!" {
		t.Errorf("custom describer not used, got %q", got)
	}
}

// --- ToolCall roundtrip ---

func TestFunctionTool_ToolCall(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())

	got, err := tool.ToolCall(context.Background(), json.RawMessage(`{"path":"a.txt"}`))
	if err != nil {
		t.Fatalf("ToolCall failed: %v", err)
	}
	out, ok := got.Data.(sampleOutput)
	if !ok {
		t.Fatalf("Data should be sampleOutput, got %T", got.Data)
	}
	if !out.OK {
		t.Error("expected OK=true for non-empty path")
	}
}

func TestFunctionTool_ToolCall_BadInput(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	_, err := tool.ToolCall(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected parse error on bad input")
	}
}

// --- ToolDef ---

func TestFunctionTool_ToolDef(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test desc", sampleHandler)
	def := tool.ToolDef()
	if def.Name != "sample" {
		t.Errorf("ToolDef.Name = %q, want %q", def.Name, "sample")
	}
	if def.Description != "test desc" {
		t.Errorf("ToolDef.Description = %q, want %q", def.Description, "test desc")
	}
	if len(def.InputSchema) == 0 {
		t.Error("ToolDef.InputSchema should be non-empty")
	}
}
