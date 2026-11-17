// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// wire is what a screen receives: each event marshalled and read back
// through panel.DecodeEvent, which validates it.
func wire(t *testing.T, events []panel.Event) []panel.Event {
	t.Helper()
	out := make([]panel.Event, 0, len(events))
	for _, e := range events {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		back, err := panel.DecodeEvent(raw)
		if err != nil {
			t.Fatalf("event %s seq %d is no valid panel event: %v", e.Kind, e.Seq, err)
		}
		out = append(out, back)
	}
	return out
}

// artifactOf is the artifact a replace op's node carries.
func artifactOf(t *testing.T, op render.Op) map[string]any {
	t.Helper()
	a, ok := op.Node["artifact"].(map[string]any)
	if !ok {
		t.Fatalf("node %v carries no artifact", op.Node)
	}
	return a
}

func onlyReplace(t *testing.T, events []panel.Event, target string) render.Op {
	t.Helper()
	if len(events) != 1 || events[0].Kind != panel.KindOps || len(events[0].Ops) != 1 {
		t.Fatalf("want one ops event with one op, got %+v", events)
	}
	op := events[0].Ops[0]
	if op.Op != render.OpReplace || op.Target != target {
		t.Fatalf("want replace of %s, got %s of %s", target, op.Op, op.Target)
	}
	return op
}

func page(id string) agentapi.UIArtifactMountEvent {
	return agentapi.UIArtifactMountEvent{ArtifactID: id, Kind: "page", Title: "Board",
		DSLTree: map[string]any{"kind": "vbox", "children": []any{
			map[string]any{"kind": "kpi", "id": "kpi-open", "label": "Open", "value": 3},
		}},
		Data: map[string]any{"items": []any{}}}
}

// The first artifact opens the run's chat panel - type chat, owned by the
// run - and shows the artifact as the node artifact:<id> of the artifacts
// node; a later one replaces the artifacts node with both, in mount order;
// a mount fired again shows nothing new.
func TestAMountOpensTheChatPanelAndShowsTheArtifactAsItsNode(t *testing.T) {
	b := New("", "s1", "run-1")
	got, err := b.Mount(page("a1"))
	if err != nil {
		t.Fatal(err)
	}
	got = wire(t, got)
	if len(got) != 2 || got[0].Kind != panel.KindOpen || got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("want open then ops, got %+v", got)
	}
	if o := got[0].Open; o.Type != Type || o.Owner != "run-1" || got[0].Panel != b.ID() || !panel.ValidID(b.ID()) {
		t.Fatalf("open = %+v on %s", o, got[0].Panel)
	}
	op := onlyReplace(t, got[1:], ArtifactsNode)
	kids, _ := op.Node["children"].([]any)
	if len(kids) != 1 || kids[0].(map[string]any)["id"] != NodeID("a1") {
		t.Fatalf("artifacts node = %v", op.Node)
	}

	m := page("a2")
	m.Kind, m.DSLTree, m.Data, m.Value = "form", nil, nil, map[string]any{"name": "x"}
	got, err = b.Mount(m)
	if err != nil {
		t.Fatal(err)
	}
	op = onlyReplace(t, wire(t, got), ArtifactsNode)
	if kids, _ := op.Node["children"].([]any); len(kids) != 2 || kids[1].(map[string]any)["id"] != NodeID("a2") {
		t.Fatalf("second mount: artifacts node = %v", op.Node)
	}
	if again, _ := b.Mount(page("a1")); len(again) != 0 {
		t.Fatalf("a mount fired again showed %+v", again)
	}
}

// Each patch the agent sends - by node id or JSON Pointer - is applied to the
// artifact's kept value and becomes one replace of the artifact's node: no
// pointer crosses into the panel ops.
func TestEachPatchIsOneReplaceOfTheArtifactsNode(t *testing.T) {
	b := New("", "s1", "run-1")
	if _, err := b.Mount(page("a1")); err != nil {
		t.Fatal(err)
	}
	got, err := b.Patch(agentapi.UIArtifactPatchEvent{ArtifactID: "a1", Ops: []any{
		map[string]any{"kind": "rfc7396", "target": "kpi-open", "mergePatch": map[string]any{"value": 4}},
		map[string]any{"kind": "rfc6902", "op": "add", "path": "/children/-", "value": map[string]any{"kind": "text", "id": "t1", "content": "hi"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	op := onlyReplace(t, wire(t, got), NodeID("a1"))
	a := artifactOf(t, op)
	kids := a["dslTree"].(map[string]any)["children"].([]any)
	if len(kids) != 2 || kids[0].(map[string]any)["value"] != float64(4) || kids[1].(map[string]any)["id"] != "t1" {
		t.Fatalf("patched tree = %v", a["dslTree"])
	}
	if a["status"] != StatusStreaming || len(b.Errors) != 0 {
		t.Fatalf("status %v errors %v", a["status"], b.Errors)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), `"path"`) {
		t.Fatalf("a pointer crossed into the panel event: %s", raw)
	}
}

// The stream plugin's def replace (one replace of the root) lays each field
// of the def over the artifact - data a page's outer data, any other
// component's value.
func TestADefReplaceLaysTheDefOverTheArtifact(t *testing.T) {
	b := New("", "s1", "run-1")
	f := agentapi.UIArtifactMountEvent{ArtifactID: "f1", Kind: "form", Schema: "ktw-x:/data-type/person"}
	if _, err := b.Mount(f); err != nil {
		t.Fatal(err)
	}
	got, err := b.Patch(agentapi.UIArtifactPatchEvent{ArtifactID: "f1", Ops: []any{
		map[string]any{"kind": "replace", "path": "", "value": map[string]any{"kind": "form", "data": map[string]any{"name": "Ada"}, "mode": "edit"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	a := artifactOf(t, onlyReplace(t, wire(t, got), NodeID("f1")))
	if !reflect.DeepEqual(a["value"], map[string]any{"name": "Ada"}) || a["mode"] != "edit" || a["schema"] != "ktw-x:/data-type/person" {
		t.Fatalf("artifact = %v", a)
	}
}

// A deck's patch edits its markdown by slide number.
func TestADeckPatchEditsItsSlides(t *testing.T) {
	b := New("", "s1", "run-1")
	md := "# One\n\n---\n\n## Two\n\n---\n\n## Three"
	if _, err := b.Mount(agentapi.UIArtifactMountEvent{ArtifactID: "d1", Kind: "deck", Value: md}); err != nil {
		t.Fatal(err)
	}
	got, err := b.Patch(agentapi.UIArtifactPatchEvent{ArtifactID: "d1", Ops: []any{
		map[string]any{"kind": "replace", "target": "slide-2", "value": "## Deux"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	a := artifactOf(t, onlyReplace(t, wire(t, got), NodeID("d1")))
	if a["value"] != "# One\n\n---\n\n## Deux\n\n---\n\n## Three" {
		t.Fatalf("deck = %q (errors %v)", a["value"], b.Errors)
	}
}

// A completion replaces the artifact's node with its status complete, once;
// a patch or a completion of an artifact the chat does not show sends
// nothing; an op that cannot apply is skipped and said, the others applied.
func TestCompletionAndUnknownArtifactsAndBadOps(t *testing.T) {
	b := New("", "s1", "run-1")
	if _, err := b.Mount(page("a1")); err != nil {
		t.Fatal(err)
	}
	got, err := b.Patch(agentapi.UIArtifactPatchEvent{ArtifactID: "a1", Ops: []any{
		map[string]any{"kind": "replace", "target": "nowhere", "value": 1},
		map[string]any{"kind": "rfc7396", "target": "kpi-open", "mergePatch": map[string]any{"label": "Ouverts"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	a := artifactOf(t, onlyReplace(t, wire(t, got), NodeID("a1")))
	if len(b.Errors) != 1 || a["dslTree"].(map[string]any)["children"].([]any)[0].(map[string]any)["label"] != "Ouverts" {
		t.Fatalf("errors %v artifact %v", b.Errors, a)
	}
	got, err = b.Complete(agentapi.UIArtifactCompleteEvent{ArtifactID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if a := artifactOf(t, onlyReplace(t, wire(t, got), NodeID("a1"))); a["status"] != StatusComplete {
		t.Fatalf("completed artifact = %v", a)
	}
	if again, _ := b.Complete(agentapi.UIArtifactCompleteEvent{ArtifactID: "a1"}); len(again) != 0 {
		t.Fatalf("a second completion sent %+v", again)
	}
	if p, _ := b.Patch(agentapi.UIArtifactPatchEvent{ArtifactID: "zz", Ops: []any{map[string]any{"kind": "remove", "path": "/x"}}}); len(p) != 0 {
		t.Fatalf("a patch of an unknown artifact sent %+v", p)
	}
	if c, _ := b.Complete(agentapi.UIArtifactCompleteEvent{ArtifactID: "zz"}); len(c) != 0 {
		t.Fatalf("a completion of an unknown artifact sent %+v", c)
	}
}
