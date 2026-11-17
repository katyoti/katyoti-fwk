// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE DETAIL PANE, on a real CWM: the tab strip is declared by panels (each
// once, in order, by type, capability and `when`) and derived from the type's
// fields and the capabilities' schemas; contents are resolved against the
// object; aspects, links, children, badges, errors and the body come as CWM
// read them.

// paneWorld runs CWM with the base pack plus files (path → YAML), and extra
// extensions, and creates the module `m`.
func paneWorld(t *testing.T, files map[string]string, extra ...application.Extension) (context.Context, cwmapi.CWMService, api.ObjectPaneService) {
	t.Helper()
	pack := fstest.MapFS{
		"object-types/module.yaml": {Data: []byte("name: module\ntree: node\n")},
	}
	for p, body := range files {
		pack[p] = &fstest.MapFile{Data: []byte(body)}
	}
	exts := append([]application.Extension{cwmapp.Pack(pack), panelocal.Extension()}, extra...)
	ctx, cwm := cwmapp.New(t, exts...)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc
}

func create(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) {
	t.Helper()
	if o.Parent == "" && o.ID != "m" {
		o.Parent = "m"
	}
	if o.Name == "" {
		o.Name = o.ID
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("create %s: %v", o.ID, err)
	}
}

func detail(t *testing.T, ctx context.Context, svc api.ObjectPaneService, id string, body bool) api.DetailOutput {
	t.Helper()
	d, err := svc.Detail(ctx, api.DetailInput{ID: id, Body: body})
	if err != nil {
		t.Fatalf("Detail(%s): %v", id, err)
	}
	return d
}

func ids(ps []api.ObjectPanel) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func panelByID(ps []api.ObjectPanel) map[string]api.ObjectPanel {
	out := map[string]api.ObjectPanel{}
	for _, p := range ps {
		out[p.ID] = p
	}
	return out
}

const ext = "content: {kind: extension, extension: x}\n"

// A panel declares where it shows; a marker capability draws nothing; a
// capability with a schema no matched panel names gets its fields section.
func TestPanelsAreDeclaredNotOnePerCapability(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/marker.yaml":     "name: marker\n",
		"capabilities/content.yaml":    "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"capabilities/confluence.yaml": "name: confluence\nlabel: Confluence\nschema: sys:/data-type/confluence\n",
		"object-types/thing.yaml":      "name: thing\ntree: node\ncapabilities: [marker, content, confluence]\n",
		"object-panels/confluence-sync.yaml": "name: Sync\nicon: {tabler: cloud}\nfilter: {capabilities: [confluence]}\n" +
			"content: {kind: extension, extension: method-panel}\n",
		"object-panels/confluence-pages.yaml": "name: Pages\nfilter: {capabilities: [confluence]}\nsource: 'ktw-acme:confluence/pages'\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	panels := detail(t, ctx, svc, "m.thing", true).Panels
	by := panelByID(panels)
	if len(panels) != 3 {
		t.Fatalf("strip %v, want content derived + the two declared panels", ids(panels))
	}
	if _, ok := by["section:cap/marker"]; ok {
		t.Errorf("a marker drew a tab: %v", ids(panels))
	}
	if p := by["section:cap/content"]; p.Kind != api.PanelKindFields || p.Schema == "" || p.Name != "Content" || !p.Section {
		t.Errorf("derived fields section: %+v", p)
	}
	if p := by["tab:confluence-sync"]; p.Kind != api.PanelKindContent || p.Name != "Sync" || p.Icon == nil || p.Icon.Tabler != "cloud" {
		t.Errorf("content panel: %+v", p)
	}
	if p := by["tab:confluence-pages"]; p.Kind != api.PanelKindSource || p.Source == "" {
		t.Errorf("source panel: %+v", p)
	}
	if _, ok := by["section:cap/confluence"]; ok {
		t.Errorf("a capability a matched panel covers also got a fields section: %v", ids(panels))
	}
}

// A capability nobody declares is listed, never given a section.
func TestPanelsSkipAnUndeclaredCapability(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/content.yaml": "name: content\nschema: sys:/data-type/content\n",
		"object-types/thing.yaml":   "name: thing\ntree: node\ncapabilities: [content]\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing", Capabilities: map[string]map[string]any{"ghost": {"x": 1}}})
	d := detail(t, ctx, svc, "m.thing", true)
	if strings.Join(ids(d.Panels), ",") != "section:cap/content" {
		t.Errorf("panels %v, want only the declared capability's", ids(d.Panels))
	}
	if !strings.Contains(strings.Join(d.Capabilities, ","), "ghost") {
		t.Errorf("the undeclared capability is not listed: %v", d.Capabilities)
	}
}

// A panel matching twice — two capabilities, or one id from two providers —
// is one tab (goal object-pane.g11 AC2).
func TestPanelShownAtMostOnce(t *testing.T) {
	second := fstest.MapFS{"object-panels/both.yaml": {Data: []byte("name: Both again\nfilter: {types: ['*']}\n" + ext)}}
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/want.yaml":      "name: want\n",
		"capabilities/checkable.yaml": "name: checkable\n",
		"object-types/thing.yaml":     "name: thing\ntree: node\ncapabilities: [want, checkable]\n",
		"object-panels/both.yaml":     "name: Both\nfilter: {capabilities: [want, checkable]}\n" + ext,
	}, manifestcontrib.ContributePackPart("ktw-other", "other", "cwm", second))
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	if got := ids(detail(t, ctx, svc, "m.thing", true).Panels); strings.Join(got, ",") != "tab:both" {
		t.Errorf("strip %v, want the panel exactly once", got)
	}
}

// Order wins, then name; an undeclared order is the derived sections' order
// (goal object-pane.g11 AC3).
func TestPanelsOrderedByOrderThenName(t *testing.T) {
	all := "filter: {types: ['*']}\n" + ext
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/content.yaml": "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"object-types/thing.yaml":   "name: thing\ntree: node\ncapabilities: [content]\n",
		"object-panels/late.yaml":   "name: Late\norder: 500\n" + all,
		"object-panels/zulu.yaml":   "name: Zulu\n" + all,
		"object-panels/first.yaml":  "name: First\norder: 10\n" + all,
		"object-panels/alpha.yaml":  "name: Alpha\n" + all,
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	got := strings.Join(ids(detail(t, ctx, svc, "m.thing", true).Panels), ",")
	if want := "tab:first,tab:alpha,section:cap/content,tab:zulu,tab:late"; got != want {
		t.Errorf("order %s, want %s", got, want)
	}
}

// The filter's types and `when` decide; a `when` that cannot be read hides.
func TestPanelFilterWhenAndTypes(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":        "name: thing\ntree: node\n",
		"object-panels/typed.yaml":       "name: Typed\nfilter: {types: [thing]}\n" + ext,
		"object-panels/other-type.yaml":  "name: Other\nfilter: {types: [note]}\n" + ext,
		"object-panels/when-true.yaml":   "name: W1\nfilter: {when: \"current.type == 'thing'\"}\n" + ext,
		"object-panels/when-false.yaml":  "name: W2\nfilter: {when: \"current.type == 'note'\"}\n" + ext,
		"object-panels/when-broken.yaml": "name: W3\nfilter: {when: \"node.type == 'thing'\"}\n" + ext,
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	got := strings.Join(ids(detail(t, ctx, svc, "m.thing", true).Panels), ",")
	if got != "tab:typed,tab:when-true" && got != "tab:when-true,tab:typed" {
		t.Errorf("strip %s, want typed and when-true only", got)
	}
}

// `${...}` in a content panel is resolved on the server; an unresolvable one
// is empty, never the template; an extension's input is passed verbatim.
func TestPanelContentResolvesAgainstCurrent(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\n",
		"object-panels/hello.yaml": "name: Hello\nfilter: {types: ['*']}\n" +
			"content: {kind: markdown, title: '${current.name}', data: {id: '${current.id}', missing: '${current.nope.deeper}', said: 'it is ${current.name}'}}\n",
		"object-panels/mp.yaml": "name: MP\nfilter: {types: ['*']}\n" +
			"content: {kind: extension, extension: method-panel, title: '${current.id}', data: {args: '${opt.dryRun}'}}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing", Name: "The Thing"})
	by := panelByID(detail(t, ctx, svc, "m.thing", true).Panels)
	c := by["tab:hello"].Content
	if c == nil || c.Title != "The Thing" {
		t.Fatalf("hello: %+v", c)
	}
	v, _ := c.Data.(map[string]any)
	if v["id"] != "m.thing" || v["missing"] != "" || v["said"] != "it is The Thing" {
		t.Errorf("data = %v", v)
	}
	mp := by["tab:mp"].Content
	if mp == nil || mp.Title != "m.thing" {
		t.Fatalf("mp: %+v — the Def's own fields still resolve", mp)
	}
	if v, _ := mp.Data.(map[string]any); v["args"] != "${opt.dryRun}" {
		t.Errorf("an extension's input must pass verbatim, got %v", mp.Data)
	}
}

// A panel's name carrying `${...}` is evaluated over the object like its
// content (: a source's take tab named by its connector)
// an empty answer falls back to the panel's id; a plain name stays as written.
func TestAPanelNameIsEvaluatedOverTheObject(t *testing.T) {
	const rest = "filter: {types: ['*']}\ncontent: {kind: markdown, data: x}\n"
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":  "name: thing\ntree: node\n",
		"object-panels/named.yaml": "name: '${current.name}'\n" + rest,
		"object-panels/mixed.yaml": "name: 'Of ${current.name}'\n" + rest,
		"object-panels/gone.yaml":  "name: '${current.nope.deeper}'\n" + rest,
		"object-panels/plain.yaml": "name: Plain\n" + rest,
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing", Name: "The Thing"})
	by := panelByID(detail(t, ctx, svc, "m.thing", true).Panels)
	for id, want := range map[string]string{"tab:named": "The Thing", "tab:mixed": "Of The Thing", "tab:gone": "gone", "tab:plain": "Plain"} {
		if got := by[id].Name; got != want {
			t.Errorf("%s is named %q, want %q", id, got, want)
		}
	}
}

// Declared together, the source answers the data and the content is the
// display it fills.
func TestAPanelWithSourceAndContentCarriesBoth(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\n",
		"object-panels/ctx.yaml": "name: Agent context\nfilter: {types: ['*']}\nsource: 'ktw-agents:object-context/agent-context'\n" +
			"content: {kind: page, title: 'On ${id}', dslTree: {kind: table, dataField: '$.system'}}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	p := panelByID(detail(t, ctx, svc, "m.thing", true).Panels)["tab:ctx"]
	if p.Kind != api.PanelKindSource || p.Source == "" || p.Content == nil || p.Content.Title != "On m.thing" {
		t.Fatalf("panel = %+v", p)
	}
}

// The core is no panel; the type's own section carries the body as the type
// declares it, or Text in markdown when it says nothing.
func TestTheBodyIsShownAsItsTypeSays(t *testing.T) {
	section := func(d api.DetailOutput) *api.ObjectPanel {
		for _, p := range d.Panels {
			if p.ID == "section:fields" {
				return &p
			}
		}
		return nil
	}
	ctx, cwm, svc := paneWorld(t, map[string]string{"object-types/note.yaml": "name: note\ntree: node\n"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.empty", Type: "note"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.prose", Type: "note", Body: "Some prose."})
	if p := section(detail(t, ctx, svc, "m.empty", true)); p != nil {
		t.Errorf("a note with no body and no fields has a type section: %+v", p)
	}
	p := section(detail(t, ctx, svc, "m.prose", true))
	if p == nil || p.Body == nil || p.Body.Label != "Text" || p.Body.Format != cwmapi.BodyMarkdown || !p.Section {
		t.Fatalf("an undeclared body: %+v", p)
	}

	ctx, cwm, svc = paneWorld(t, map[string]string{"object-types/note.yaml": "name: note\ntree: node\nbody: {label: Reasoning, format: text}\n"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.empty", Type: "note"})
	p = section(detail(t, ctx, svc, "m.empty", true))
	if p == nil || p.Body == nil || p.Body.Label != "Reasoning" || p.Body.Format != cwmapi.BodyText {
		t.Fatalf("a declared body: %+v — the type's label and format, even while empty", p)
	}
}

// Aspects come resolved (an undeclared one reported, never dropped, and an
// error of the object); links are named from each side, the target resolved.
func TestDetailCarriesAspectsAndLinks(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/checkable.yaml": "name: checkable\n",
		"aspects/isa.yaml":            "name: isa\nlabel: ISA\ncapabilities: [checkable]\n",
		"object-types/thing.yaml":     "name: thing\ntree: node\n",
		"edge-types/depends-on.yaml":  "name: depends-on\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\ninverse_name: needed-by\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.target", Type: "thing", Name: "Target"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.src", Type: "thing", Aspects: []string{"isa", "ghost"},
		Edges: []cwmapi.ObjectEdge{{Kind: "depends-on", To: "m.target"}}})

	d := detail(t, ctx, svc, "m.src", true)
	by := map[string]api.AspectRef{}
	for _, a := range d.Aspects {
		by[a.Name] = a
	}
	if len(d.Aspects) != 2 || !by["isa"].Declared || len(by["isa"].Capabilities) == 0 || by["ghost"].Declared {
		t.Errorf("aspects: %+v", d.Aspects)
	}
	ghost := false
	for _, e := range d.Diagnostics {
		if e.Code == cwmapi.DiagnosticInvalid && strings.Contains(e.Label, `aspect "ghost"`) {
			ghost = true
		}
	}
	if !ghost {
		t.Errorf("an undeclared aspect must be an error of the object: %+v", d.Diagnostics)
	}
	if !strings.Contains(strings.Join(d.Capabilities, ","), "checkable") {
		t.Errorf("the aspect's capability is not effective: %v", d.Capabilities)
	}
	if len(d.Links) != 1 || d.Links[0].Direction != "out" || d.Links[0].TargetID != "m.target" || !d.Links[0].Resolved || d.Links[0].TargetTitle != "Target" {
		t.Fatalf("links: %+v", d.Links)
	}
	back := detail(t, ctx, svc, "m.target", true).Links
	if len(back) != 1 || back[0].Direction != "in" || back[0].TargetID != "m.src" || back[0].Label != "needed-by" {
		t.Errorf("the target's view: %+v — incoming, named by the inverse", back)
	}
}

// A link to an archived object is ENDED: unresolved (it is no object of the
// project any more) and marked so the GUI shows it as ended, not broken; a
// link to a missing id is neither.
func TestALinkToAnArchivedObjectIsEnded(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":    "name: thing\ntree: node\n",
		"edge-types/depends-on.yaml": "name: depends-on\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.done", Type: "thing", Name: "Done"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.live", Type: "thing", Name: "Live"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.src", Type: "thing",
		Edges: []cwmapi.ObjectEdge{{Kind: "depends-on", To: "m.done"}, {Kind: "depends-on", To: "m.live"}}})
	if _, err := cwm.ObjectArchive(ctx, cwmapi.ObjectArchiveInput{ID: "m.done"}); err != nil {
		t.Fatal(err)
	}
	by := map[string]api.ObjectLink{}
	for _, l := range detail(t, ctx, svc, "m.src", false).Links {
		by[l.TargetID] = l
	}
	if l := by["m.done"]; l.Resolved || !l.Ended {
		t.Errorf("the link to the archived object: %+v — want unresolved and ended", l)
	}
	if l := by["m.live"]; !l.Resolved || l.Ended {
		t.Errorf("the link to a live object: %+v — want resolved, not ended", l)
	}
}

// A missing type is an error on the pane, never a failure to open it.
func TestDetailReportsAnUndeclaredType(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, nil)
	create(t, ctx, cwm, cwmapi.Object{ID: "m.orphan", Type: "no-such-type"})
	d := detail(t, ctx, svc, "m.orphan", true)
	found := false
	for _, e := range d.Diagnostics {
		if e.Code == cwmapi.DiagnosticInvalid && strings.Contains(e.Label, `"no-such-type"`) {
			found = true
		}
	}
	if d.Object.ID != "m.orphan" || !found {
		t.Errorf("detail: %+v / %+v", d.Object, d.Diagnostics)
	}
}

// The body and the fields are a file read, taken on request.
func TestTheBodyIsReadOnlyWhenAsked(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{"object-types/record.yaml": "name: record\n"})
	o := cwmapi.Object{ID: "m.r1", Type: "record"}
	o.SetFieldsJSON(json.RawMessage(`{"amount":12}`))
	create(t, ctx, cwm, o)
	if lean := detail(t, ctx, svc, "m.r1", false).Object; len(lean.FieldsJSON()) > 0 {
		t.Errorf("the resident record carries fields: %s", lean.FieldsJSON())
	}
	full := detail(t, ctx, svc, "m.r1", true).Object
	if !strings.Contains(string(full.FieldsJSON()), `"amount":12`) {
		t.Errorf("Body: true must read the fields: %s", full.FieldsJSON())
	}
}

// One children tab per child type not drawn in the tree; a drawn child type
// is in the tree only; a panel whose `when` reads `links` shows only on an
// object with links.
func TestChildrenTabsAndALinksGatedPanel(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":    "name: thing\ntree: node\n",
		"object-types/record.yaml":   "name: record\nlabel: Records\n",
		"edge-types/depends-on.yaml": "name: depends-on\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n",
		"object-panels/links.yaml":   "name: Links\norder: 850\nfilter: {types: ['*'], when: 'len(links) > 0'}\ncontent: {kind: extension, extension: object-links}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.a", Type: "thing"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.b", Type: "thing", Parent: "m.a", Edges: []cwmapi.ObjectEdge{{Kind: "depends-on", To: "m.a"}}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.r", Type: "record", Parent: "m.a"})

	a := detail(t, ctx, svc, "m.a", true)
	by := panelByID(a.Panels)
	if p := by["tab:children/record"]; p.Kind != api.PanelKindChildren || p.ChildType != "record" || p.Name != "Records" {
		t.Errorf("records tab: %+v (strip %v)", p, ids(a.Panels))
	}
	if _, ok := by["tab:children/thing"]; ok {
		t.Errorf("a drawn child type is also a tab: %v", ids(a.Panels))
	}
	drawn := map[string]bool{}
	for _, g := range a.Children {
		drawn[g.Type] = g.Drawn
	}
	if !drawn["thing"] || drawn["record"] || len(a.Children) != 2 {
		t.Errorf("children: %+v", a.Children)
	}
	if _, ok := by["tab:links"]; !ok {
		t.Errorf("m.a has an incoming link: the links panel must show (%v)", ids(a.Panels))
	}
	if _, ok := panelByID(detail(t, ctx, svc, "m.r", true).Panels)["tab:links"]; ok {
		t.Error("m.r has no link: the links panel must not show")
	}
}

// The header's badges: the object's own, then each declared one whose field
// has a value — a capability's own field and a type's path.
func TestBadgesAreResolvedFromTheObject(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/lifecycle.yaml": "name: lifecycle\nbadges: [{field: state, icon: progress, tooltip: where it stands, colors: {proposed: yellow}}]\n",
		"capabilities/review.yaml":    "name: review\nbadges: [{field: verdict}]\n",
		// The type's own badge is its `pane:` section's, read by the pane.
		"object-types/note.yaml": "name: note\ntree: node\ncapabilities: [lifecycle, review]\npane:\n  badges: [{field: fields.owner, label: owner}]\n",
	})
	o := cwmapi.Object{ID: "m.n", Type: "note", Fields: map[string]any{"owner": "ana"},
		Badges: []cwmapi.Badge{{ID: "pinned", Label: "pinned", Color: "grape"}}}
	o.SetLifecycle(cwmapi.Lifecycle{State: "proposed"})
	create(t, ctx, cwm, o)
	got := map[string]cwmapi.Badge{}
	var order []string
	for _, b := range detail(t, ctx, svc, "m.n", false).Badges {
		got[b.ID] = b
		order = append(order, b.ID)
	}
	if len(order) != 3 || order[0] != "pinned" {
		t.Fatalf("badges %v — the object's own first, then the two with a value", order)
	}
	if b := got["lifecycle.state"]; b.Label != "proposed" || b.Color != "yellow" || b.Icon != "progress" || b.Capability != "lifecycle" || b.Tooltip == "" {
		t.Errorf("the state badge: %+v", b)
	}
	if b := got["fields.owner"]; b.Label != "owner: ana" {
		t.Errorf("the type's path badge: %+v", b)
	}
}

// A panel's filter may call a capability's function: shown while it holds.
func TestAPanelFilterCallsAFunction(t *testing.T) {
	acts := &actions{calls: map[string]int{}}
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/scores.yaml": "name: scores\nfunctions:\n  - {name: score, action: 'x:score/get', returns: number}\n",
		"object-types/thing.yaml":  "name: thing\ntree: node\ncapabilities: [scores]\n",
		"object-panels/high.yaml":  "name: High\nfilter: {types: [thing], when: 'score() > 2'}\n" + ext,
		"object-panels/low.yaml":   "name: Low\nfilter: {types: [thing], when: 'score() > 5'}\n" + ext,
	}, application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }))
	create(t, ctx, cwm, cwmapi.Object{ID: "m.t", Type: "thing"})
	got := strings.Join(ids(detail(t, ctx, svc, "m.t", true).Panels), ",")
	if got != "tab:high" {
		t.Errorf("strip %s, want only the panel whose function-guard holds (score() is 3)", got)
	}
	if n := acts.count("x:score/get"); n != 1 {
		t.Errorf("score() asked %d times for one pane, want once", n)
	}
}

// Opening a pane is a preview: a panel filter calling a costly function does
// not run it — CWM answers it decided at the move, an error that hides the
// panel — while a cheap function in the same pane runs.
func TestAPaneNeverRunsACostlyFunction(t *testing.T) {
	acts := &actions{calls: map[string]int{}}
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/scores.yaml": "name: scores\nfunctions:\n  - {name: score, action: 'x:score/get', returns: number}\n" +
			"  - {name: heavy, action: 'x:heavy/get', returns: boolean, costly: true}\n",
		"object-types/thing.yaml":  "name: thing\ntree: node\ncapabilities: [scores]\n",
		"object-panels/high.yaml":  "name: High\nfilter: {types: [thing], when: 'score() > 2'}\n" + ext,
		"object-panels/audit.yaml": "name: Audit\nfilter: {types: [thing], when: 'heavy()'}\n" + ext,
	}, application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }))
	create(t, ctx, cwm, cwmapi.Object{ID: "m.t", Type: "thing"})
	if got := strings.Join(ids(detail(t, ctx, svc, "m.t", true).Panels), ","); got != "tab:high" {
		t.Errorf("strip %s, want only High: Audit's filter is undecided in a pane", got)
	}
	if n := acts.count("x:heavy/get"); n != 0 {
		t.Errorf("opening the pane ran the costly heavy() %d times", n)
	}
	if n := acts.count("x:score/get"); n != 1 {
		t.Errorf("the cheap score() ran %d times, want once", n)
	}
	// Outside a preview the same function runs.
	res, err := cwm.ObjectEvaluate(ctx, cwmapi.ObjectEvaluateInput{ID: "m.t", Expressions: []string{"heavy()"}})
	if err != nil || res.Results[0].Value != true || acts.count("x:heavy/get") != 1 {
		t.Errorf("an evaluation that is not a preview runs heavy(): %+v, %v, %d calls", res, err, acts.count("x:heavy/get"))
	}
}

// The type's own fields are a section of Information, named by the type; a
// capability's fields no panel covers are a section too; the capabilities
// come with label, icon and tooltip.
func TestTheTypesFieldsAndCapabilitiesAreSections(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/sizing.yaml":   "name: sizing\nlabel: Sizing\ndescription: How big it is\nicon: ruler\nschema: x:/data-type/cap-sizing\n",
		"object-types/estimate.yaml": "name: estimate\nlabel: Estimate\ntree: node\nfields: x:/data-type/estimate\ncapabilities: [sizing]\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.e", Type: "estimate", Body: "Why three.", Fields: map[string]any{"unit": "days"}})
	d := detail(t, ctx, svc, "m.e", true)
	by := panelByID(d.Panels)
	if p := by["section:fields"]; p.Schema != "x:/data-type/estimate" || p.Name != "Estimate" || !p.Section || p.Body == nil {
		t.Errorf("the type's section: %+v", p)
	}
	if p, ok := by["section:cap/sizing"]; !ok || !p.Section {
		t.Errorf("the sizing section: %+v", p)
	}
	var sizing api.CapabilityInfo
	for _, c := range d.CapabilityInfo {
		if c.ID == "sizing" {
			sizing = c
		}
	}
	if sizing.Label != "Sizing" || sizing.Icon != "ruler" || sizing.Description != "How big it is" || sizing.Source != "type" {
		t.Errorf("capability info: %+v", d.CapabilityInfo)
	}
}

// The policies in force are the ones CWM puts on the object by type: named,
// in full, never in the capability set.
func TestThePoliciesInForceAreListed(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":   "name: thing\ntree: node\n",
		"object-types/note.yaml":    "name: note\ntree: node\n",
		"policies/everywhere.yaml":  "name: everywhere\nlabel: Everywhere\napplies: {types: ['*']}\n",
		"policies/things-only.yaml": "name: things-only\napplies: {types: [thing]}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.t", Type: "thing"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.n", Type: "note"})
	names := func(id string) string {
		d := detail(t, ctx, svc, id, false)
		var out []string
		for _, p := range d.Policies {
			out = append(out, p.Name)
		}
		for _, c := range d.Capabilities {
			if c == "everywhere" || c == "things-only" {
				t.Errorf("%s: a policy is in the capability set: %v", id, d.Capabilities)
			}
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	if got := names("m.t"); got != "everywhere,things-only" {
		t.Errorf("thing: %s", got)
	}
	if got := names("m.n"); got != "everywhere" {
		t.Errorf("note: %s", got)
	}
	if d := detail(t, ctx, svc, "m.n", false); len(d.Policies) != 1 || d.Policies[0].Label != "Everywhere" {
		t.Errorf("the policy comes in full: %+v", d.Policies)
	}
}

// A panel says which object changes re-read it while shown: a source panel
// follows its module unless it declares otherwise, static content follows
// nothing — and the declared scope and types reach the host as written.
func TestAPanelCarriesItsRefreshScope(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":    "name: thing\ntree: node\n",
		"object-panels/sourced.yaml": "name: Sourced\nfilter: {types: [thing]}\nsource: 'ktw-agents:x/y'\n",
		"object-panels/static.yaml":  "name: Static\nfilter: {types: [thing]}\n" + ext,
		"object-panels/kids.yaml":    "name: Kids\nfilter: {types: [thing]}\nsource: 'ktw-agents:x/z'\nrefresh: {scope: children, types: [use-case]}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	ps := panelByID(detail(t, ctx, svc, "m.thing", false).Panels)
	for id, want := range map[string]api.PanelRefresh{
		"tab:sourced": {Scope: api.RefreshModule},
		"tab:static":  {Scope: api.RefreshNone},
		"tab:kids":    {Scope: api.RefreshChildren, Types: []string{"use-case"}},
	} {
		got := ps[id].Refresh
		if got == nil || got.Scope != want.Scope || strings.Join(got.Types, ",") != strings.Join(want.Types, ",") {
			t.Errorf("%s: refresh %+v, want %+v", id, got, want)
		}
	}
}

// A template only the page's host can fill — the row an action is on — is
// kept as written; the object's own templates are resolved beside it.
func TestAPanelKeepsTheHostsTemplates(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\n",
		"object-panels/rows.yaml": "name: Rows\nfilter: {types: [thing]}\nsource: 'ktw-agents:x/y'\n" +
			"content: {kind: page, dslTree: {kind: table, dataField: '$.rows', open: {navigate: {object: '${item.id}'}}, " +
			"actions: [{label: 'On ${id}', uri: 'ktw-agents:x/z', args: {row: '${item.id}', at: '${id}', who: '${person}'}}]}}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	p := panelByID(detail(t, ctx, svc, "m.thing", false).Panels)["tab:rows"]
	b, _ := json.Marshal(p.Content)
	got := string(b)
	for _, want := range []string{`"object":"${item.id}"`, `"row":"${item.id}"`, `"who":"${person}"`, `"at":"m.thing"`, `"label":"On m.thing"`} {
		if !strings.Contains(got, want) {
			t.Errorf("content lacks %s: %s", want, got)
		}
	}
}

// tabWorld is a strip with Information, three panels, a fields section and a
// children tab, on objects carrying the aspects given (name → YAML).
func tabWorld(t *testing.T, aspects map[string]string) (context.Context, cwmapi.CWMService, api.ObjectPaneService) {
	t.Helper()
	all := "filter: {types: ['*']}\n" + ext
	files := map[string]string{
		"capabilities/content.yaml":      "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"object-types/thing.yaml":        "name: thing\ntree: node\ncapabilities: [content]\n",
		"object-types/record.yaml":       "name: record\nlabel: Records\n",
		"object-panels/information.yaml": "name: Information\norder: 1\n" + all,
		"object-panels/alpha.yaml":       "name: Alpha\norder: 10\n" + all,
		"object-panels/beta.yaml":        "name: Beta\norder: 20\n" + all,
		"object-panels/gamma.yaml":       "name: Gamma\norder: 30\n" + all,
	}
	for name, body := range aspects {
		files["aspects/"+name+".yaml"] = body
	}
	return paneWorld(t, files)
}

func stripOf(ps []api.ObjectPanel) string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		s := p.ID
		if p.More {
			s += "+"
		}
		out = append(out, s)
	}
	return strings.Join(out, ",")
}

// An aspect's `pane.tabs` lists the object's main tabs: they come first, in
// list order (a panel id or a full strip id); a name the strip does not hold
// is skipped; every other tab keeps its order and is More; Information stays
// first and main; a section of the Information tab is never More
func TestAnAspectsTabListPutsItsTabsFirstAndTheRestBehindMore(t *testing.T) {
	ctx, cwm, svc := tabWorld(t, map[string]string{
		"study": "name: study\ncapabilities: []\npane:\n  tabs: [gamma, ghost, 'tab:children/record', alpha]\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.a", Type: "thing", Aspects: []string{"study"}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.r", Type: "record", Parent: "m.a"})
	got := stripOf(detail(t, ctx, svc, "m.a", true).Panels)
	if want := "tab:information,tab:gamma,tab:children/record,tab:alpha,tab:beta+,section:cap/content"; got != want {
		t.Errorf("strip %s, want %s", got, want)
	}
}

// With no list — no aspect, or an aspect writing no pane — the strip is as
// its orders say and nothing is More.
func TestNoTabListLeavesTheStripUnchanged(t *testing.T) {
	ctx, cwm, svc := tabWorld(t, map[string]string{"plain": "name: plain\ncapabilities: []\n"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.a", Type: "thing", Aspects: []string{"plain"}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.b", Type: "thing"})
	want := "tab:information,tab:alpha,tab:beta,tab:gamma,section:cap/content"
	for _, id := range []string{"m.a", "m.b"} {
		if got := stripOf(detail(t, ctx, svc, id, true).Panels); got != want {
			t.Errorf("%s: strip %s, want %s", id, got, want)
		}
	}
}

// Two aspects' lists are joined in aspect order, each name once; Information
// is main though no list names it.
func TestTwoAspectsTabListsAreJoinedWithoutDuplicates(t *testing.T) {
	ctx, cwm, svc := tabWorld(t, map[string]string{
		"one": "name: one\ncapabilities: []\npane: {tabs: [beta, alpha]}\n",
		"two": "name: two\ncapabilities: []\npane: {tabs: [alpha, 'tab:beta', beta]}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.a", Type: "thing", Aspects: []string{"one", "two"}})
	got := stripOf(detail(t, ctx, svc, "m.a", true).Panels)
	if want := "tab:information,tab:beta,tab:alpha,tab:gamma+,section:cap/content"; got != want {
		t.Errorf("strip %s, want %s", got, want)
	}
}

// A type whose `pane.only` makes its listed tabs the whole pane shows them
// alone, in list order - no Information, no section, no other panel, no
// children tab, nothing behind More, whatever its aspects list - and says the
// pane draws no object header; without `only` its list
// keeps the others behind More.
func TestATypesOnlyTabsAreTheWholePane(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/content.yaml":      "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"object-types/slim.yaml":         "name: slim\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma, ghost, alpha]\n  only: true\n",
		"object-types/listed.yaml":       "name: listed\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma, alpha]\n",
		"object-types/record.yaml":       "name: record\nlabel: Records\n",
		"aspects/extra.yaml":             "name: extra\ncapabilities: []\npane: {tabs: [beta]}\n",
		"object-panels/information.yaml": "name: Information\norder: 1\nfilter: {types: ['*']}\n" + ext,
		"object-panels/alpha.yaml":       "name: Alpha\norder: 10\nfilter: {types: ['*']}\n" + ext,
		"object-panels/beta.yaml":        "name: Beta\norder: 20\nfilter: {types: ['*']}\n" + ext,
		"object-panels/gamma.yaml":       "name: Gamma\norder: 30\nfilter: {types: ['*']}\n" + ext,
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.s", Type: "slim", Aspects: []string{"extra"}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.r", Type: "record", Parent: "m.s"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.l", Type: "listed"})
	d := detail(t, ctx, svc, "m.s", true)
	if got, want := stripOf(d.Panels), "tab:gamma,tab:alpha"; got != want || !d.Bare {
		t.Errorf("only: strip %s bare %v, want %s and bare", got, d.Bare, want)
	}
	l := detail(t, ctx, svc, "m.l", true)
	if got, want := stripOf(l.Panels), "tab:information,tab:gamma,tab:alpha,tab:beta+,section:cap/content"; got != want || l.Bare {
		t.Errorf("without only: strip %s bare %v, want %s and a header", got, l.Bare, want)
	}
}

// Whether a pane draws the standard object header is each type's own option
// (`pane.header`): `header: true` keeps it above a type's
// only tabs, `header: false` leaves it out of a type with the full strip, and
// unset keeps the default - none under `only`, one otherwise.
func TestATypesHeaderIsItsOwnOption(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/content.yaml":      "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"object-types/headed.yaml":       "name: headed\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma, alpha]\n  only: true\n  header: true\n",
		"object-types/slim.yaml":         "name: slim\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma, alpha]\n  only: true\n",
		"object-types/plain.yaml":        "name: plain\ntree: node\ncapabilities: [content]\npane:\n  header: false\n",
		"object-types/listed.yaml":       "name: listed\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma]\n",
		"object-panels/information.yaml": "name: Information\norder: 1\nfilter: {types: ['*']}\n" + ext,
		"object-panels/alpha.yaml":       "name: Alpha\norder: 10\nfilter: {types: ['*']}\n" + ext,
		"object-panels/gamma.yaml":       "name: Gamma\norder: 30\nfilter: {types: ['*']}\n" + ext,
	})
	for _, o := range []cwmapi.Object{{ID: "m.h", Type: "headed"}, {ID: "m.s", Type: "slim"}, {ID: "m.p", Type: "plain"}, {ID: "m.l", Type: "listed"}} {
		create(t, ctx, cwm, o)
	}
	for _, c := range []struct {
		id    string
		bare  bool
		strip string
	}{
		{"m.h", false, "tab:gamma,tab:alpha"},
		{"m.s", true, "tab:gamma,tab:alpha"},
		{"m.p", true, ""},
		{"m.l", false, ""},
	} {
		d := detail(t, ctx, svc, c.id, true)
		if d.Bare != c.bare {
			t.Errorf("%s: bare %v, want %v", c.id, d.Bare, c.bare)
		}
		if got := stripOf(d.Panels); c.strip != "" && got != c.strip {
			t.Errorf("%s: strip %s, want %s - the header option changes no tab", c.id, got, c.strip)
		}
	}
}

// A type's only tabs follow its variants too (: IN-062's
// switch opens an increment on other tabs): the first variant whose when
// holds gives the whole pane; none holding - or a when that fails - the plain
// tabs.
func TestATypesOnlyTabsFollowTheFirstVariantThatHolds(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/content.yaml": "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"object-types/slim.yaml": "name: slim\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma, alpha]\n  only: true\n  variants:\n" +
			"    - {when: \"current.name == 'broken' && nothing.here\", tabs: [beta]}\n" +
			"    - {when: \"current.name == 'new'\", tabs: [beta, information]}\n",
		"object-panels/information.yaml": "name: Information\norder: 1\nfilter: {types: ['*']}\n" + ext,
		"object-panels/alpha.yaml":       "name: Alpha\norder: 10\nfilter: {types: ['*']}\n" + ext,
		"object-panels/beta.yaml":        "name: Beta\norder: 20\nfilter: {types: ['*']}\n" + ext,
		"object-panels/gamma.yaml":       "name: Gamma\norder: 30\nfilter: {types: ['*']}\n" + ext,
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.new", Name: "new", Type: "slim"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.broken", Name: "broken", Type: "slim"})
	for id, want := range map[string]string{"m.new": "tab:beta,tab:information", "m.broken": "tab:gamma,tab:alpha"} {
		if d := detail(t, ctx, svc, id, true); stripOf(d.Panels) != want || !d.Bare {
			t.Errorf("%s: strip %s bare %v, want %s and bare", id, stripOf(d.Panels), d.Bare, want)
		}
	}
}

// ktw-code's Dashboard shows on a module whose code capability lists packages,
// and not on one that lists none — its shipped `when`, read from the pack file
// (answer).
func TestTheDashboardShowsOnlyOnAModuleWithCode(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "app", "ktw-code", "embed", "cwm", "object-panels", "module-dashboard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/module-health.yaml":     "name: module-health\n",
		"capabilities/code.yaml":              "name: code\n",
		"object-types/area.yaml":              "name: area\ntree: node\ncapabilities: [module-health]\n",
		"object-panels/module-dashboard.yaml": string(raw),
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.code", Type: "area", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{"internal/x"}}}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.empty", Type: "area", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{}}}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.docs", Type: "area"})
	for id, want := range map[string]bool{"m.code": true, "m.empty": false, "m.docs": false} {
		if _, got := panelByID(detail(t, ctx, svc, id, true).Panels)["tab:module-dashboard"]; got != want {
			t.Errorf("%s: Dashboard shown %v, want %v", id, got, want)
		}
	}
}

// An action a type or a capability declares quick shows on the object's first
// page as a small button - beside the defaults - and stays in the action
// panel; one not declared quick is not among them.
func TestQuickActions(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/planned.yaml": "name: planned\nactions:\n  - uri: ktw-claude:sessions/run-request\n    label: Run in Claude Code\n    quick: true\n  - uri: ktw-x:tasks/archive\n    label: Archive\n",
		"object-types/thing.yaml":   "name: thing\ntree: node\ncapabilities: [planned]\nactions:\n  - uri: ktw-x:tasks/review\n    label: Review\n    kind: ai\n    quick: true\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	got := detail(t, ctx, svc, "m.thing", false).QuickActions
	var labels []string
	for _, a := range got {
		labels = append(labels, a.Label)
		if a.Label != "Start chat" && !a.Quick {
			t.Errorf("a declared quick action is not marked quick: %+v", a)
		}
	}
	if strings.Join(labels, ",") != "Start chat,Review,Run in Claude Code" {
		t.Fatalf("the quick actions: %v; want the default, then the type's, then the capability's quick ones - not Archive", labels)
	}
	if got[1].Kind != "ai" {
		t.Errorf("the type's quick action lost its kind: %+v", got[1])
	}
}

// : a pane's variants name its main tabs by condition - the
// first whose when holds on the object gives them, the others behind More;
// none holding (or a broken when), the plain tabs.
func TestAPaneTabsFollowTheFirstVariantThatHolds(t *testing.T) {
	ctx, cwm, svc := tabWorld(t, map[string]string{
		"study": "name: study\ncapabilities: []\npane:\n  tabs: [alpha, beta]\n  variants:\n" +
			"    - {when: \"current.name == 'broken' && nothing.here\", tabs: [beta]}\n" +
			"    - {when: \"current.name == 'new'\", tabs: [gamma]}\n" +
			"    - {when: \"current.name == 'other'\", tabs: [beta, gamma]}\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.new", Name: "new", Type: "thing", Aspects: []string{"study"}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.old", Type: "thing", Aspects: []string{"study"}})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.other", Name: "other", Type: "thing", Aspects: []string{"study"}})
	for id, want := range map[string]string{
		"m.new":   "tab:information,tab:gamma,tab:alpha+,tab:beta+,section:cap/content",
		"m.other": "tab:information,tab:beta,tab:gamma,tab:alpha+,section:cap/content",
		"m.old":   "tab:information,tab:alpha,tab:beta,tab:gamma+,section:cap/content",
	} {
		if got := stripOf(detail(t, ctx, svc, id, true).Panels); got != want {
			t.Errorf("%s: strip %s, want %s", id, got, want)
		}
	}
}

// : a type naming its « More… » tabs (`pane.more`) makes
// its strip exact - its listed tabs main, in order, the More tabs exactly
// those it names, in its order, Information one of them when it names it,
// every other tab (a panel, a children tab) hidden; a section stays. A
// variant that holds gives its own More list, not the plain one; no More
// list, every other tab behind More as before. An aspect's More list joins
// the type's, a name a main list holds staying main.
func TestATypesMoreListIsExactlyItsMoreTabs(t *testing.T) {
	all := "filter: {types: ['*']}\n" + ext
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/content.yaml": "name: content\nlabel: Content\nschema: sys:/data-type/content\n",
		"object-types/exact.yaml": "name: exact\ntree: node\ncapabilities: [content]\npane:\n  tabs: [gamma]\n  more: [information, ghost, alpha]\n  variants:\n" +
			"    - {when: \"current.name == 'v'\", tabs: [beta, 'tab:children/record'], more: [gamma]}\n" +
			"    - {when: \"current.name == 'w'\", tabs: [beta]}\n",
		"object-types/record.yaml":       "name: record\nlabel: Records\n",
		"aspects/extra.yaml":             "name: extra\ncapabilities: []\npane: {tabs: [delta], more: [beta, gamma]}\n",
		"object-panels/information.yaml": "name: Information\norder: 1\n" + all,
		"object-panels/alpha.yaml":       "name: Alpha\norder: 10\n" + all,
		"object-panels/beta.yaml":        "name: Beta\norder: 20\n" + all,
		"object-panels/gamma.yaml":       "name: Gamma\norder: 30\n" + all,
		"object-panels/delta.yaml":       "name: Delta\norder: 40\n" + all,
	})
	for _, o := range []cwmapi.Object{
		{ID: "m.plain", Type: "exact"},
		{ID: "m.v", Name: "v", Type: "exact"},
		{ID: "m.w", Name: "w", Type: "exact"},
		{ID: "m.asp", Type: "exact", Aspects: []string{"extra"}},
	} {
		create(t, ctx, cwm, o)
		create(t, ctx, cwm, cwmapi.Object{ID: o.ID + "r", Type: "record", Parent: o.ID})
	}
	for id, want := range map[string]string{
		"m.plain": "tab:gamma,tab:information+,tab:alpha+,section:cap/content",
		"m.v":     "tab:information,tab:beta,tab:children/record,tab:gamma+,section:cap/content",
		"m.w":     "tab:information,tab:beta,tab:alpha+,tab:gamma+,tab:delta+,section:cap/content,tab:children/record+",
		"m.asp":   "tab:gamma,tab:delta,tab:information+,tab:alpha+,tab:beta+,section:cap/content",
	} {
		if got := stripOf(detail(t, ctx, svc, id, true).Panels); got != want {
			t.Errorf("%s: strip %s, want %s", id, got, want)
		}
	}
}
