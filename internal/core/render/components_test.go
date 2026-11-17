// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

func bookSchema() map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(`{"name":"book","root":"Book","types":{"Book":{"type":"object","fields":{"title":{"type":"string"}},
		"extensions":[{"type":"ui-view","name":"default","fields":["title"]},{"type":"ui-view","name":"compact","fields":["title"]}]}}}`), &m)
	return m
}

type fetchCall struct{ uri, ext, name string }

func fakeFetcher(calls *[]fetchCall) render.ComponentResolver {
	return render.ComponentResolver{Fetch: func(ref render.SchemaRef, ext, name string) (map[string]any, error) {
		*calls = append(*calls, fetchCall{ref.String(), ext, name})
		return bookSchema(), nil
	}}
}

func parseJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A component naming its schema by id gets the AST, fetched with the
// ui-<kind> extension its variant names.
func TestResolveComponentsBySchemaID(t *testing.T) {
	var calls []fetchCall
	out, err := render.ResolveComponents(parseJSON(t, `{"kind":"view","schema":"ktw-demo:/data-type/book","variant":"compact","data":{"title":"Dune"}}`), fakeFetcher(&calls))
	if err != nil {
		t.Fatal(err)
	}
	c := out.(map[string]any)
	if _, ok := c["schema"].(map[string]any); !ok {
		t.Fatalf("schema not inlined: %#v", c["schema"])
	}
	if len(calls) != 1 || calls[0] != (fetchCall{"ktw-demo:/data-type/book", "ui-view", "compact"}) {
		t.Errorf("fetch calls = %+v", calls)
	}
	if c["data"].(map[string]any)["title"] != "Dune" {
		t.Error("data must pass through untouched")
	}
}

// Every schema-bound node nested in a page resolves too, each schema fetched
// once per variant.
func TestResolveComponentsInsideAPage(t *testing.T) {
	var calls []fetchCall
	page := `{"kind":"page","dslTree":{"kind":"vbox","children":[
		{"kind":"view","schema":"ktw-demo:/data-type/book"},
		{"kind":"card","content":{"kind":"view","schema":"ktw-demo:/data-type/book"}},
		{"kind":"table","schema":"ktw-demo:/data-type/book","variant":"list"}]}}`
	out, err := render.ResolveComponents(parseJSON(t, page), fakeFetcher(&calls))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), `"schema":"ktw-demo`) {
		t.Errorf("an id survived: %s", b)
	}
	if len(calls) != 2 {
		t.Errorf("want 2 distinct fetches (view default, table list), got %+v", calls)
	}
	if calls[0].name != "*" {
		t.Errorf("the default variant is fetched with every extension of its type, got %q", calls[0].name)
	}
}

// A ui block is laid over the schema's root type as the variant it names,
// replacing the same-named extension; the component then uses it.
func TestResolveComponentsLaysAUIBlockOver(t *testing.T) {
	var calls []fetchCall
	out, err := render.ResolveComponents(parseJSON(t, `{"kind":"view","schema":"ktw-demo:/data-type/book","ui":{"name":"compact","fields":["title","author"]}}`), fakeFetcher(&calls))
	if err != nil {
		t.Fatal(err)
	}
	c := out.(map[string]any)
	if _, has := c["ui"]; has {
		t.Error("the ui block must be consumed")
	}
	if c["variant"] != "compact" {
		t.Errorf("variant = %v", c["variant"])
	}
	exts := c["schema"].(map[string]any)["types"].(map[string]any)["Book"].(map[string]any)["extensions"].([]any)
	var compact []map[string]any
	for _, e := range exts {
		if m := e.(map[string]any); m["name"] == "compact" {
			compact = append(compact, m)
		}
	}
	if len(compact) != 1 || len(compact[0]["fields"].([]any)) != 2 || compact[0]["type"] != "ui-view" {
		t.Errorf("compact extension = %+v", compact)
	}
}

// An inline schema needs no fetch; a ui block with no schema is refused.
func TestResolveComponentsInlineAndErrors(t *testing.T) {
	var calls []fetchCall
	if _, err := render.ResolveComponents(map[string]any{"kind": "form", "schema": bookSchema()}, fakeFetcher(&calls)); err != nil || len(calls) != 0 {
		t.Errorf("inline schema: err=%v calls=%v", err, calls)
	}
	if _, err := render.ResolveComponents(parseJSON(t, `{"kind":"view","ui":{"fields":["x"]}}`), fakeFetcher(&calls)); err == nil {
		t.Error("a ui block without a schema must be refused")
	}
}

// A form bound to an action's input names it {action: uri}: the fetcher is
// asked for that action's input, once per variant, and an inline schema is
// finished (its options computed) without a fetch.
func TestResolveComponentsByActionAndFinish(t *testing.T) {
	var calls []fetchCall
	finished := 0
	r := fakeFetcher(&calls)
	r.Finish = func(s map[string]any) (map[string]any, error) {
		finished++
		s["finished"] = true
		return s, nil
	}
	page := `{"kind":"page","dslTree":{"kind":"vbox","children":[
		{"kind":"form","schema":{"action":"ktw-agents:planification/file-request"}},
		{"kind":"list","item":{"kind":"form","schema":{"action":"ktw-agents:planification/file-request"}}},
		{"kind":"tabs","tabs":[{"label":"A","content":{"kind":"view","schema":{"name":"x","root":"X","types":{"X":{"type":"object"}}}}}]}]}}`
	out, err := render.ResolveComponents(parseJSON(t, page), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != (fetchCall{"the input of ktw-agents:planification/file-request", "ui-form", "*"}) {
		t.Errorf("one fetch of the action's input, got %+v", calls)
	}
	b, _ := json.Marshal(out)
	if strings.Count(string(b), `"finished":true`) != 3 || strings.Contains(string(b), `"action":"ktw-agents:planification`) {
		t.Errorf("every schema inline and finished (the fetched one once, copied): %s", b)
	}
	if finished != 2 {
		t.Errorf("finish runs once per fetched ref and once per inline schema, ran %d", finished)
	}
}

// A `slots` view and a compact table are payload of their
// type's extensions: resolved, each keeps its slots — a dotted path, a choice
// action with its value field — and its density, header, select and menu.
func TestResolveComponentsKeepsSlotsAndCompactTable(t *testing.T) {
	stream := `{"name":"rows","root":"Row","types":{
		"Row":{"type":"object","fields":{"id":{"type":"string","content":"object-id"},"release":{"type":"Card"}},"extensions":[
			{"type":"ui-view","name":"header","layout":"slots","slots":{"title":"id","footer":["release.gates"],
				"actions":[{"id":"mode","label":"Release mode","uri":"ktw-agents:streams/set","args":{"id":"${item.id}","release_mode":"${value}"},"choice":"manual|periodic","value":"mode"}]}},
			{"type":"ui-table","name":"default","density":"compact","header":false,"select":"increment",
				"columns":[{"id":"id","width":150}],
				"menu":[{"id":"hold","label":"Put on hold","uri":"ktw-agents:increments/hold","prompt":"Why does it wait?"},
					{"id":"take-out","label":"Take out","uri":"ktw-agents:increments/unship","confirm":"Take it out?"}]}]},
		"Card":{"type":"object","fields":{"gates":{"type":"array","content":"gate-dots","items":{"type":"object","fields":{"name":{"type":"string"}}}}}}}}`
	// The host's fetcher keeps the extensions of the asked kind and variant
	// (the schema service's GetDataTypeWithExtensions).
	r := render.ComponentResolver{Fetch: func(_ render.SchemaRef, ext, name string) (map[string]any, error) {
		var s map[string]any
		if err := json.Unmarshal([]byte(stream), &s); err != nil {
			return nil, err
		}
		for _, tv := range s["types"].(map[string]any) {
			tm := tv.(map[string]any)
			exts, _ := tm["extensions"].([]any)
			var kept []any
			for _, e := range exts {
				m := e.(map[string]any)
				if m["type"] == ext && (name == "*" || m["name"] == name) {
					kept = append(kept, m)
				}
			}
			tm["extensions"] = kept
		}
		return s, nil
	}}
	page := `{"kind":"page","dslTree":{"kind":"vbox","children":[
		{"kind":"view","schema":"ktw-agents:/data-type/rows","variant":"header"},
		{"kind":"table","schema":"ktw-agents:/data-type/rows"}]}}`
	out, err := render.ResolveComponents(parseJSON(t, page), r)
	if err != nil {
		t.Fatal(err)
	}
	kids := out.(map[string]any)["dslTree"].(map[string]any)["children"].([]any)
	ext := func(i int) map[string]any {
		exts := kids[i].(map[string]any)["schema"].(map[string]any)["types"].(map[string]any)["Row"].(map[string]any)["extensions"].([]any)
		if len(exts) != 1 {
			t.Fatalf("child %d: want its one extension, got %+v", i, exts)
		}
		return exts[0].(map[string]any)
	}
	view, _ := json.Marshal(ext(0))
	for _, want := range []string{`"layout":"slots"`, `"footer":["release.gates"]`, `"choice":"manual|periodic"`, `"value":"mode"`, `"release_mode":"${value}"`} {
		if !strings.Contains(string(view), want) {
			t.Errorf("slots view lost %s: %s", want, view)
		}
	}
	table, _ := json.Marshal(ext(1))
	for _, want := range []string{`"density":"compact"`, `"header":false`, `"select":"increment"`, `"prompt":"Why does it wait?"`, `"confirm":"Take it out?"`, `"width":150`} {
		if !strings.Contains(string(table), want) {
			t.Errorf("compact table lost %s: %s", want, table)
		}
	}
}

// A lane timeline draws its header and its cards with ui-view
// variants: resolved by reference, its schema keeps the ui-timeline-lanes
// variant AND the ui-view variants it names — the header on the lane type,
// the card on the item type — and no other ui-view; each fetched once.
func TestResolveComponentsKeepsTheViewsATimelineNames(t *testing.T) {
	doc := `{"name":"lanes","root":"Lane","types":{
		"Lane":{"type":"object","fields":{"id":{"type":"string"},"items":{"type":"array","items":"Item"}},"extensions":[
			{"type":"ui-timeline-lanes","name":"default","laneHeader":"head","items":"items","itemDate":"date","itemView":"card","select":"item"},
			{"type":"ui-view","name":"head","layout":"slots","slots":{"title":"id"}},
			{"type":"ui-view","name":"default"},
			{"type":"ui-table","name":"default"}]},
		"Item":{"type":"object","fields":{"id":{"type":"string"},"date":{"type":"string"}},"extensions":[
			{"type":"ui-view","name":"card","layout":"slots","slots":{"eyebrow":"id"}},
			{"type":"ui-view","name":"big"}]}}}`
	var calls []fetchCall
	r := render.ComponentResolver{Fetch: func(ref render.SchemaRef, ext, name string) (map[string]any, error) {
		calls = append(calls, fetchCall{ref.String(), ext, name})
		var s map[string]any
		if err := json.Unmarshal([]byte(doc), &s); err != nil {
			return nil, err
		}
		for _, tv := range s["types"].(map[string]any) {
			tm := tv.(map[string]any)
			var kept []any
			for _, e := range tm["extensions"].([]any) {
				m := e.(map[string]any)
				if m["type"] == ext && (name == "*" || m["name"] == name) {
					kept = append(kept, m)
				}
			}
			tm["extensions"] = kept
		}
		return s, nil
	}}
	page := `{"kind":"vbox","children":[
		{"kind":"timeline-lanes","schema":"ktw-agents:/data-type/lanes","data":[]},
		{"kind":"timeline-lanes","schema":"ktw-agents:/data-type/lanes","data":[]}]}`
	out, err := render.ResolveComponents(parseJSON(t, page), r)
	if err != nil {
		t.Fatal(err)
	}
	kids := out.(map[string]any)["children"].([]any)
	for i := range kids {
		types := kids[i].(map[string]any)["schema"].(map[string]any)["types"].(map[string]any)
		names := func(tn string) []string {
			var got []string
			for _, e := range types[tn].(map[string]any)["extensions"].([]any) {
				m := e.(map[string]any)
				got = append(got, m["type"].(string)+":"+m["name"].(string))
			}
			return got
		}
		if got := strings.Join(names("Lane"), ","); got != "ui-timeline-lanes:default,ui-view:head" {
			t.Errorf("child %d: lane type keeps %s", i, got)
		}
		if got := strings.Join(names("Item"), ","); got != "ui-view:card" {
			t.Errorf("child %d: item type keeps %s", i, got)
		}
	}
	want := []fetchCall{
		{"ktw-agents:/data-type/lanes", "ui-timeline-lanes", "*"},
		{"ktw-agents:/data-type/lanes", "ui-view", "card"},
		{"ktw-agents:/data-type/lanes", "ui-view", "head"},
	}
	sort.Slice(calls, func(a, b int) bool { return calls[a].ext+calls[a].name < calls[b].ext+calls[b].name })
	sort.Slice(want, func(a, b int) bool { return want[a].ext+want[a].name < want[b].ext+want[b].name })
	if len(calls) != len(want) {
		t.Fatalf("fetch calls = %+v, want %+v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("fetch calls = %+v, want %+v", calls, want)
			break
		}
	}
}

// A timeline-lanes node is a schema-bound kind on both sides.
func TestTimelineLanesIsSchemaBound(t *testing.T) {
	if !render.SchemaBoundKinds["timeline-lanes"] {
		t.Error("render.SchemaBoundKinds lacks timeline-lanes")
	}
	if err := render.ValidatePage(parseJSON(t, `{"kind":"timeline-lanes","schema":"ktw-agents:/data-type/stream-lane","data":[]}`)); err != nil {
		t.Errorf("ValidatePage: %v", err)
	}
}

// A list whose elements hold their own node (`nodeField`) has those nodes
// resolved too: an answer form inside a card arrives with its ui block laid
// over the schema - the « À faire » cards did not. Any other
// `data` stays the component's own payload.
func TestResolveComponentsWalksAListsNodeField(t *testing.T) {
	schema := map[string]any{"name": "answer", "root": "Answer", "types": map[string]any{"Answer": map[string]any{"type": "object", "fields": map[string]any{"text": map[string]any{"type": "string"}}}}}
	card := render.AnswerForm(schema, render.Action{Label: "Answer"}, "send", map[string]any{})
	list := render.List([]any{map[string]any{"id": "a", "card": card}}, nil, false).NodeField("card")
	raw, err := json.Marshal(render.Section("To do", "", list))
	if err != nil {
		t.Fatal(err)
	}
	tree := parseJSON(t, string(raw))
	if !render.HasComponents(tree) {
		t.Error("HasComponents must see the form a list element holds in its node field")
	}
	out, err := render.ResolveComponents(tree, render.ComponentResolver{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), `"ui":{`) {
		t.Errorf("the card's form still carries its ui block: %s", b)
	}
	// A list without nodeField keeps its data as payload, untouched.
	plain := parseJSON(t, `{"kind":"list","item":{"kind":"text","content":"x"},"data":[{"card":{"kind":"form","schema":{"root":"A","types":{"A":{"type":"object"}}},"ui":{"actions":[]}}}]}`)
	if render.HasComponents(plain) {
		t.Error("a list without nodeField must not look into its data")
	}
}

// A list naming its elements' data type by id (itemType) gets it inline, as
// the view each element is drawn by — fetched with its itemVariant's ui-view
// extension, default card —, a nested reference in the same tree too
func TestResolveComponentsInlinesAListsItemType(t *testing.T) {
	tree := parseJSON(t, `{"kind":"page","children":[
		{"kind":"list","id":"releases","itemType":"ktw-agents:/data-type/release-card","data":[{"id":"RL-033"},{"id":"RL-034"}]},
		{"kind":"list","id":"rows","itemType":"ktw-agents:/data-type/release-card","itemVariant":"compact","data":[]},
		{"kind":"section","children":[{"kind":"view","schema":"ktw-demo:/data-type/book"}]}]}`)
	if !render.HasComponents(parseJSON(t, `{"kind":"page","children":[{"kind":"list","itemType":"ktw-agents:/data-type/release-card","data":[]}]}`)) {
		t.Error("a list naming its itemType by id holds something to resolve")
	}
	var calls []fetchCall
	out, err := render.ResolveComponents(tree, fakeFetcher(&calls))
	if err != nil {
		t.Fatal(err)
	}
	kids := out.(map[string]any)["children"].([]any)
	for i := 0; i < 2; i++ {
		l := kids[i].(map[string]any)
		if _, ok := l["itemType"].(map[string]any); !ok {
			t.Errorf("list %d: itemType not inlined: %#v", i, l["itemType"])
		}
		if len(l["data"].([]any)) != 2*(1-i) {
			t.Errorf("list %d: data must pass through", i)
		}
	}
	nested := kids[2].(map[string]any)["children"].([]any)[0].(map[string]any)
	if _, ok := nested["schema"].(map[string]any); !ok {
		t.Errorf("nested view schema not inlined: %#v", nested["schema"])
	}
	want := map[fetchCall]bool{
		{"ktw-agents:/data-type/release-card", "ui-view", "card"}:    true,
		{"ktw-agents:/data-type/release-card", "ui-view", "compact"}: true,
		{"ktw-demo:/data-type/book", "ui-view", "*"}:                 true,
	}
	if len(calls) != len(want) {
		t.Fatalf("fetch calls = %+v", calls)
	}
	for _, c := range calls {
		if !want[c] {
			t.Errorf("unexpected fetch %+v", c)
		}
	}
}
