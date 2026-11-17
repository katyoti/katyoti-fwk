// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A dashboard built with the builders is a valid page and survives the wire
// unchanged: a section with its icon, count badge, aside and empty state; a
// toned badge with an icon and a tooltip; a stacked gauge with a legend; a
// collapsible markdown; a button and list actions.
func TestThePageBlocksRoundTripAndValidate(t *testing.T) {
	act := render.Action{Label: "Verify", Icon: "player-play", URI: "ktw-gocode:gocode/module-verify", Args: map[string]any{"id": "${id}"}}
	tree := render.Card(
		render.Section("Use-cases", "shield-check",
			render.Stacked("", render.Segment{Value: 3, Color: "green", Label: "pass"}, render.Segment{Value: 1, Color: "red", Label: "fail"}),
		).With("size", "sm").With("badge", 4).Aside(render.Badge("healthy", "green").With("icon", "circle-check").With("tooltip", "all good")),
		render.Section("Promote gates", "clock").Empty("All gates clear", "green"),
		render.Markdown("# long\n\ntext").Collapsible(120),
		render.Button("Verify", act),
		render.List([]any{map[string]any{"path": "a/b"}}, render.Badge("", "gray").With("dataField", "$.item.path").With("mono", true), true, act),
	)
	def := render.Def{Kind: "page", DSLTree: tree}
	if err := def.Validate(); err != nil {
		t.Fatalf("a built dashboard must validate: %v", err)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	var back render.Def
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("the decoded page must validate: %v", err)
	}
	s := string(raw)
	for _, want := range []string{
		`"kind":"section"`, `"icon":"shield-check"`, `"badge":4`, `"aside":[`, `"emptyColor":"green"`,
		`"segments":[{"value":3,"color":"green","label":"pass"}`, `"legend":true`, `"maxHeight":120`,
		`"tooltip":"all good"`, `"mono":true`, `"variant":"cards"`, `"uri":"ktw-gocode:gocode/module-verify"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the wire form lacks %s: %s", want, s)
		}
	}
}

// Validation names what is wrong and where.
func TestValidatePageRefusesWhatCannotRender(t *testing.T) {
	for name, tc := range map[string]struct {
		tree any
		want string
	}{
		"unknown kind":        {render.VBox(render.Node{"kind": "sparkle"}), `unknown kind "sparkle"`},
		"badge without text":  {render.Card(render.Node{"kind": "badge"}), `"text" is required`},
		"empty gauge":         {render.Node{"kind": "gauge", "label": "x"}, "needs value or segments"},
		"negative segment":    {render.Stacked("x", render.Segment{Value: -1}), "segment 0"},
		"action without verb": {render.Button("go", render.Action{Label: "go"}), "exactly one of uri, navigate, chat, create"},
		"bad uri":             {render.Button("go", render.RunURI("go", "nothing", nil)), "an action needs its uri"},
		"two verbs":           {render.Button("go", render.Action{URI: "a:b/c", Navigate: &render.Navigate{Object: "m.x"}}), "got [uri, navigate]"},
		"navigate no object":  {render.Link("x", render.Action{Navigate: &render.Navigate{Panel: "p"}}), "needs its object"},
		"chat no task":        {render.Button("go", render.Action{Chat: &render.Chat{Target: "m.x"}}), "needs its task"},
		"link without either": {render.Node{"kind": "link", "text": "x"}, "an href or an action"},
		"link with both":      {render.Node{"kind": "link", "text": "x", "href": "http://a.test", "action": render.NavigateTo("", "m.x", "")}, "an href or an action"},
		"bad lane move":       {render.Node{"kind": "lanes", "lanes": []any{map[string]any{"id": "a", "title": "A", "move": map[string]any{}}}}, "exactly one of"},
		"bad node in aside":   {render.Section("s", "").Aside(render.Node{"kind": "nope"}), `aside[0]`},
		"list item unchecked": {render.List(nil, render.Node{"kind": "heading"}, false), `"text" is required`},
		"page without a tree": {nil, "needs its dslTree"},
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			if tc.tree == nil {
				err = render.Def{Kind: "page"}.Validate()
			} else {
				err = render.ValidatePage(tc.tree)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
	// A node bound by dataField takes its main prop from the data.
	if err := render.ValidatePage(render.Node{"kind": "badge", "dataField": "$.state"}); err != nil {
		t.Errorf("a bound badge is valid: %v", err)
	}
}

// The Go list of page kinds is the display library's: a kind added to the
// renderer and not here (or the reverse) would make ValidatePage refuse a
// page the GUI draws, or accept one it cannot.
func TestPageKindsAreTheDisplayLibrarys(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "app", "ktw-display-lib", "src", "ui-builders", "page", "ui-page.types.ts"))
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]string{"layout": "LAYOUT_KINDS", "atomic": "ATOMIC_KINDS", "schema-bound": "SCHEMA_BOUND_KINDS", "interactive": "INTERACTIVE_KINDS"}
	quoted := regexp.MustCompile(`'([a-z-]+)'`)
	for cat, name := range consts {
		m := regexp.MustCompile(`(?s)export const ` + name + ` = \[(.*?)\]`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("%s not found in ui-page.types.ts", name)
		}
		var ts []string
		for _, q := range quoted.FindAllSubmatch(m[1], -1) {
			ts = append(ts, string(q[1]))
		}
		goKinds := append([]string(nil), render.PageKinds[cat]...)
		sort.Strings(ts)
		sort.Strings(goKinds)
		if strings.Join(ts, ",") != strings.Join(goKinds, ",") {
			t.Errorf("%s: display library %v, render.PageKinds %v", cat, ts, goKinds)
		}
	}
}

// Each verb of an action is a valid page action on its own: open an object or
// one of its panels (a link, a badge), a chat, the new-object dialog, a method
// — and the wire form carries the verb the host dispatches on.
func TestActionVerbsValidateAndRoundTrip(t *testing.T) {
	tree := render.VBox(
		render.Link("Open the feature", render.NavigateTo("", "${item.id}", "dashboard")),
		render.Badge("blocked", "red").With("action", render.NavigateTo("Why", "${id}", "")),
		render.Button("Ask", render.ChatOn("Ask", "ktw-agents:tasks/chat", "${id}")),
		render.Button("New use-case", render.CreateUnder("New", "use-case", "")),
		render.Button("Verify", render.RunURI("Verify", "ktw-gocode:gocode/module-verify", map[string]any{"id": "${id}"})),
		render.Button("By state", render.ShowTab("By state", "by-state")),
		render.Button("Annuler", render.CloseHere("Annuler")),
	)
	if err := render.ValidatePage(tree); err != nil {
		t.Fatalf("every verb is a valid action: %v", err)
	}
	raw, _ := json.Marshal(tree)
	for _, want := range []string{
		`"navigate":{"object":"${item.id}","panel":"dashboard"}`, `"chat":{"task":"ktw-agents:tasks/chat","target":"${id}"}`,
		`"create":{"type":"use-case"}`, `"uri":"ktw-gocode:gocode/module-verify"`, `"tab":"by-state"`, `"close":true`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the wire form lacks %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), `"uri":""`) {
		t.Errorf("a verb other than uri must not carry an empty uri: %s", raw)
	}
	// The result convention a method answers with.
	b, _ := json.Marshal(render.Result{OK: true, Message: "Filed", Open: "billing.rq-1#dashboard"})
	if string(b) != `{"ok":true,"message":"Filed","open":"billing.rq-1#dashboard"}` {
		t.Errorf("result wire form: %s", b)
	}
	// A result carrying its inverse: the action taking the change back.
	undo := render.RunURI("Undo", "ktw-agents:requests/assess", map[string]any{"id": "IN-1", "priority": "P2"})
	b, _ = json.Marshal(render.Result{OK: true, Message: "IN-1 moved to P0", Undo: &undo})
	if !strings.Contains(string(b), `"undo":{`) || !strings.Contains(string(b), `"uri":"ktw-agents:requests/assess"`) || !strings.Contains(string(b), `"priority":"P2"`) {
		t.Errorf("result with undo wire form: %s", b)
	}
}

// A table's row opens what it is and carries its actions: both are actions
// ValidatePage holds to the grammar.
func TestTableRowsOpenAndAct(t *testing.T) {
	rows := []any{map[string]any{"id": "m.a", "title": "A"}}
	tbl := render.Table("ktw-demo:/data-type/book", rows,
		render.RunURI("Close", "ktw-agents:cwm/object-transition", map[string]any{"id": "${item.id}"}),
	).Open(render.NavigateTo("", "${item.id}", ""))
	if err := render.ValidatePage(render.VBox(tbl)); err != nil {
		t.Fatalf("a table with open and row actions: %v", err)
	}
	if err := render.ValidatePage(render.Table("x:/data-type/y", rows).Open(render.Action{Label: "nothing"})); err == nil {
		t.Error("a row open with no verb must be refused")
	}
	raw, _ := json.Marshal(tbl)
	if !strings.Contains(string(raw), `"open":{"navigate":{"object":"${item.id}"}}`) || !strings.Contains(string(raw), `"actions":[{"label":"Close"`) {
		t.Errorf("wire form: %s", raw)
	}
}

// A form over an action's input whose submit runs it: the schema names the
// action, the submit is an action ValidatePage holds to the grammar.
func TestAFormSubmitsToAnAction(t *testing.T) {
	f := render.ActionForm("File", "ktw-agents:planification/file-request")
	if err := render.ValidatePage(render.VBox(f)); err != nil {
		t.Fatalf("an action form: %v", err)
	}
	raw, _ := json.Marshal(f)
	if !strings.Contains(string(raw), `"schema":{"action":"ktw-agents:planification/file-request"}`) || !strings.Contains(string(raw), `"submit":{"label":"File","uri":"ktw-agents:planification/file-request"}`) {
		t.Errorf("wire form: %s", raw)
	}
	if err := render.ValidatePage(render.Form("x:/data-type/y", render.Action{Label: "Save"})); err == nil {
		t.Error("a submit naming no verb must be refused")
	}
}

// A query re-reads the panel's source with its args over the input — a chip
// setting a filter, a form laying its whole value (no args) — and an accordion
// folds its panels, each walked by ValidatePage.
func TestQueryVerbAndAccordion(t *testing.T) {
	tree := render.VBox(
		render.Badge("goal 3", "teal").With("action", render.QueryWith("Only goals", map[string]any{"type": "goal"})),
		render.Form(map[string]any{"root": "F", "types": map[string]any{}}, render.QueryWith("Search", nil)),
		render.Accordion([]render.Fold{{ID: "goal", Label: "Goals", Badge: 3, Content: render.VBox(render.Text("a"))}}, "goal"),
	)
	if err := render.ValidatePage(tree); err != nil {
		t.Fatalf("a query and an accordion are valid: %v", err)
	}
	raw, _ := json.Marshal(tree)
	for _, want := range []string{`"query":{"args":{"type":"goal"}}`, `"submit":{"label":"Search","query":{}}`, `"defaultOpen":["goal"]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the wire form lacks %s: %s", want, raw)
		}
	}
	bad := render.Accordion([]render.Fold{{Label: "x", Content: render.Node{"kind": "nope"}}})
	if err := render.ValidatePage(bad); err == nil {
		t.Error("a fold's content is walked: an unknown kind must be refused")
	}
	if err := render.CheckAction(map[string]any{"query": "x"}); err == nil {
		t.Error("a query that is not an object must be refused")
	}
}

// The verbs of an action are the display library's ACTION_VERBS.
func TestActionVerbsAreTheDisplayLibrarys(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "app", "ktw-display-lib", "src", "ui-builders", "page", "ui-page.interactive.ts"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`export const ACTION_VERBS = \[(.*?)\]`).FindSubmatch(src)
	if m == nil {
		t.Fatal("ACTION_VERBS not found in ui-page.interactive.ts")
	}
	var ts []string
	for _, q := range regexp.MustCompile(`'([a-z-]+)'`).FindAllSubmatch(m[1], -1) {
		ts = append(ts, string(q[1]))
	}
	if strings.Join(ts, ",") != strings.Join(render.ActionVerbs, ",") {
		t.Errorf("display library %v, render.ActionVerbs %v", ts, render.ActionVerbs)
	}
}

// A tab action names the panel it shows.
func TestATabActionNamesItsPanel(t *testing.T) {
	if err := render.CheckAction(map[string]any{"tab": "by-state"}); err != nil {
		t.Errorf("a tab action: %v", err)
	}
	if err := render.CheckAction(map[string]any{"tab": 3}); err == nil {
		t.Errorf("a tab action naming no panel: want refused")
	}
	if err := render.CheckAction(map[string]any{"tab": "by-state", "uri": "a:b/c"}); err == nil {
		t.Errorf("a tab action with a second verb: want refused")
	}
}

// A tabs node's select runs when a tab is chosen: it is checked as an action
// - one naming no verb is refused.
func TestATabsSelectIsCheckedAsAnAction(t *testing.T) {
	tabs := func(sel any) map[string]any {
		return map[string]any{"kind": "tabs", "select": sel, "tabs": []any{
			map[string]any{"id": "a", "label": "A", "content": map[string]any{"kind": "text", "content": "a"}},
		}}
	}
	if err := render.ValidatePage(tabs(map[string]any{"label": "Show", "query": map[string]any{"args": map[string]any{"tab": "${item.id}"}}})); err != nil {
		t.Errorf("a tabs node whose select re-reads the source is refused: %v", err)
	}
	if err := render.ValidatePage(tabs(map[string]any{"label": "Nothing"})); err == nil {
		t.Error("a tabs node whose select names no verb is accepted")
	}
	// The pick tree is gone: its kind is refused.
	pick := map[string]any{"kind": "pick-tree", "browse": "x:browse", "select": "x:select", "object": "o"}
	if err := render.ValidatePage(pick); err == nil {
		t.Error("a pick-tree, deleted, is accepted")
	}
}

// A split holds a main and a side, each walked as a node; the part of a page
// fed by its own source is a loaded section, the region kind is gone
func TestASplitAndALoadedSectionValidate(t *testing.T) {
	region := render.LoadedSection("release", render.Load{Method: "ktw-methods:delivery/release", Args: map[string]any{"release": "${selection.release}"}}, render.Placeholder{Kind: render.PlaceholderRows, Count: 4}, "Pick a release")
	split := map[string]any{"kind": "split", "main": map[string]any{"kind": "text", "content": "streams"}, "side": region}
	if err := render.ValidatePage(split); err != nil {
		t.Errorf("a split with a loaded section on its side: %v", err)
	}
	if region["kind"] != "box" || region["empty"] != "Pick a release" {
		t.Errorf("a loaded section is an empty box with its empty text: %v", region)
	}
	for name, n := range map[string]map[string]any{
		"a split without its side":      {"kind": "split", "main": map[string]any{"kind": "text", "content": "x"}},
		"a split without its main":      {"kind": "split", "side": region},
		"a region, deleted":             {"kind": "region", "id": "r", "source": "a:b/c"},
		"a loaded section without id":   {"kind": "box", "children": []any{}, "load": map[string]any{"method": "a:b/c"}},
		"a split whose side is unknown": {"kind": "split", "main": map[string]any{"kind": "text", "content": "x"}, "side": map[string]any{"kind": "nope"}},
	} {
		if err := render.ValidatePage(n); err == nil {
			t.Errorf("%s: want refused", name)
		}
	}
}

// A select action names the key it sets; its builder makes a valid one, and
// `${selection.<key>}` is a host template the object pane leaves as written.
func TestASelectActionNamesItsKey(t *testing.T) {
	if err := render.CheckAction(render.SelectOn("Open", "release", "${item.id}")); err != nil {
		t.Errorf("a select action: %v", err)
	}
	if err := render.CheckAction(map[string]any{"select": map[string]any{"value": "x"}}); err == nil {
		t.Error("a select action naming no key: want refused")
	}
	if err := render.CheckAction(map[string]any{"select": map[string]any{"key": "k"}, "tab": "t"}); err == nil {
		t.Error("a select action with a second verb: want refused")
	}
	if !render.IsHostTemplate("selection.release") {
		t.Error("selection.release is not a host template")
	}
}

// An inline select and an action menu are page nodes whose
// actions are checked: the select's action, each menu entry's — its own
// `action`, or the verb on the entry itself; a prompt is a string, a choice a
// string or an object.
func TestASelectAndAMenuValidateTheirActions(t *testing.T) {
	assess := render.RunURI("Priority", "ktw-agents:requests/assess", map[string]any{"id": "${item.id}", "priority": "${value}"})
	assess.ID = "priority"
	hold := render.RunURI("Put on hold", "ktw-agents:increments/hold", map[string]any{"id": "${item.id}", "reason": "${value}"})
	hold.ID = "hold"
	good := render.VBox(
		render.InlineSelect("P1", "P0|P1|P2", assess),
		render.Menu("", render.MenuEntry(hold, "Why does it wait?", ""), render.MenuEntry(assess, "", "P0|P1|P2|P3")),
		render.Menu("More", map[string]any{"id": "take-out", "label": "Take out", "uri": "ktw-agents:increments/unship", "confirm": "Sure?"},
			map[string]any{"id": "move", "label": "Change release", "uri": "ktw-agents:increments/plan-for", "choice": map[string]any{"optionsField": "options"}}),
	)
	if err := render.ValidatePage(good); err != nil {
		t.Fatalf("a select and a menu: %v", err)
	}
	entry := func(e map[string]any) render.Node { return render.Menu("", e) }
	for name, bad := range map[string]render.Node{
		"a select without its action":           {"kind": "select", "options": []any{}},
		"a select whose action has none":        {"kind": "select", "action": map[string]any{"label": "x"}},
		"a menu without its items":              {"kind": "menu"},
		"a menu entry without an action":        entry(map[string]any{"id": "a", "label": "A"}),
		"a menu entry without its id":           entry(map[string]any{"label": "A", "uri": "p:s/m"}),
		"a menu entry with a bad uri":           entry(map[string]any{"id": "a", "label": "A", "action": map[string]any{"uri": "nope"}}),
		"a prompt that is not a string":         entry(map[string]any{"id": "a", "label": "A", "uri": "p:s/m", "prompt": 3}),
		"a choice that is a number":             entry(map[string]any{"id": "a", "label": "A", "uri": "p:s/m", "choice": 3}),
		"a choice whose options are not a list": entry(map[string]any{"id": "a", "label": "A", "uri": "p:s/m", "choice": map[string]any{"options": "x"}}),
	} {
		if err := render.ValidatePage(bad); err == nil {
			t.Errorf("%s: want refused", name)
		}
	}
}

// : a board names the selection key its rows set and the panel
// drawn beside; its toggle is checked as an action, its panel as a node.
func TestABoardValidatesItsPanelAndToggle(t *testing.T) {
	region := render.LoadedSection("source", render.Load{Method: "ktw-agents:sources/source-page", Args: map[string]any{"connector": "${selection.connector}"}}, render.Placeholder{Kind: render.PlaceholderSkeleton}, "")
	use := render.RunURI("Use", "ktw-agents:sources/use", map[string]any{"connector": "${item.key}", "on": "${value}"})
	good := render.Node{"kind": "board", "select": "connector", "dataField": "$.connectors", "panel": region, "toggle": use}
	if err := render.ValidatePage(good); err != nil {
		t.Fatalf("a board: %v", err)
	}
	for name, bad := range map[string]render.Node{
		"a board without its panel":          {"kind": "board", "select": "connector"},
		"a board without its select key":     {"kind": "board", "panel": region},
		"a board whose toggle has no verb":   {"kind": "board", "select": "connector", "panel": region, "toggle": map[string]any{"label": "x"}},
		"a board whose panel is off grammar": {"kind": "board", "select": "connector", "panel": map[string]any{"kind": "no-such-kind"}},
	} {
		if err := render.ValidatePage(bad); err == nil {
			t.Errorf("%s validated", name)
		}
	}
}

// : a grid board names its rows and columns; its move, its open
// its card actions and each column's own move are checked as actions, its card
// template as a node.
func TestAGridBoardValidatesItsCellsAndMoves(t *testing.T) {
	assess := render.RunURI("Priority", "ktw-agents:requests/assess", map[string]any{"id": "${item.id}", "priority": "${column}"})
	hold := render.RunURI("Hold", "ktw-agents:increments/hold", map[string]any{"id": "${item.id}", "reason": "${value}"})
	rows := []any{map[string]any{"id": "SM-001", "title": "KaTyoti"}}
	cols := []any{map[string]any{"id": "P0", "title": "P0"}, map[string]any{"id": "held", "title": "On hold", "prompt": "Why?", "move": hold}}
	good := render.Node{"kind": "grid-board", "rows": rows, "columns": cols, "move": assess, "item": render.Node{"kind": "text", "content": "x"}}
	if err := render.ValidatePage(good); err != nil {
		t.Fatalf("a grid board: %v", err)
	}
	for name, bad := range map[string]render.Node{
		"a grid board without rows":    {"kind": "grid-board", "columns": cols},
		"a grid board without columns": {"kind": "grid-board", "rows": rows},
		"a column move with no verb":   {"kind": "grid-board", "rows": rows, "columns": []any{map[string]any{"id": "x", "title": "X", "move": map[string]any{"label": "x"}}}},
		"a card template off grammar":  {"kind": "grid-board", "rows": rows, "columns": cols, "item": map[string]any{"kind": "no-such-kind"}},
	} {
		if err := render.ValidatePage(bad); err == nil {
			t.Errorf("%s validated", name)
		}
	}
}

func TestPageFiltersSetTheSelectionAndFilterARowsNode(t *testing.T) {
	n := 2
	page := render.Node{"kind": "vbox", "children": []any{
		render.Chips("stream", "All streams", render.ChipOption{Value: "SM-001", Label: "KaTyoti", Count: &n}),
		render.Search("q", "Find an increment"),
		render.Switch("show_held", "Show held increments"),
		render.Node{"kind": "list", "dataField": "$.rows", "item": render.Node{"kind": "text", "content": "x"},
			"filter": []any{render.FilterEquals("stream", "stream", "none"), render.FilterContains("q", "title", "id"), render.FilterUnless("show_held", "held")}},
	}}
	if err := render.ValidatePage(page); err != nil {
		t.Fatalf("page filters: %v", err)
	}
	for name, bad := range map[string]render.Node{
		"chips without a key":    {"kind": "chips", "options": []any{}},
		"a search without a key": {"kind": "search", "placeholder": "x"},
		"a switch without label": {"kind": "switch", "key": "k"},
	} {
		if err := render.ValidatePage(bad); err == nil {
			t.Errorf("%s validated", name)
		}
	}
	// A chip group bound to its rows keeps its key.
	if err := render.ValidatePage(render.Node{"kind": "chips", "key": "stream", "dataField": "$.rows"}); err != nil {
		t.Errorf("bound chips: %v", err)
	}
}

// A CREATE PAGE PICKS ONE OPTION, NAMES IT AND CREATES:
// choice cards, a text field shown under a condition and a button requiring
// their keys validate, a multi-line text field too; a choice
// card without its value, a text field without its label or with rows that is
// no number of lines and requires that is no list of keys are refused.
func TestAChoiceCardsTextFieldAndRequiringButtonValidate(t *testing.T) {
	page := render.VBox(
		render.ChoiceCards("connector", "Connecteur",
			render.ChoiceCard{Value: "leanix-application", Label: "LeanIX", Color: "teal", Badge: "LX"},
			render.ChoiceCard{Value: "documents", Label: "Documents", Disabled: true, Pill: "déjà dans l'étude", Tooltip: "Une seule par étude"}),
		render.TextField("name", "Nom", "LeanIX"),
		render.TextField("space", "Espace", "").ShowWhen("connector=confluence"),
		render.TextField("remark", "Remarque", "").Rows(4),
		render.Button("Créer", render.Action{URI: "ktw-agents:sources/use", Args: map[string]any{"connector": "${selection.connector}"}}).
			Requires("Choisissez un connecteur et un nom", "connector", "name"),
		render.Button("Suivante", render.Action{URI: "ktw-agents:studies/next"}).With("icon", "chevron-right").IconAfter(),
	)
	if err := render.ValidatePage(page); err != nil {
		t.Fatalf("create page: %v", err)
	}
	for name, bad := range map[string]render.Node{
		"choice cards without a key":      {"kind": "choice-cards", "options": []any{}},
		"a choice card without a value":   render.ChoiceCards("c", "", render.ChoiceCard{Label: "x"}),
		"a text field without a label":    {"kind": "text-field", "key": "k"},
		"a text field of zero rows":       render.TextField("k", "K", "").Rows(0),
		"a text field whose rows is text": {"kind": "text-field", "key": "k", "label": "K", "rows": "4"},
		"requires that is no list":        {"kind": "button", "label": "x", "action": map[string]any{"uri": "ktw-agents:sources/use"}, "requires": "name"},
		"requires with an empty key":      {"kind": "button", "label": "x", "action": map[string]any{"uri": "ktw-agents:sources/use"}, "requires": []any{""}},
		"an icon position neither side":   {"kind": "button", "label": "x", "action": map[string]any{"uri": "ktw-agents:studies/next"}, "iconPosition": "top"},
	} {
		if err := render.ValidatePage(bad); err == nil {
			t.Errorf("%s validated", name)
		}
	}
}

// A GRAPH DRAWS FRAMES AND STATUS CARDS: a box in a group's
// frame, a dotted external box, a status and badges validate; a frame other
// than solid or dotted, a group naming no box and badges that are not texts
// are refused.
func TestAGraphWithFramesAndStatusCardsValidates(t *testing.T) {
	graph := func(boxes ...map[string]any) render.Node {
		nodes := make([]any, len(boxes))
		for i, b := range boxes {
			nodes[i] = b
		}
		return render.Node{"kind": "graph", "data": map[string]any{"nodes": nodes, "edges": []any{map[string]any{"from": "IN-1", "to": "EXT"}}}}
	}
	ok := graph(
		map[string]any{"id": "IN-1", "title": "Delivery overview", "badges": []any{"ready to run"}},
		map[string]any{"id": "RQ-1", "title": "Tabs", "group": "IN-1", "status": "running"},
		map[string]any{"id": "EXT", "title": "m.RQ-9", "frame": "dotted", "status": "waiting"},
	)
	if err := render.ValidatePage(ok); err != nil {
		t.Fatalf("a graph with a frame, a dotted box, statuses and badges: %v", err)
	}
	for name, tc := range map[string]struct {
		box  map[string]any
		want string
	}{
		"frame":  {map[string]any{"id": "a", "title": "A", "frame": "dashed"}, "frame is solid or dotted"},
		"group":  {map[string]any{"id": "a", "title": "A", "group": "nowhere"}, "is no box of the graph"},
		"badges": {map[string]any{"id": "a", "title": "A", "badges": []any{1}}, "badges are a list of texts"},
	} {
		if err := render.ValidatePage(graph(tc.box)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v; want %q", name, err, tc.want)
		}
	}
}

// A GOAL BAR LISTS ITS REQUESTS, A STEPPER DRAWS DOTS: a
// stacked gauge's segments carry the items they count and a stepper the dots
// variant, each step its own state; an item without its id and a variant
// other than steps or dots are refused.
func TestASegmentListsItsItemsAndAStepperDrawsDots(t *testing.T) {
	bar := render.Stacked("I send invoices by mail",
		render.Segment{Value: 1, Label: "done", Items: []render.SegmentItem{{ID: "billing.RQ-1", Title: "Mail invoices"}}},
		render.Segment{Value: 2, Label: "running", Items: []render.SegmentItem{{ID: "billing.RQ-2"}, {ID: "billing.RQ-3"}}})
	dots := render.Node{"kind": "stepper", "variant": "dots", "steps": []any{
		map[string]any{"id": "intake", "label": "intake", "status": "done"},
		map[string]any{"id": "implement", "label": "implement", "status": "running"},
	}}
	if err := render.ValidatePage(render.VBox(bar, dots)); err != nil {
		t.Fatalf("a bar listing its items and a stepper of dots: %v", err)
	}
	raw, _ := json.Marshal(bar)
	if !strings.Contains(string(raw), `"items":[{"id":"billing.RQ-1","title":"Mail invoices"}]`) {
		t.Errorf("the segment's items: %s", raw)
	}
	noID := render.Stacked("x", render.Segment{Value: 1, Items: []render.SegmentItem{{Title: "no id"}}})
	if err := render.ValidatePage(noID); err == nil || !strings.Contains(err.Error(), "item 0 needs its id") {
		t.Errorf("an item without its id: %v", err)
	}
	if err := render.ValidatePage(render.Node{"kind": "stepper", "variant": "pills", "steps": []any{}}); err == nil || !strings.Contains(err.Error(), "steps or dots") {
		t.Errorf("a variant other than steps or dots: %v", err)
	}
}

// A kpi tile's one variant is inline (: the deck's « Needs
// you », the label bold with the count on its line); another is refused.
func TestValidatePageKPIVariant(t *testing.T) {
	tile := render.KPI("Needs you", 3, "grape", "").With("variant", "inline")
	if err := render.ValidatePage(render.VBox(tile, render.KPI("Open", 7, "indigo", "stack-2"))); err != nil {
		t.Fatalf("an inline tile and a plain one: %v", err)
	}
	if err := render.ValidatePage(render.KPI("Open", 7, "", "").With("variant", "big")); err == nil || !strings.Contains(err.Error(), "variant is inline") {
		t.Errorf("a kpi variant other than inline: %v", err)
	}
}
