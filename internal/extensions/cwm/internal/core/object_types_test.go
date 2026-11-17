// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE DECLARED TYPES as the forms and pickers read them: ObjectAllowedChildren,
// ObjectTypeGet / List, ObjectAspectList — and `current`, the object every
// expression and display sees.

func choiceNames(cs []api.TypeChoice) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

func objectSidePicker(t *testing.T, svc *Service, objectID string) []string {
	t.Helper()
	out, err := svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{ParentID: objectID})
	if err != nil {
		t.Fatalf("ObjectAllowedChildren(%s): %v", objectID, err)
	}
	return choiceNames(out.Choices)
}

func hasName(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	out := in[:0:0]
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ── the picker: two verbs → one ────────────────────────────────────────────

// TestObjectAllowedChildrenUnderAnUncuratedParentFollowsTheWritePath: under
// an object that curates nothing (a `note`, below the levels), the picker
// offers the types naming it - never one accepting any parent or saying
// nothing - and every type it offers is one ObjectCreate accepts
// under that parent.
func TestObjectAllowedChildrenUnderAnUncuratedParentFollowsTheWritePath(t *testing.T) {
	f := detailFixture(t)
	f.svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{
		{Name: "named-under-note", AcceptParents: []string{"note"}},
		{Name: "any-parent", AcceptParents: []string{"*"}},
		{Name: "silent"},
	}})
	noteID := f.byKey["N"] // a `note`: no allowed_children, no allowed_item_types
	offered := objectSidePicker(t, f.svc, qualify(noteID))
	if !hasName(offered, "named-under-note") || hasName(offered, "any-parent") || hasName(offered, "silent") {
		t.Fatalf("an uncurated object offers %v; want the types naming it only", offered)
	}
	for _, typeName := range offered {
		o := api.Object{ID: "probe-" + typeName, Type: typeName}
		o.Name = "probe"
		o.Parent = qualify(noteID)
		if _, err := f.svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: fixtureModule, Object: o}); err != nil {
			t.Errorf("the picker offered %q under a note but ObjectCreate refused it: %v", typeName, err)
		}
	}
}

// TestObjectAllowedChildrenNeverOffersAnAbstractType — an abstract type cannot
// be instantiated, so offering it is offering a dead button.
func TestObjectAllowedChildrenNeverOffersAnAbstractType(t *testing.T) {
	f := detailFixture(t)
	f.svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{
		{Name: "abstract-thing", Abstract: true, AcceptParents: []string{"note"}},
		{Name: "concrete-thing", AcceptParents: []string{"note"}},
	}})
	got := objectSidePicker(t, f.svc, qualify(f.byKey["N"]))
	if hasName(got, "abstract-thing") {
		t.Errorf("picker offered an abstract type: %v", got)
	}
	if !hasName(got, "concrete-thing") {
		t.Errorf("picker dropped the concrete control type: %v", got)
	}
}

// ── the type lookup: four verbs → two ──────────────────────────────────────

// ── the detail bundle ──────────────────────────────────────────────────────

// TestObjectAspectListIsThePicker — the "+ Aspect" chip row reads the declared
// aspects, sorted.
func TestObjectAspectListIsThePicker(t *testing.T) {
	f := detailFixture(t)
	f.svc.Config.CatalogTypes.Aspects = []api.Aspect{
		{Name: "zeta", Capabilities: []string{"checkable"}},
		{Name: "alpha", Capabilities: []string{"checkable"}},
	}
	out, err := f.svc.ObjectAspectList(stdctx.Background(), api.ObjectAspectListInput{})
	if err != nil {
		t.Fatalf("ObjectAspectList: %v", err)
	}
	if len(out.Aspects) != 2 || out.Aspects[0].Name != "alpha" || out.Aspects[1].Name != "zeta" {
		t.Errorf("aspect picker: %+v, want alpha then zeta", out.Aspects)
	}
}

func hasDiagnostic(ds []api.ObjectDiagnostic, code string) bool {
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}

// ── the tab strip is not the capability list ───────────────────────────────

// TestCurrentCarriesMinimalMetadata — goal cwm.g13 AC1: id, name, type,
// type_name, icon, icon_name (from the type), parent, module, file.
func TestCurrentCarriesMinimalMetadata(t *testing.T) {
	f := detailFixture(t)
	mkVerifyObject(t, f.svc, "m.cur", "note", "", nil)
	got, err := f.svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "m", ID: "m.cur"})
	if err != nil {
		t.Fatalf("ObjectGet: %v", err)
	}
	cur := f.svc.currentOf(got.Object)
	for _, k := range []string{"id", "name", "type", "type_name", "icon", "icon_name", "parent", "module", "file"} {
		if _, ok := cur[k]; !ok {
			t.Errorf("current lacks %q: %v", k, cur)
		}
	}
	if cur["id"] != "m.cur" || cur["type"] != "note" || cur["module"] != "m" {
		t.Errorf("current = %v", cur)
	}
	if f, _ := cur["file"].(string); !strings.HasSuffix(f, ".cwm.yaml") && !strings.HasSuffix(f, ".cwm.md") {
		t.Errorf("current.file = %q, want the object's file", cur["file"])
	}
}

// mkVerifyObject creates an object in module m with a structured body.
func mkVerifyObject(t *testing.T, svc *Service, id, typ, parent string, body map[string]any) {
	t.Helper()
	o := api.Object{ID: id, Type: typ}
	o.Name = id
	o.Parent = parent
	raw, _ := json.Marshal(body)
	o.Name = id
	o.SetFieldsJSON(raw)
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

// : a type's accept_when is a condition over the parent - `current`
// is the parent - under which the picker offers it: a condition the parent
// meets offers it, one it does not leaves it out, an unconditioned type is
// always offered, a broken condition leaves it out, and with no parent the
// condition is not evaluated.
func TestObjectAllowedChildrenFollowsAcceptWhen(t *testing.T) {
	f := detailFixture(t)
	f.svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{
		{Name: "under-a-note", AcceptParents: []string{"note", "*"}, AcceptWhen: "current.type == 'note'"},
		{Name: "under-a-planned", AcceptParents: []string{"note", "*"}, AcceptWhen: "type != 'note' or 'planned' in current.capabilities"},
		{Name: "anywhere", AcceptParents: []string{"note", "*"}},
		{Name: "broken-condition", AcceptParents: []string{"note", "*"}, AcceptWhen: "current.type =="},
	}})
	got := objectSidePicker(t, f.svc, qualify(f.byKey["N"]))
	if !hasName(got, "under-a-note") {
		t.Errorf("a condition the parent meets did not offer the type: %v", got)
	}
	if hasName(got, "under-a-planned") {
		t.Errorf("a condition the parent does not meet (an unplanned note) offered the type: %v", got)
	}
	if !hasName(got, "anywhere") {
		t.Errorf("an unconditioned type was dropped: %v", got)
	}
	if hasName(got, "broken-condition") {
		t.Errorf("a condition that cannot be evaluated offered the type: %v", got)
	}
	root, err := f.svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(root.Choices))
	for _, c := range root.Choices {
		names = append(names, c.Name)
	}
	if !hasName(names, "under-a-planned") {
		t.Errorf("with no parent, the condition was evaluated and dropped the type: %v", names)
	}
}

// A CHOICE SAYS WHETHER IT IS NAMED: a type is named
// only when it names the parent's type among the parents it accepts ("*"
// names none). Under an object below the levels only the named ones are
// offered, so every choice there reads named.
func TestAnAllowedChildIsNamedOnlyWhenDeclared(t *testing.T) {
	f := detailFixture(t)
	f.svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{
		{Name: "named-under-note", AcceptParents: []string{"note"}},
		{Name: "any-parent", AcceptParents: []string{"*"}},
	}})
	out, err := f.svc.ObjectAllowedChildren(stdctx.Background(), api.ObjectAllowedChildrenInput{ParentID: qualify(f.byKey["N"])})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Choices) == 0 {
		t.Fatal("under a note nothing is offered; want the type naming it")
	}
	for _, c := range out.Choices {
		ct, _ := f.svc.lookupObjectType(c.Name)
		if !c.Named || !slices.Contains(ct.AcceptParents, "note") {
			t.Errorf("%s under a note: named %v (its accept_parents %v); want only named types naming note", c.Name, c.Named, ct.AcceptParents)
		}
	}
}
