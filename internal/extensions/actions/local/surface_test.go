// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE ACTION SURFACE, on a real CWM: the type's and the effective capabilities'
// actions, the tasks that apply themselves to the type, the global registry —
// `when:` evaluated, `input:` interpolated. The task-side half is the one a
// type never declares; without it a test proves half the surface.

const runbook = `name: runbook
capabilities: [container, content]
tree: node
actions:
  - uri: p:tasks/archive
    label: Archive
    icon: box
    confirm: true
    when: "!('status' in object.fields) || object.fields.status != 'archived'"
  - uri: p:tasks/review
    label: Request review
    when: "(!('review_state' in object.fields) || object.fields.review_state == 'pending') && any(children, .type == 'todo' && .status == 'done')"
  - uri: p:tasks/approve
    label: Approve
    when: "object.fields.review_state == 'in-review'"
  - uri: p:tasks/open
    label: Open the file
    input: {path: "${object.fields.skill_file}", id: "${id}", where: "in ${id}"}
  - uri: p:tasks/bulk
    label: Bulk edit
    target: {scope: collection}
`

func tasks(context.Context) []actionsapi.TaskAction {
	return []actionsapi.TaskAction{
		{Provider: "p", Name: "refine", Label: "Refine", Kind: cwmapi.ActionScopeObject, Types: []string{"runbook"}, SessionMode: cwmapi.SessionModeChat},
		{Provider: "p", Name: "publish", Label: "Publish", Kind: cwmapi.ActionScopeObject, Types: []string{"runbook"},
			Filter: "object.fields.review_state == 'approved'"},
		{Provider: "p", Name: "elsewhere", Label: "Elsewhere", Kind: cwmapi.ActionScopeObject, Types: []string{"other-type"}},
		{Provider: "p", Name: "sweep", Label: "Sweep", Kind: cwmapi.ActionScopeCollection, Types: []string{"runbook"}},
		{Provider: "p", Name: "anywhere", Label: "Anywhere", Kind: cwmapi.ActionScopeGlobal},
	}
}

func world(t *testing.T) (context.Context, cwmapi.CWMService, actionsapi.SurfaceService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml":   {Data: []byte("name: container\n")},
			"capabilities/content.yaml":     {Data: []byte("name: content\n")},
			"capabilities/lifecycle.yaml":   {Data: []byte("name: lifecycle\n")},
			"capabilities/composition.yaml": {Data: []byte("name: composition\nactions:\n  - {uri: p:tasks/compose, label: Compose}\n")},
			"object-types/module.yaml":      {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/runbook.yaml":     {Data: []byte(runbook)},
			"object-types/todo.yaml":        {Data: []byte("name: todo\ncapabilities: [container, content, lifecycle]\n")},
		}),
		actionslocal.SurfaceExtensionWith(tasks),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	s, err := application.UseService[actionsapi.SurfaceService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, s
}

func mkRunbook(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string, fields map[string]any) {
	t.Helper()
	o := cwmapi.Object{ID: id, Type: "runbook"}
	o.Name, o.Parent = id, "m"
	raw, _ := json.Marshal(fields)
	o.SetFieldsJSON(raw)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

func list(t *testing.T, ctx context.Context, s actionsapi.SurfaceService, in actionsapi.ListInput) map[string]cwmapi.Action {
	t.Helper()
	out, err := s.List(ctx, in)
	if err != nil {
		t.Fatalf("List %+v: %v", in, err)
	}
	m := map[string]cwmapi.Action{}
	for _, a := range out.Actions {
		m[a.URI] = a
	}
	return m
}

func keys(m map[string]cwmapi.Action) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestAWhenIsEvaluatedOverTheObject(t *testing.T) {
	ctx, cwm, s := world(t)
	mkRunbook(t, ctx, cwm, "m.r1", map[string]any{"status": "archived", "review_state": "in-review"})
	mkRunbook(t, ctx, cwm, "m.r2", map[string]any{"review_state": "pending"})

	a := list(t, ctx, s, actionsapi.ListInput{ID: "m.r1"})
	if !a["p:tasks/archive"].Disabled || !strings.Contains(a["p:tasks/archive"].DisabledReason, "condition not met") {
		t.Errorf("an archived runbook must not offer Archive: %+v", a["p:tasks/archive"])
	}
	if a["p:tasks/approve"].Disabled {
		t.Errorf("an in-review runbook must offer Approve: %+v", a["p:tasks/approve"])
	}
	if !a["p:tasks/review"].Disabled {
		t.Errorf("Request review needs a done todo child: %+v", a["p:tasks/review"])
	}
	if _, present := a["p:tasks/bulk"]; present {
		t.Errorf("a collection action leaked into the object list")
	}
	b := list(t, ctx, s, actionsapi.ListInput{ID: "m.r2"})
	if b["p:tasks/archive"].Disabled || !b["p:tasks/approve"].Disabled {
		t.Errorf("a pending runbook: archive %+v approve %+v", b["p:tasks/archive"], b["p:tasks/approve"])
	}
	todo := cwmapi.Object{ID: "m.t1", Type: "todo"}
	todo.Name, todo.Parent = "t1", "m.r2"
	todo.SetLifecycle(cwmapi.Lifecycle{State: "done"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: todo}); err != nil {
		t.Fatal(err)
	}
	if list(t, ctx, s, actionsapi.ListInput{ID: "m.r2"})["p:tasks/review"].Disabled {
		t.Errorf("with a done todo child, Request review must be offered")
	}
}

// `input:` is interpolated: a whole expression keeps its value, an embedded
// one is text, and one that cannot resolve is EMPTY — never the template.
func TestAnInputIsInterpolated(t *testing.T) {
	ctx, cwm, s := world(t)
	mkRunbook(t, ctx, cwm, "m.r3", map[string]any{"skill_file": "skills/a/SKILL.md"})
	mkRunbook(t, ctx, cwm, "m.r4", map[string]any{})
	got := list(t, ctx, s, actionsapi.ListInput{ID: "m.r3"})["p:tasks/open"].BoundInput
	if got["path"] != "skills/a/SKILL.md" || got["id"] != "m.r3" || got["where"] != "in m.r3" {
		t.Errorf("interpolated input = %v", got)
	}
	if v := list(t, ctx, s, actionsapi.ListInput{ID: "m.r4"})["p:tasks/open"].BoundInput["path"]; v != "" {
		t.Errorf("an unresolvable interpolation = %q, want empty", v)
	}
}

// An instance-added capability brings its actions to that object alone.
func TestTheCapabilitiesAreTheEffectiveOnes(t *testing.T) {
	ctx, cwm, s := world(t)
	mkRunbook(t, ctx, cwm, "m.r5", map[string]any{})
	mkRunbook(t, ctx, cwm, "m.r6", map[string]any{})
	if _, has := list(t, ctx, s, actionsapi.ListInput{ID: "m.r5"})["p:tasks/compose"]; has {
		t.Fatal("composition's action is offered without the capability")
	}
	cur, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.r5"})
	o := cur.Object
	_ = o.SetCapability("composition", map[string]any{})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, has := list(t, ctx, s, actionsapi.ListInput{ID: "m.r5"})["p:tasks/compose"]; !has {
		t.Error("an instance-added composition must surface its capability's action")
	}
	if _, has := list(t, ctx, s, actionsapi.ListInput{ID: "m.r6"})["p:tasks/compose"]; has {
		t.Error("...on that object alone")
	}
}

// The task-side walk, the collection and the global registry; a bad scope or
// a missing id is refused rather than answered with an empty list.
func TestTheTasksAndTheScopes(t *testing.T) {
	ctx, cwm, s := world(t)
	mkRunbook(t, ctx, cwm, "m.r8", map[string]any{"review_state": "approved"})
	mkRunbook(t, ctx, cwm, "m.r9", map[string]any{"review_state": "pending"})

	a := list(t, ctx, s, actionsapi.ListInput{ID: "m.r8"})
	if got, ok := a["p:tasks/refine"]; !ok || got.Kind != "ai" || got.Source != "applies-to" {
		t.Errorf("a task that applies to this type: %+v", got)
	}
	if a["p:tasks/publish"].Disabled {
		t.Errorf("an approved runbook must offer Publish: %+v", a["p:tasks/publish"])
	}
	for _, k := range []string{"p:tasks/elsewhere", "p:tasks/anywhere"} {
		if _, has := a[k]; has {
			t.Errorf("%s leaked into the object list: %v", k, keys(a))
		}
	}
	if !list(t, ctx, s, actionsapi.ListInput{ID: "m.r9"})["p:tasks/publish"].Disabled {
		t.Errorf("a pending runbook must not offer Publish")
	}
	g := list(t, ctx, s, actionsapi.ListInput{Scope: cwmapi.ActionScopeGlobal})
	if _, has := g["p:tasks/anywhere"]; !has || len(g) != 1 {
		t.Errorf("the global registry: %v", keys(g))
	}
	c := list(t, ctx, s, actionsapi.ListInput{Scope: cwmapi.ActionScopeCollection, ID: "m.r8", Type: "runbook"})
	if _, has := c["p:tasks/sweep"]; !has {
		t.Errorf("the collection task is missing: %v", keys(c))
	}
	if _, has := c["p:tasks/bulk"]; !has {
		t.Errorf("the type's collection action is missing: %v", keys(c))
	}
	if _, has := c["p:tasks/archive"]; has {
		t.Errorf("an object action leaked into the collection list: %v", keys(c))
	}
	for _, in := range []actionsapi.ListInput{{Scope: cwmapi.ActionScopeObject}, {Scope: "nonsense", ID: "m.r8"}} {
		if _, err := s.List(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%+v: %v, want refused", in, err)
		}
	}
}
