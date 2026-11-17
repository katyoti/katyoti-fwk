// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

// A part named by URI travels as {uri, hash}; an inline part as itself - and
// both read back as what was written.
func TestSpecRoundTrip(t *testing.T) {
	a := Answer{
		Tree:  &Spec{Inline: VBox(Text("hello"))},
		Parts: map[string]Spec{"ktw-x:/data-type/request": SpecOf("ktw-x:/data-type/request", "sha256:ab12")},
		Data:  map[string]any{"n": 1},
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"parts":{"ktw-x:/data-type/request":{"uri":"ktw-x:/data-type/request","hash":"sha256:ab12"}}`) {
		t.Fatalf("named part on the wire: %s", raw)
	}
	if !strings.Contains(string(raw), `"tree":{"children":[{"content":"hello","kind":"text"}]`) {
		t.Fatalf("inline tree on the wire: %s", raw)
	}
	var back Answer
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if p := back.Parts["ktw-x:/data-type/request"]; !p.Named() || p.Hash != "sha256:ab12" {
		t.Fatalf("named part read back: %+v", p)
	}
	if back.Tree == nil || back.Tree.Named() || back.Tree.Inline == nil {
		t.Fatalf("inline tree read back: %+v", back.Tree)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("the answer read back validates: %v", err)
	}
}

// An inline part that happens to hold a uri among other fields stays inline:
// only {uri} or {uri, hash} names a part.
func TestSpecInlineWithURIField(t *testing.T) {
	var s Spec
	if err := json.Unmarshal([]byte(`{"kind":"link","uri":"x","href":"y"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Named() {
		t.Fatalf("a node with a uri field is inline: %+v", s)
	}
}

func TestAnswerValidate(t *testing.T) {
	tree := InlineSpec(Text("x"))
	cases := map[string]struct {
		a  Answer
		ok bool
	}{
		"a tree":                     {Answer{Tree: &tree}, true},
		"a named tree and data":      {Answer{Tree: &Spec{URI: "ktw-x:/page-template/t", Hash: "h"}, Data: []any{1}}, true},
		"a panel":                    {Answer{Panel: "dp-1"}, true},
		"data alone":                 {Answer{Data: map[string]any{"a": 1}}, true},
		"nothing":                    {Answer{}, false},
		"a tree and a panel":         {Answer{Tree: &tree, Panel: "dp-1"}, false},
		"an empty part":              {Answer{Data: 1, Parts: map[string]Spec{"x": {}}}, false},
		"an inline tree off grammar": {Answer{Tree: &Spec{Inline: Node{"kind": "nope"}}}, false},
	}
	for name, c := range cases {
		err := c.a.Validate()
		if (err == nil) != c.ok {
			t.Errorf("%s: ok=%v, got %v", name, c.ok, err)
		}
	}
}

func TestOpValidate(t *testing.T) {
	cases := map[string]struct {
		op Op
		ok bool
	}{
		"replace":             {Replace(1, "s1", Text("x")), true},
		"replace off grammar": {Replace(1, "s1", Node{"kind": "nope"}), false},
		"replace no node":     {Op{Seq: 1, Op: OpReplace, Target: "s1"}, false},
		"append":              {Append(2, "list", map[string]any{"id": "a"}), true},
		"append nothing":      {Op{Seq: 2, Op: OpAppend, Target: "list"}, false},
		"merge":               {Op{Seq: 3, Op: OpMerge, Target: "s1", Value: map[string]any{"title": "y"}}, true},
		"merge a scalar":      {Op{Seq: 3, Op: OpMerge, Target: "s1", Value: 3}, false},
		"remove":              {Op{Seq: 4, Op: OpRemove, Target: "s1"}, true},
		"fail":                {Fail(5, "s1", "boom"), true},
		"fail silent":         {Op{Seq: 5, Op: OpFail, Target: "s1"}, false},
		"complete":            {Complete(6), true},
		"no target":           {Op{Seq: 7, Op: OpRemove}, false},
		"a JSON Patch op":     {Op{Seq: 8, Op: "rfc6902", Target: "s1"}, false},
	}
	for name, c := range cases {
		err := c.op.Validate()
		if (err == nil) != c.ok {
			t.Errorf("%s: ok=%v, got %v", name, c.ok, err)
		}
	}
}

// An op decoded from the wire validates as the one built in Go.
func TestOpRoundTrip(t *testing.T) {
	raw, _ := json.Marshal([]Op{Replace(1, "lane:s1", Card(Text("x"))), Append(2, "rows", 1, 2), Complete(3)})
	var back []Op
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 3 || back[0].Target != "lane:s1" || back[1].Op != OpAppend || back[2].Op != OpComplete {
		t.Fatalf("ops read back: %+v", back)
	}
	for _, o := range back {
		if err := o.Validate(); err != nil {
			t.Fatalf("op %d read back: %v", o.Seq, err)
		}
	}
}

// Ops target nodes by id, so a page refuses an id used twice; a section
// needs its id and its load method, and knows its placeholders and whens.
func TestValidatePageSections(t *testing.T) {
	load := Load{Method: "ktw-x:views/lane", Args: map[string]any{"stream": "s1"}}
	ok := VBox(
		Card(Text("a")).AsSection("lane:s1", load, Placeholder{Kind: PlaceholderCards, Count: 3}).Refresh("module", "request"),
		Card(Text("b")).AsSection("lane:s2", load, Placeholder{Kind: PlaceholderSkeleton}).LoadWhen(WhenEager),
	)
	if err := ValidatePage(ok); err != nil {
		t.Fatalf("two sections: %v", err)
	}
	bad := map[string]Node{
		"an id used twice":             VBox(Text("a").With("id", "x"), HBox(Text("b").With("id", "x"))),
		"a section without its id":     VBox(Text("a").With("load", load)),
		"a section without its method": VBox(Text("a").AsSection("s", Load{}, Placeholder{Kind: PlaceholderRows})),
		"an unknown placeholder":       VBox(Text("a").AsSection("s", load, Placeholder{Kind: "spinner"})),
		"an unknown when":              VBox(Text("a").AsSection("s", load, Placeholder{Kind: PlaceholderRows}).LoadWhen("later")),
	}
	for name, page := range bad {
		if err := ValidatePage(page); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A part's hash is stable over a JSON round-trip and its key order, changes
// with its body, and is what NamedSpec names the part with.
func TestSpecHash(t *testing.T) {
	part := map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "number", "max": 1.5}, "a": map[string]any{"type": "string"}}}
	h1, err := SpecHash(part)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h1, "sha256:") || len(h1) != len("sha256:")+64 {
		t.Fatalf("hash %q", h1)
	}
	var back any
	dec := json.NewDecoder(strings.NewReader(`{"properties":{"a":{"type":"string"},"b":{"max":1.5,"type":"number"}},"type":"object"}`))
	dec.UseNumber()
	if err := dec.Decode(&back); err != nil {
		t.Fatal(err)
	}
	if h2, _ := SpecHash(back); h2 != h1 {
		t.Fatalf("the same part read back hashes %s, not %s", h2, h1)
	}
	part["title"] = "Request"
	if h3, _ := SpecHash(part); h3 == h1 {
		t.Fatal("a changed body kept its hash")
	}
	s, err := NamedSpec("ktw-x:/data-type/request", part)
	if err != nil {
		t.Fatal(err)
	}
	if h4, _ := SpecHash(part); !s.Named() || s.URI != "ktw-x:/data-type/request" || s.Hash != h4 {
		t.Fatalf("named spec %+v", s)
	}
}
