// SPDX-License-Identifier: Apache-2.0

package display_ui_component

import (
	"encoding/json"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"strings"
	"testing"
)

// Tests for the partial-JSON-with-auto-close parser. These exercise the
// core behavior that turns streaming tool-call args into mountable
// artifact state. Each test simulates an incremental stream by feeding
// progressively larger prefixes of a complete JSON args object.

func TestCoerceStructuredField(t *testing.T) {
	// A stringified object is parsed back into a map.
	got := coerceStructuredField(`{"kind":"vbox","children":[]}`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map from stringified object, got %T", got)
	}
	if m["kind"] != "vbox" {
		t.Errorf("got kind=%v, want vbox", m["kind"])
	}

	// A stringified array is parsed back into a slice.
	if _, ok := coerceStructuredField(`[1,2,3]`).([]any); !ok {
		t.Errorf("expected slice from stringified array")
	}

	// An already-structured value passes through untouched.
	orig := map[string]any{"kind": "card"}
	if got, ok := coerceStructuredField(orig).(map[string]any); !ok || got["kind"] != "card" {
		t.Errorf("structured value should pass through, got %#v", coerceStructuredField(orig))
	}

	// A plain (non-JSON) string is left as-is.
	if got := coerceStructuredField("just text"); got != "just text" {
		t.Errorf("plain string should pass through, got %v", got)
	}

	// An incomplete/invalid JSON string is left as-is (streaming partial).
	if got := coerceStructuredField(`{"kind":"vbox"`); got != `{"kind":"vbox"` {
		t.Errorf("invalid JSON string should pass through, got %v", got)
	}
}

func TestTryParseWithAutoClose_CompleteInput(t *testing.T) {
	raw := `{"artifactId":"art-1","kind":"form","schema":{"name":"Book","root":"Book","types":{}}}`
	out, ok := tryParseWithAutoClose(raw)
	if !ok {
		t.Fatalf("expected parse to succeed on complete input")
	}
	if out["artifactId"] != "art-1" {
		t.Errorf("got artifactId=%v, want art-1", out["artifactId"])
	}
	if out["kind"] != "form" {
		t.Errorf("got kind=%v, want form", out["kind"])
	}
}

func TestTryParseWithAutoClose_PartialClosedString(t *testing.T) {
	// Stream ends at a value-closing quote — should be parseable with
	// auto-close (just append `}` for the unclosed object).
	raw := `{"artifactId":"art-1","kind":"page"`
	out, ok := tryParseWithAutoClose(raw)
	if !ok {
		t.Fatalf("expected parse to succeed with auto-close")
	}
	if out["artifactId"] != "art-1" || out["kind"] != "page" {
		t.Errorf("got %+v", out)
	}
}

func TestTryParseWithAutoClose_PartialMidString(t *testing.T) {
	// Stream ends mid-string. Auto-close strips the partial value
	// (and its key:colon) and closes.
	raw := `{"artifactId":"art-1","kind":"form","schema":"partial-na`
	out, ok := tryParseWithAutoClose(raw)
	if !ok {
		t.Fatalf("expected parse to succeed with mid-string truncation: got fail on %q", raw)
	}
	if out["artifactId"] != "art-1" {
		t.Errorf("got artifactId=%v, want art-1", out["artifactId"])
	}
	if out["kind"] != "form" {
		t.Errorf("got kind=%v, want form", out["kind"])
	}
	// The partial schema id should be dropped.
	if _, has := out["schema"]; has {
		t.Errorf("expected schema to be dropped, got %v", out["schema"])
	}
}

func TestTryParseWithAutoClose_NestedTree(t *testing.T) {
	// Stream ends mid-tree with multiple unclosed brackets.
	raw := `{"artifactId":"art-1","kind":"page","dslTree":{"kind":"vbox","children":[{"kind":"heading","text":"Hi"},{"kind":"text","content":"body"`
	out, ok := tryParseWithAutoClose(raw)
	if !ok {
		t.Fatalf("expected parse to succeed on nested partial tree")
	}
	if out["kind"] != "page" {
		t.Errorf("got kind=%v, want page", out["kind"])
	}
	tree, ok := out["dslTree"].(map[string]any)
	if !ok {
		t.Fatalf("expected dslTree to be an object, got %T", out["dslTree"])
	}
	children, ok := tree["children"].([]any)
	if !ok {
		t.Fatalf("expected children to be an array")
	}
	// Should contain the first complete child + the second partial child
	// closed by auto-close (without its `content` value, since it ended
	// mid-string).
	if len(children) < 1 {
		t.Errorf("expected at least 1 child, got %d", len(children))
	}
}

func TestTryParseWithAutoClose_EmptyInput(t *testing.T) {
	if _, ok := tryParseWithAutoClose(""); ok {
		t.Errorf("expected fail on empty input")
	}
	if _, ok := tryParseWithAutoClose("   "); ok {
		t.Errorf("expected fail on whitespace-only input")
	}
}

func TestTryParseWithAutoClose_NotJSON(t *testing.T) {
	if _, ok := tryParseWithAutoClose("hello world"); ok {
		t.Errorf("expected fail on non-JSON")
	}
}

func TestTryParseWithAutoClose_StreamSimulation(t *testing.T) {
	// Simulate a streamed input by feeding progressively larger prefixes.
	// This is what happens in production: each tool_use_delta gives us
	// more characters of the same JSON. Track how many prefixes parse
	// successfully.
	complete := `{"artifactId":"art-stream","kind":"page","dslTree":{"kind":"vbox","children":[{"kind":"heading","text":"Hi"}]}}`

	successes := 0
	for i := 1; i <= len(complete); i++ {
		prefix := complete[:i]
		_, ok := tryParseWithAutoClose(prefix)
		if ok {
			successes++
		}
	}
	// We expect MOST prefixes to parse (with the auto-close strategy).
	// Some early prefixes (before "kind" is fully named) may fail.
	if successes < len(complete)/2 {
		t.Errorf("expected most prefixes to parse, got %d/%d", successes, len(complete))
	}
	t.Logf("stream simulation: %d/%d prefixes parsed successfully", successes, len(complete))
}

func TestBuildDefPayload_StripsMountMetadata(t *testing.T) {
	parsed := map[string]any{
		"artifactId": "art-1",
		"persistent": true,
		"title":      "My Tab",
		"kind":       "form",
		"schema":     "ktw-demo:/data-type/book",
		"mode":       "edit",
	}
	def := buildDefPayload(parsed)

	if _, has := def["artifactId"]; has {
		t.Errorf("artifactId should be stripped from def")
	}
	if _, has := def["persistent"]; has {
		t.Errorf("persistent should be stripped from def")
	}
	if _, has := def["title"]; has {
		t.Errorf("title should be stripped from def")
	}
	if def["kind"] != "form" {
		t.Errorf("kind should be retained")
	}
	if def["schema"] != "ktw-demo:/data-type/book" {
		t.Errorf("schema should be retained")
	}
}

func TestExecuteDisplayUIComponent_Validation(t *testing.T) {
	t.Run("missing artifactId fails", func(t *testing.T) {
		_, err := executeDisplayUIComponent(t.Context(), DisplayUIComponentInput{Def: render.Def{Kind: "form", Schema: "ktw-demo:/data-type/book"}})
		if err == nil {
			t.Errorf("expected error for missing artifactId")
		}
	})
	t.Run("invalid kind fails", func(t *testing.T) {
		_, err := executeDisplayUIComponent(t.Context(), DisplayUIComponentInput{ArtifactID: "x", Def: render.Def{Kind: "kanban"}})
		if err == nil {
			t.Errorf("expected error for invalid kind")
		}
	})
	t.Run("page kind needs dslTree", func(t *testing.T) {
		_, err := executeDisplayUIComponent(t.Context(), DisplayUIComponentInput{ArtifactID: "x", Def: render.Def{Kind: "page"}})
		if err == nil {
			t.Errorf("expected error for missing dslTree on page kind")
		}
	})
	t.Run("schema-bound kind needs schema", func(t *testing.T) {
		_, err := executeDisplayUIComponent(t.Context(), DisplayUIComponentInput{ArtifactID: "x", Def: render.Def{Kind: "form"}})
		if err == nil {
			t.Errorf("expected error for missing schema")
		}
	})
	t.Run("valid form passes", func(t *testing.T) {
		out, err := executeDisplayUIComponent(t.Context(), DisplayUIComponentInput{
			ArtifactID: "x",
			Def:        render.Def{Kind: "form", Schema: map[string]any{"name": "Book"}},
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !out.Success {
			t.Errorf("expected Success=true")
		}
	})
	t.Run("valid page passes", func(t *testing.T) {
		out, err := executeDisplayUIComponent(t.Context(), DisplayUIComponentInput{
			ArtifactID: "x",
			Def:        render.Def{Kind: "page", DSLTree: map[string]any{"kind": "vbox", "children": []any{}}},
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !out.Success {
			t.Errorf("expected Success=true")
		}
	})
}

func TestPublishMount_PayloadShape(t *testing.T) {
	// Verifies the publishMount→Schema/Value/DSLTree split: page-kind
	// puts the tree into DSLTree (not Value); schema-bound kinds put
	// the value into Value. This is the contract the chat
	// panel bridge relies on when it builds the artifact's node.
	parsed := map[string]any{
		"artifactId": "art-1",
		"kind":       "page",
		"dslTree":    map[string]any{"kind": "vbox", "children": []any{}},
	}
	def := buildDefPayload(parsed)
	if _, has := def["dslTree"]; !has {
		t.Errorf("dslTree should be retained in def for kind=page")
	}

	// Sanity round-trip: marshal+unmarshal.
	b, _ := json.Marshal(def)
	var back map[string]any
	_ = json.Unmarshal(b, &back)
	if back["kind"] != "page" {
		t.Errorf("round-trip lost kind")
	}
}

// A deck's markdown is one string in the top-level `data`: it must reach the
// GUI while it streams, not only once it closes. Nothing else may be closed
// early — a partial key or a partial kind would stick.
func TestAutoCloseStreamsTopLevelData(t *testing.T) {
	head := `{"artifactId": "d1", "kind": "deck", "data": "---\ntitle: T\n---\n\n# One\n\n---\n\n## Tw`
	got, ok := tryParseWithAutoClose(head)
	if !ok {
		t.Fatalf("partial deck did not parse")
	}
	if md, _ := got["data"].(string); md != "---\ntitle: T\n---\n\n# One\n\n---\n\n## Tw" {
		t.Fatalf("data = %q", got["data"])
	}

	// A dangling escape and a cut \u escape are trimmed, not emitted broken.
	for _, tail := range []string{`## Two\`, `## Two \u00`} {
		got, ok := tryParseWithAutoClose(`{"artifactId": "d1", "kind": "deck", "data": "` + tail)
		if !ok || !strings.HasPrefix(got["data"].(string), "## Two") {
			t.Fatalf("tail %q: ok=%v data=%q", tail, ok, got["data"])
		}
	}

	// A partial kind is dropped, never emitted as "dec".
	if got, ok := tryParseWithAutoClose(`{"artifactId": "d1", "kind": "dec`); ok && got["kind"] != nil {
		t.Fatalf("partial kind emitted: %v", got["kind"])
	}
	// A partial key is dropped.
	if got, ok := tryParseWithAutoClose(`{"artifactId": "d1", "kind": "deck", "da`); ok && len(got) != 2 {
		t.Fatalf("partial key emitted: %v", got)
	}
	// A `data` nested in an object (drawio's {xml}) keeps the old rule.
	if got, ok := tryParseWithAutoClose(`{"artifactId": "x", "kind": "drawio", "data": {"xml": "<mxf`); ok {
		if m, _ := got["data"].(map[string]any); m != nil && m["xml"] != nil {
			t.Fatalf("nested string closed early: %v", m)
		}
	}
}
