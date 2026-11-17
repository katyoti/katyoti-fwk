// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// --- Synthetic LSP-shaped interface for testing ---

type gotoDefInput struct {
	File string `json:"file" jsonschema:"Source file"`
	Line int    `json:"line" jsonschema:"Line number"`
}
type gotoDefOutput struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Found bool   `json:"found"`
}

type findRefsInput struct {
	Symbol string `json:"symbol" jsonschema:"Symbol name"`
}
type findRefsOutput struct {
	Locations []string `json:"locations"`
}

type symbolsInput struct {
	File string `json:"file"`
}
type symbolsOutput struct {
	Names []string `json:"names"`
}

// fakeLSP is the synthetic test interface — 3 methods, all matching the
// required signature func(ctx, In) (Out, error).
type fakeLSP interface {
	GotoDefinition(ctx context.Context, in gotoDefInput) (gotoDefOutput, error)
	FindReferences(ctx context.Context, in findRefsInput) (findRefsOutput, error)
	Symbols(ctx context.Context, in symbolsInput) (symbolsOutput, error)
}

type fakeLSPImpl struct {
	openCalled  bool
	closeCalled bool
}

func (l *fakeLSPImpl) GotoDefinition(_ context.Context, in gotoDefInput) (gotoDefOutput, error) {
	if in.File == "" {
		return gotoDefOutput{}, errors.New("missing file")
	}
	return gotoDefOutput{File: in.File, Line: in.Line, Found: true}, nil
}

func (l *fakeLSPImpl) FindReferences(_ context.Context, in findRefsInput) (findRefsOutput, error) {
	return findRefsOutput{Locations: []string{in.Symbol + ":1", in.Symbol + ":42"}}, nil
}

func (l *fakeLSPImpl) Symbols(_ context.Context, _ symbolsInput) (symbolsOutput, error) {
	return symbolsOutput{Names: []string{"Foo", "Bar"}}, nil
}

func (l *fakeLSPImpl) Open(_ context.Context) error  { l.openCalled = true; return nil }
func (l *fakeLSPImpl) Close(_ context.Context) error { l.closeCalled = true; return nil }

// Compile-time assertions.
var (
	_ fakeLSP              = (*fakeLSPImpl)(nil)
	_ ConnectableToolGroup = (*fakeLSPImpl)(nil)
)

// --- Tests ---

func TestNewToolGroup_BuildsOneToolPerMethod(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "Language Server.", &fakeLSPImpl{}, WithReadOnly())
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools (one per method), got %d", len(tools))
	}
}

func TestNewToolGroup_DefaultNamingIsKebabCaseWithGroupPrefix(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "Language Server.", &fakeLSPImpl{}, WithReadOnly())
	got := map[string]bool{}
	for _, tl := range tools {
		got[tl.ToolDef().Name] = true
	}
	want := []string{"lsp-goto-definition", "lsp-find-references", "lsp-symbols"}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing expected tool %q in %v", name, got)
		}
	}
}

func TestNewToolGroup_WithGroupPrefix_Empty(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "Language Server.", &fakeLSPImpl{},
		WithReadOnly(), WithGroupPrefix(""))
	for _, tl := range tools {
		if strings.HasPrefix(tl.ToolDef().Name, "lsp") {
			t.Errorf("expected unprefixed name, got %q", tl.ToolDef().Name)
		}
	}
}

func TestNewToolGroup_WithGroupPrefix_Custom(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "Language Server.", &fakeLSPImpl{},
		WithReadOnly(), WithGroupPrefix("code-"))
	for _, tl := range tools {
		if !strings.HasPrefix(tl.ToolDef().Name, "code-") {
			t.Errorf("expected custom prefix, got %q", tl.ToolDef().Name)
		}
	}
}

func TestNewToolGroup_DescriptionIncludesHumanizedMethod(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP ops.", &fakeLSPImpl{}, WithReadOnly())
	for _, tl := range tools {
		def := tl.ToolDef()
		if !strings.HasPrefix(def.Description, "LSP ops.") {
			t.Errorf("description should start with group desc, got %q", def.Description)
		}
		if !strings.Contains(def.Description, "—") {
			t.Errorf("description should contain humanized method, got %q", def.Description)
		}
	}
}

func TestNewToolGroup_SharesOptionsAcrossMethods(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	for _, tl := range tools {
		if !tl.IsReadOnly() {
			t.Errorf("%s should inherit WithReadOnly", tl.ToolDef().Name)
		}
		if !tl.IsConcurrencySafe() {
			t.Errorf("%s should inherit ConcurrencySafe from WithReadOnly", tl.ToolDef().Name)
		}
	}
}

func TestNewToolGroup_GeneratesNonEmptySchemas(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	for _, tl := range tools {
		if len(tl.ToolDef().InputSchema) == 0 {
			t.Errorf("%s has empty schema", tl.ToolDef().Name)
		}
	}
}

func TestNewToolGroup_DispatchHappyPath(t *testing.T) {
	impl := &fakeLSPImpl{}
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", impl, WithReadOnly())

	var goDef Tool
	for _, tl := range tools {
		if tl.ToolDef().Name == "lsp-goto-definition" {
			goDef = tl
			break
		}
	}
	if goDef == nil {
		t.Fatal("lsp-goto-definition not found")
	}

	res, err := goDef.ToolCall(context.Background(), json.RawMessage(`{"file":"main.go","line":42}`))
	if err != nil {
		t.Fatalf("ToolCall failed: %v", err)
	}
	out, ok := res.Data.(gotoDefOutput)
	if !ok {
		t.Fatalf("Data should be gotoDefOutput, got %T", res.Data)
	}
	if out.File != "main.go" || out.Line != 42 || !out.Found {
		t.Errorf("unexpected output: %+v", out)
	}
}

func TestNewToolGroup_DispatchPropagatesError(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	var goDef Tool
	for _, tl := range tools {
		if tl.ToolDef().Name == "lsp-goto-definition" {
			goDef = tl
			break
		}
	}
	_, err := goDef.ToolCall(context.Background(), json.RawMessage(`{"file":"","line":0}`))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if err.Error() != "missing file" {
		t.Errorf("expected handler error to propagate verbatim, got %q", err.Error())
	}
}

func TestNewToolGroup_DispatchBadInput(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	_, err := tools[0].ToolCall(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse input") {
		t.Errorf("expected parse-input error, got %q", err.Error())
	}
}

// --- Validation: bad signatures panic at NewToolGroup time ---

type badArgCount interface {
	Foo(ctx context.Context) (string, error) // missing input arg
}
type badArgCountImpl struct{}

func (badArgCountImpl) Foo(_ context.Context) (string, error) { return "", nil }

func TestNewToolGroup_PanicOnWrongArgCount(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for wrong arg count")
		}
	}()
	_ = NewToolGroup[badArgCount]("bad", "x", badArgCountImpl{})
}

type badFirstArg interface {
	Foo(s string, in symbolsInput) (symbolsOutput, error) // first arg not context.Context
}
type badFirstArgImpl struct{}

func (badFirstArgImpl) Foo(_ string, _ symbolsInput) (symbolsOutput, error) {
	return symbolsOutput{}, nil
}

func TestNewToolGroup_PanicOnNonContextFirstArg(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-context first arg")
		}
	}()
	_ = NewToolGroup[badFirstArg]("bad", "x", badFirstArgImpl{})
}

type badReturnCount interface {
	Foo(ctx context.Context, in symbolsInput) symbolsOutput // missing error return
}
type badReturnCountImpl struct{}

func (badReturnCountImpl) Foo(_ context.Context, _ symbolsInput) symbolsOutput {
	return symbolsOutput{}
}

func TestNewToolGroup_PanicOnWrongReturnCount(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for wrong return count")
		}
	}()
	_ = NewToolGroup[badReturnCount]("bad", "x", badReturnCountImpl{})
}

type badSecondReturn interface {
	Foo(ctx context.Context, in symbolsInput) (symbolsOutput, string) // second return not error
}
type badSecondReturnImpl struct{}

func (badSecondReturnImpl) Foo(_ context.Context, _ symbolsInput) (symbolsOutput, string) {
	return symbolsOutput{}, ""
}

func TestNewToolGroup_PanicOnNonErrorSecondReturn(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-error second return")
		}
	}()
	_ = NewToolGroup[badSecondReturn]("bad", "x", badSecondReturnImpl{})
}

func TestNewToolGroup_PanicOnNonInterfaceTGroup(t *testing.T) {
	type notAnInterface struct{ X int }
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-interface TGroup")
		}
	}()
	_ = NewToolGroup[notAnInterface]("bad", "x", notAnInterface{})
}

// --- Permission policy inheritance ---

func TestNewToolGroup_InheritsPermissionPolicy(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	for _, tl := range tools {
		got := tl.CheckPermissions(context.Background(), json.RawMessage(`{}`))
		if got.Behavior != PermissionAllow {
			t.Errorf("%s: read-only group should auto-allow, got %v", tl.ToolDef().Name, got.Behavior)
		}
	}
}

func TestNewToolGroup_FailClosedDefault(t *testing.T) {
	// No options → fail-closed: every method asks.
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{})
	for _, tl := range tools {
		got := tl.CheckPermissions(context.Background(), json.RawMessage(`{}`))
		if got.Behavior != PermissionAsk {
			t.Errorf("%s: no-options group should ask (fail-closed), got %v", tl.ToolDef().Name, got.Behavior)
		}
	}
}

// --- Helpers ---

func TestCamelToKebab(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GotoDefinition", "goto-definition"},
		{"FindReferences", "find-references"},
		{"Symbols", "symbols"},
		{"HTTPGet", "http-get"},   // run of caps followed by lowercase
		{"ParseURL", "parse-url"}, // trailing run of caps
		{"ID", "id"},              // pure caps
		{"OAuth2Login", "o-auth2-login"},
		{"", ""},
	}
	for _, c := range cases {
		if got := camelToKebab(c.in); got != c.want {
			t.Errorf("camelToKebab(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GotoDefinition", "Goto Definition"},
		{"FindReferences", "Find References"},
		{"Symbols", "Symbols"},
	}
	for _, c := range cases {
		if got := humanize(c.in); got != c.want {
			t.Errorf("humanize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
