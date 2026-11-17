// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE AGENT CONTEXT OF AN OBJECT, on a real CWM: one resolver layers project →
// rules → extensions → capabilities → policies → type → object, a closer entry
// replaces or removes a farther one, and what cannot be read or does not fit
// is reported.

var agentPack = fstest.MapFS{
	"capabilities/container.yaml": {Data: []byte("name: container\n")},
	"capabilities/content.yaml":   {Data: []byte("name: content\n")},
	"capabilities/goals.yaml": {Data: []byte(`name: goals
agent:
  system:
    - {id: hint, text: "capability hint"}
    - {id: how, text: "capability says how"}
  tools:
    - {uri: "x:goals/add"}
    - {uri: "x:goals/drop"}
  skills:
    - {selector: "goal-*"}
`)},
	"capabilities/memory.yaml": {Data: []byte(`name: memory
files:
  working: {path: "memory/${id}.md"}
agent:
  files:
    - {capability: memory, name: working, title: Memory}
`)},
	"object-types/module.yaml":  {Data: []byte("name: module\ncapabilities: [container]\n")},
	"object-types/project.yaml": {Data: []byte("name: project\ncapabilities: [container]\n")},
	"object-types/rule.yaml":    {Data: []byte("name: rule\ncapabilities: [container, content]\ncontext:\n  propagation: inherit-from-ancestors\n  resolution_strategy: accumulate\n")},
	"object-types/goal.yaml": {Data: []byte(`name: goal
label: Goal
capabilities: [container, content, goals, memory]
actions: ["x:tasks/refine"]
agent:
  system:
    - {id: how, text: "type says: goal of ${current.module}"}
  tools:
    - {plugin: workspace}
    - {uri: "x:goals/drop", when: "false"}
`)},
	"agent-contexts/goals.yaml": {Data: []byte(`name: goals
filter: {types: [goal]}
agent:
  system:
    - {id: goals-how, text: "extension says how goals are managed"}
  skills:
    - {selector: define-goals}
`)},
	"agent-contexts/features.yaml": {Data: []byte("name: features\nfilter: {types: [feature]}\nagent:\n  system:\n    - {id: features-how, text: \"not for goals\"}\n")},
	"agent-contexts/broken.yaml":   {Data: []byte("name: broken\nfilter: {when: \"current.type ==\"}\nagent:\n  system:\n    - {id: never, text: \"unreadable filter\"}\n")},
	"policies/config.yaml": {Data: []byte(`name: config
applies: {types: [goal]}
agent:
  system:
    - {id: cfg, text: "Config goes through the config service."}
`)},
}

func agentWorld(t *testing.T, cfg api.ObjectContextConfig, goal cwmapi.Object) (context.Context, cwmapi.CWMService, api.ObjectContextService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(agentPack), oclocal.ExtensionWith(cfg))
	cwmapp.Module(t, ctx, cwm, "billing")
	mk := func(o cwmapi.Object, parent, title string) {
		o.Parent, o.Name = parent, title
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	mk(cwmapi.Object{ID: "billing.proj", Type: "project"}, "billing", "Billing")
	mk(cwmapi.Object{ID: "billing.r1", Type: "rule"}, "billing.proj", "Money is integer cents")
	mk(goal, "billing.proj", "Invoices go out on time")
	oc, err := application.UseService[api.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, oc
}

func agentContext(t *testing.T, ctx context.Context, oc api.ObjectContextService, id string) api.AgentContextOutput {
	t.Helper()
	out, err := oc.AgentContext(ctx, api.AgentContextInput{ID: id})
	if err != nil {
		t.Fatalf("AgentContext: %v", err)
	}
	return out
}

// sectionIDs names each section by id@layer, the layer's own name cut to its
// kind for an extension (its provider varies with the host).
func sectionIDs(out api.AgentContextOutput) string {
	var ids []string
	for _, s := range out.System {
		src := s.Source
		if strings.HasPrefix(src, api.LayerExtension+":") {
			src = api.LayerExtension + ":" + src[strings.LastIndex(src, "/")+1:]
		}
		ids = append(ids, s.ID+"@"+src)
	}
	return strings.Join(ids, " ")
}

func hasNote(out api.AgentContextOutput, list, key, kind string) bool {
	for _, n := range out.Notes {
		if n.List == list && n.Key == key && n.Kind == kind {
			return true
		}
	}
	return false
}

var project = api.ObjectContextConfig{Agent: &cwmapi.AgentDecl{
	System: []cwmapi.AgentSection{{ID: "framing", Text: "This workspace builds billing."}},
	Tools:  []cwmapi.AgentTool{{URI: "x:search/all"}},
}}

func TestEverySourceReachesTheContextInOrder(t *testing.T) {
	g := cwmapi.Object{ID: "billing.g1", Type: "goal", Agent: &cwmapi.AgentDecl{
		System: []cwmapi.AgentSection{{ID: "mine", Text: "object says"}},
		Tools:  []cwmapi.AgentTool{{URI: "x:goals/close"}},
	}}
	ctx, _, oc := agentWorld(t, project, g)
	out := agentContext(t, ctx, oc, "billing.g1")

	want := "framing@project rule:billing.r1@rule:billing.r1 cfg@policy:config goals-how@extension:goals hint@capability:goals file:memory/working@capability:memory how@type:goal current-object@object mine@object"
	if got := sectionIDs(out); got != want {
		t.Fatalf("sections\n got %s\nwant %s", got, want)
	}
	var tools []string
	for _, tl := range out.Tools {
		key := tl.URI
		if key == "" {
			key = "plugin:" + tl.Plugin
		}
		tools = append(tools, key+"@"+tl.Source)
	}
	wantTools := "x:search/all@project x:goals/add@capability:goals x:tasks/refine@type:goal plugin:workspace@type:goal x:goals/close@object"
	if got := strings.Join(tools, " "); got != wantTools {
		t.Fatalf("tools\n got %s\nwant %s", got, wantTools)
	}
	if len(out.Skills) != 2 || out.Skills[0].Selector != "define-goals" || out.Skills[1].Selector != "goal-*" || out.Skills[1].Source != "capability:goals" {
		t.Fatalf("skills = %+v", out.Skills)
	}
	if !hasNote(out, "system", "broken", api.NoteError) {
		t.Errorf("an unreadable extension filter is not reported: %+v", out.Notes)
	}
	if out.System[1].Text != "Money is integer cents" {
		t.Errorf("rule section = %+v, want the rule's statement", out.System[1])
	}
}

func TestACloserSectionReplacesAndACloserFalseRemoves(t *testing.T) {
	ctx, _, oc := agentWorld(t, api.ObjectContextConfig{}, cwmapi.Object{ID: "billing.g1", Type: "goal"})
	out := agentContext(t, ctx, oc, "billing.g1")
	var how *api.Section
	for i := range out.System {
		if out.System[i].ID == "how" {
			if how != nil {
				t.Fatalf("section `how` appears twice: %s", sectionIDs(out))
			}
			how = &out.System[i]
		}
	}
	if how == nil || how.Source != "type:goal" || how.Text != "type says: goal of billing" {
		t.Fatalf("how = %+v, want the type's, interpolated", how)
	}
	for _, tl := range out.Tools {
		if tl.URI == "x:goals/drop" {
			t.Fatalf("x:goals/drop survived a closer when:false: %+v", out.Tools)
		}
	}
	if !hasNote(out, "tools", "x:goals/drop", api.NoteRemoved) {
		t.Errorf("no removed note for x:goals/drop: %+v", out.Notes)
	}
}

func TestAnUnreadableExpressionIsReportedNeverInjected(t *testing.T) {
	g := cwmapi.Object{ID: "billing.g1", Type: "goal", Agent: &cwmapi.AgentDecl{
		System: []cwmapi.AgentSection{
			{ID: "bad-template", Text: "the ${current.nope.deeper} thing"},
			{ID: "bad-when", Text: "x", When: "current.type =="},
			{ID: "off", Text: "y", When: "type == 'feature'"},
			{ID: "no-function", Text: "z", When: "verify().green"},
		},
	}}
	ctx, _, oc := agentWorld(t, api.ObjectContextConfig{}, g)
	out := agentContext(t, ctx, oc, "billing.g1")
	for _, s := range out.System {
		if strings.Contains(s.Text, "${") || s.ID == "bad-template" || s.ID == "bad-when" || s.ID == "off" || s.ID == "no-function" {
			t.Fatalf("section %q injected: %+v", s.ID, s)
		}
	}
	for _, c := range []struct{ key, kind string }{
		{"bad-template", api.NoteError}, {"bad-when", api.NoteError}, {"off", api.NoteSkipped}, {"no-function", api.NoteError},
	} {
		if !hasNote(out, "system", c.key, c.kind) {
			t.Errorf("no %s note for %s: %+v", c.kind, c.key, out.Notes)
		}
	}
}

func TestTheBudgetTrimsFarthestFirstAndReports(t *testing.T) {
	cfg := api.ObjectContextConfig{MaxBytes: 700, Agent: &cwmapi.AgentDecl{System: []cwmapi.AgentSection{{ID: "framing", Text: strings.Repeat("p", 400)}}}}
	ctx, _, oc := agentWorld(t, cfg, cwmapi.Object{ID: "billing.g1", Type: "goal"})
	out := agentContext(t, ctx, oc, "billing.g1")
	if out.Budget != 700 || out.Bytes > 700 {
		t.Fatalf("budget %d, bytes %d", out.Budget, out.Bytes)
	}
	if strings.Contains(sectionIDs(out), "framing@") || !hasNote(out, "system", "framing", api.NoteTrimmed) {
		t.Fatalf("the farthest section was not trimmed and reported: %s %+v", sectionIDs(out), out.Notes)
	}
	if !strings.Contains(sectionIDs(out), "current-object@object") {
		t.Errorf("the closest section was trimmed: %s", sectionIDs(out))
	}
}

func TestANamedFileIsReadAsASection(t *testing.T) {
	ctx, cwm, oc := agentWorld(t, api.ObjectContextConfig{}, cwmapi.Object{ID: "billing.g1", Type: "goal"})
	if out := agentContext(t, ctx, oc, "billing.g1"); !hasNote(out, "files", "memory/working", api.NoteEmpty) {
		t.Fatalf("no empty note before the file exists: %+v", out.Notes)
	}
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: "billing.g1", Capability: "memory", Name: "working", Content: []byte("customers pay late in August")}); err != nil {
		t.Fatal(err)
	}
	out := agentContext(t, ctx, oc, "billing.g1")
	var found bool
	for _, s := range out.System {
		if s.ID == "file:memory/working" {
			found = s.Text == "customers pay late in August" && s.Title == "Memory" && s.Source == "capability:memory"
		}
	}
	if !found {
		t.Fatalf("memory file not a section: %s", sectionIDs(out))
	}
}

func TestTheOutputTravelsAsJSON(t *testing.T) {
	ctx, _, oc := agentWorld(t, api.ObjectContextConfig{}, cwmapi.Object{ID: "billing.g1", Type: "goal"})
	raw, _ := json.Marshal(agentContext(t, ctx, oc, "billing.proj"))
	for _, k := range []string{`"system":[`, `"tools":[`, `"skills":[`} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("%s missing in %s", k, raw)
		}
	}
}

func TestTheCardCarriesTheTypesFields(t *testing.T) {
	g := cwmapi.Object{ID: "billing.g1", Type: "goal"}
	g.SetFieldsJSON(json.RawMessage(`{"workspace_path":"D:/dossiers/x","manufacturer":"Acme","tags":["a","b"]}`))
	ctx, _, oc := agentWorld(t, api.ObjectContextConfig{}, g)
	out := agentContext(t, ctx, oc, "billing.g1")
	var card string
	for _, s := range out.System {
		if s.ID == api.SectionCurrentObject {
			card = s.Text
		}
	}
	for _, want := range []string{"id: billing.g1", "type: goal (Goal)", "module: billing", "name: Invoices go out on time", "parent_id: billing.proj", "file: ",
		"type_specific:\n  manufacturer: Acme\n  tags: [\"a\",\"b\"]\n  workspace_path: D:/dossiers/x"} {
		if !strings.Contains(card, want) {
			t.Errorf("card lacks %q:\n%s", want, card)
		}
	}
}
