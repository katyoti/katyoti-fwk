// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

func op(kind, o, path string, value any) terminal.UIPatchOp {
	p := terminal.UIPatchOp{Kind: kind, Op: o, Path: path}
	if value != nil {
		p.Value, _ = json.Marshal(value)
	}
	return p
}

func decoded(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// The bridge applies the chat artifact's ops as the GUI's store does
// (patch-apply.ts): RFC 6902, RFC 7396, append, replace and remove by
// pointer, never changing the value it was given.
func TestThePatchOpsApplyAsTheGUIStoreApplies(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ops  []terminal.UIPatchOp
		want string
	}{
		{"add inserts at an index", `{"a":[1,3]}`, []terminal.UIPatchOp{op("rfc6902", "add", "/a/1", 2)}, `{"a":[1,2,3]}`},
		{"add - appends", `{"a":[1]}`, []terminal.UIPatchOp{op("rfc6902", "add", "/a/-", 2)}, `{"a":[1,2]}`},
		{"move", `{"a":1,"b":{}}`, []terminal.UIPatchOp{{Kind: "rfc6902", Op: "move", From: "/a", Path: "/b/c"}}, `{"b":{"c":1}}`},
		{"copy", `{"a":[1]}`, []terminal.UIPatchOp{{Kind: "rfc6902", Op: "copy", From: "/a", Path: "/b"}}, `{"a":[1],"b":[1]}`},
		{"a failed test leaves it", `{"a":1}`, []terminal.UIPatchOp{op("rfc6902", "test", "/a", 2)}, `{"a":1}`},
		{"rfc6902 remove", `{"a":[1,2]}`, []terminal.UIPatchOp{op("rfc6902", "remove", "/a/0", nil)}, `{"a":[2]}`},
		{"merge", `{"a":{"x":1,"y":2}}`, []terminal.UIPatchOp{{Kind: "rfc7396", MergePatch: json.RawMessage(`{"a":{"x":null,"z":3}}`)}}, `{"a":{"y":2,"z":3}}`},
		{"append makes the array", `{}`, []terminal.UIPatchOp{op("append", "", "items", 1), op("append", "", "/items", 2)}, `{"items":[1,2]}`},
		{"replace makes the path", `{}`, []terminal.UIPatchOp{op("replace", "", "/a/b", 1)}, `{"a":{"b":1}}`},
		{"remove", `{"a":1,"b":2}`, []terminal.UIPatchOp{op("remove", "", "/a", nil)}, `{"b":2}`},
		{"escaped pointer", `{"a/b":{"c~d":1}}`, []terminal.UIPatchOp{op("replace", "", "/a~1b/c~0d", 2)}, `{"a/b":{"c~d":2}}`},
		{"a target is its node", `{"kids":[{"id":"k","v":1}]}`, []terminal.UIPatchOp{{Kind: "rfc7396", Target: "k", MergePatch: json.RawMessage(`{"v":2}`)}}, `{"kids":[{"id":"k","v":2}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := decoded(t, c.in)
			kept := decoded(t, c.in)
			got, _ := applyOps(in, c.ops)
			if !reflect.DeepEqual(got, decoded(t, c.want)) {
				g, _ := json.Marshal(got)
				t.Fatalf("got %s, want %s", g, c.want)
			}
			if !reflect.DeepEqual(in, kept) {
				t.Fatalf("the value given was changed: %v", in)
			}
		})
	}
}

// A deck's ops edit it slide by slide, as the GUI's editDeck does.
func TestADeckIsEditedSlideBySlide(t *testing.T) {
	md := "---\ntitle: T\n---\n\n# One\n\n---\n\n## Two\n\n---\n\n## Three"
	edit := func(ops ...terminal.UIPatchOp) string {
		out, errs := editDeck(md, ops)
		if len(errs) != 0 {
			t.Fatalf("errors: %v", errs)
		}
		return out
	}
	tgt := func(o terminal.UIPatchOp, target string) terminal.UIPatchOp { o.Target = target; return o }
	if got := edit(tgt(op("replace", "", "", "## Deux"), "slide-2")); got != "---\ntitle: T\n---\n\n# One\n\n---\n\n## Deux\n\n---\n\n## Three" {
		t.Fatalf("replace: %q", got)
	}
	if got := edit(tgt(op("rfc6902", "add", "", "## New"), "slide-2")); got != "---\ntitle: T\n---\n\n# One\n\n---\n\n## New\n\n---\n\n## Two\n\n---\n\n## Three" {
		t.Fatalf("add: %q", got)
	}
	if got := edit(tgt(op("remove", "", "", nil), "slide-2")); got != "---\ntitle: T\n---\n\n# One\n\n---\n\n## Three" {
		t.Fatalf("remove: %q", got)
	}
	if got := edit(op("append", "", "", "## Four")); got != md+"\n\n---\n\n## Four\n" {
		t.Fatalf("append: %q", got)
	}
	if _, errs := editDeck(md, []terminal.UIPatchOp{tgt(op("replace", "", "", "x"), "slide-9")}); len(errs) != 1 {
		t.Fatalf("a slide past the end: %v", errs)
	}
}
