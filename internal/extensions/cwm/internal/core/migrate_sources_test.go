// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a carrier's sources of one connector merge into the first by
// id - its selection the union of theirs, the documents anchored under the
// others moved under it - and the others are removed; a carrier's single
// source of another connector is left alone.
func TestACarriersSourcesOfOneConnectorMerge(t *testing.T) {
	svc, project := migrationFixture(t)
	dir := filepath.Join(project, api.DefaultModulesDir, "m", ObjectsDirName)
	write := func(o api.Object) {
		t.Helper()
		if err := WriteObjectFile(filepath.Join(dir, o.ID+ObjectFileSuffix), o); err != nil {
			t.Fatal(err)
		}
	}
	source := func(id, connector string, keys ...string) api.Object {
		items := []any{}
		for _, k := range keys {
			items = append(items, map[string]any{"key": k, "title": k})
		}
		o := api.Object{ID: id, Type: "information-source", Name: id, Parent: "m", Capabilities: map[string]map[string]any{}}
		raw, _ := json.Marshal(map[string]any{"connector": connector, "selection": map[string]any{"items": items}})
		o.SetFieldsJSON(raw)
		return o
	}
	write(source("m.leanix-application-1", "leanix-application", "A1", "A2"))
	write(source("m.leanix-application-2", "leanix-application", "A2", "A3"))
	write(source("m.confluence", "confluence", "P1"))
	write(api.Object{ID: "m.leanix-application-2-acme", Type: "source-document", Name: "Acme", Parent: "m.leanix-application-2", Capabilities: map[string]map[string]any{}})

	svc.steps = []MigrationStep{{Title: "merge", Corpus: mergeSourcesPerConnector}}
	out, err := svc.Migrate(stdctx.Background(), api.MigrateInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 1 || len(out.Steps[0].Removed) != 1 {
		t.Fatalf("one source removed: %+v", out.Steps)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.leanix-application-2"+ObjectFileSuffix)); !os.IsNotExist(err) {
		t.Fatalf("the merged source's file stays: %v", err)
	}
	kept, err := ReadObjectFile(filepath.Join(dir, "m.leanix-application-1"+ObjectFileSuffix))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Selection struct {
			Items []struct{ Key string } `json:"items"`
		} `json:"selection"`
	}
	_ = json.Unmarshal(kept.FieldsJSON(), &f)
	var keys []string
	for _, it := range f.Selection.Items {
		keys = append(keys, it.Key)
	}
	if len(keys) != 3 || keys[0] != "A1" || keys[2] != "A3" {
		t.Fatalf("the kept source's selection is the union: %v", keys)
	}
	doc, err := ReadObjectFile(filepath.Join(dir, "m.leanix-application-2-acme"+ObjectFileSuffix))
	if err != nil || doc.Parent != "m.leanix-application-1" {
		t.Fatalf("the document moves under the kept source: %v %+v", err, doc.Parent)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.confluence"+ObjectFileSuffix)); err != nil {
		t.Fatalf("a single source of another connector stays: %v", err)
	}
}

// : data version 13 drops the include_generated setting of an
// information source and of the knowledge capability; the rest stays.
func TestTheIncludeGeneratedSettingIsDropped(t *testing.T) {
	src := api.Object{ID: "m.docs", Type: "information-source", Capabilities: map[string]map[string]any{}}
	src.SetFieldsJSON([]byte(`{"connector":"gen-docs","include_generated":false,"sync":"daily"}`))
	if changed, err := dropIncludeGenerated(&src); err != nil || !changed {
		t.Fatalf("source: %v %v", changed, err)
	}
	if got := string(src.FieldsJSON()); strings.Contains(got, "include_generated") || !strings.Contains(got, `"sync":"daily"`) {
		t.Fatalf("source fields: %s", got)
	}
	mod := api.Object{ID: "m", Type: "module", Capabilities: map[string]map[string]any{"knowledge": {"include_generated": true}}}
	if changed, err := dropIncludeGenerated(&mod); err != nil || !changed {
		t.Fatalf("carrier: %v %v", changed, err)
	}
	if g, _ := mod.Capability("knowledge"); len(g) != 0 {
		t.Fatalf("knowledge: %v", g)
	}
	other := api.Object{ID: "m.F-001", Type: "feature", Capabilities: map[string]map[string]any{}}
	if changed, _ := dropIncludeGenerated(&other); changed {
		t.Fatal("an object with no such setting changed")
	}
}

// : data version 14 moves the confluence-page and
// confluence-space sources onto the one confluence connector - a location
// becomes the selection, the page or space:<KEY> - and merges a carrier's
// Confluence sources into one; a space narrowed by a query keeps its location.
func TestTheRetiredConfluenceConnectorsMoveOntoConfluence(t *testing.T) {
	svc, project := migrationFixture(t)
	dir := filepath.Join(project, api.DefaultModulesDir, "m", ObjectsDirName)
	write := func(id string, fields map[string]any) {
		t.Helper()
		o := api.Object{ID: id, Type: "information-source", Name: id, Parent: "m", Capabilities: map[string]map[string]any{}}
		raw, _ := json.Marshal(fields)
		o.SetFieldsJSON(raw)
		if err := WriteObjectFile(filepath.Join(dir, o.ID+ObjectFileSuffix), o); err != nil {
			t.Fatal(err)
		}
	}
	write("m.confluence", map[string]any{"connector": "confluence", "selection": map[string]any{"items": []any{map[string]any{"key": "P1"}}}})
	write("m.confluence-page-1", map[string]any{"connector": "confluence-page", "location": map[string]any{"page": "P9"}})
	write("m.confluence-space", map[string]any{"connector": "confluence-space", "location": map[string]any{"space": "AUP"}})
	write("m.narrowed", map[string]any{"connector": "confluence-space", "location": map[string]any{"space": "ARC", "query": "Acme"}})

	svc.steps = []MigrationStep{{Title: "one confluence", Corpus: oneConfluenceConnector}}
	out, err := svc.Migrate(stdctx.Background(), api.MigrateInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 1 || len(out.Steps[0].Removed) != 2 {
		t.Fatalf("two sources merged away: %+v", out.Steps)
	}
	var f struct {
		Connector string         `json:"connector"`
		Location  map[string]any `json:"location"`
		Selection struct {
			Items []struct{ Key string } `json:"items"`
		} `json:"selection"`
	}
	kept, err := ReadObjectFile(filepath.Join(dir, "m.confluence"+ObjectFileSuffix))
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(kept.FieldsJSON(), &f)
	var keys []string
	for _, it := range f.Selection.Items {
		keys = append(keys, it.Key)
	}
	if strings.Join(keys, ",") != "P1,P9,space:AUP" || f.Location != nil {
		t.Fatalf("the kept source holds the page and the space, no location: %v %v", keys, f.Location)
	}
	f = struct {
		Connector string         `json:"connector"`
		Location  map[string]any `json:"location"`
		Selection struct {
			Items []struct{ Key string } `json:"items"`
		} `json:"selection"`
	}{}
	narrowed, err := ReadObjectFile(filepath.Join(dir, "m.narrowed"+ObjectFileSuffix))
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(narrowed.FieldsJSON(), &f)
	if f.Connector != "confluence" || f.Location["query"] != "Acme" {
		t.Fatalf("a narrowed space reads through confluence with its location: %+v", f)
	}
}

// : data version 15 gives every stream its daily
// release-due timer, keeping the timers it has; a stream that has it and an
// object of another type are left alone.
func TestEveryStreamGetsItsDailyReleaseDueTimer(t *testing.T) {
	cut := &api.Object{ID: "m.SM-001", Type: "stream", Capabilities: map[string]map[string]any{
		"timed": {"timers": []any{map[string]any{"name": "stream-cut", "every": "336h"}}}}}
	if changed, err := streamDueTimer(cut); err != nil || !changed {
		t.Fatalf("a periodic stream: %v %v", changed, err)
	}
	timers := cut.Capabilities["timed"]["timers"].([]any)
	if len(timers) != 2 || timers[1].(map[string]any)["name"] != "release-due" {
		t.Fatalf("its timers: %v; want stream-cut kept and release-due added", timers)
	}
	if changed, _ := streamDueTimer(cut); changed {
		t.Error("a stream that has the timer changed again")
	}
	manual := &api.Object{ID: "m.SM-002", Type: "stream"}
	if changed, _ := streamDueTimer(manual); !changed || manual.Capabilities["timed"] == nil {
		t.Errorf("a manual stream with no timer: %+v", manual.Capabilities)
	}
	if changed, _ := streamDueTimer(&api.Object{ID: "m.RL-001", Type: "release"}); changed {
		t.Error("a release changed")
	}
}

// : data version 16 moves a module's code fields to its code group -
// a code kind with them, another kind dropped - and leaves the rest of its
// module group.
func TestAModulesCodeMovesToTheCodeCapability(t *testing.T) {
	sw := &api.Object{ID: "billing", Type: "module", Capabilities: map[string]map[string]any{
		"module": {"kind": "sidecar", "code_packages": []any{"internal/billing"}, "tier_current": 2, "decisions": "none"}}}
	if changed, err := moveCodeFields(sw); err != nil || !changed {
		t.Fatalf("a module with code: %v %v", changed, err)
	}
	code, mod := sw.Capabilities["code"], sw.Capabilities["module"]
	if code["kind"] != "sidecar" || code["tier_current"] != 2 || len(code["code_packages"].([]any)) != 1 {
		t.Errorf("its code group: %v", code)
	}
	if _, has := mod["code_packages"]; has || mod["decisions"] != "none" || mod["kind"] != nil {
		t.Errorf("its module group: %v; want only its decisions left", mod)
	}
	doc := &api.Object{ID: "method", Type: "module", Capabilities: map[string]map[string]any{"module": {"kind": "methodology"}}}
	if changed, _ := moveCodeFields(doc); !changed || doc.Capabilities["code"] != nil || doc.Capabilities["module"] != nil {
		t.Errorf("a methodology module: %v; want its kind dropped and no code group", doc.Capabilities)
	}
	if changed, _ := moveCodeFields(sw); changed {
		t.Error("a moved module changed again")
	}
}

// : data version 17 gives every module its type - code packages or a
// sidecar make a software module, nothing a documentation module - and leaves
// a domain's module, a system module and a project as they are.
func TestEveryModuleTakesItsType(t *testing.T) {
	for _, c := range []struct {
		o    api.Object
		want string
	}{
		{api.Object{ID: "agent", Type: "module", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{"internal/agent"}}}}, "software-module"},
		{api.Object{ID: "journalist", Type: "module", Capabilities: map[string]map[string]any{"code": {"kind": "sidecar"}}}, "software-module"},
		{api.Object{ID: "shop", Type: "code-module", Capabilities: map[string]map[string]any{"code": {"code_packages": []any{"shop"}}}}, "software-module"},
		{api.Object{ID: "rules", Type: "module"}, "documentation"},
		{api.Object{ID: "etude", Type: "module", Aspects: []string{"etude-existant"}}, "module"},
		{api.Object{ID: "sys-inbox", Type: "system-module"}, "system-module"},
		{api.Object{ID: "ktw-agents", Type: "project"}, "project"},
	} {
		o := c.o
		if _, err := typeModules(&o); err != nil {
			t.Fatal(err)
		}
		if o.Type != c.want {
			t.Errorf("%s: %s; want %s", c.o.ID, o.Type, c.want)
		}
	}
}

// : data version 18 types a module carrying an aspect (the method's
// isa) and keeps its aspects; a module with no aspect is left to 17.
func TestAModuleWithAnAspectTakesItsTypeToo(t *testing.T) {
	isa := api.Object{ID: "leanix", Type: "module", Aspects: []string{"isa"}, Capabilities: map[string]map[string]any{"code": {"code_packages": []any{"app/ktw-isa"}}}}
	if changed, _ := typeAspectModules(&isa); !changed || isa.Type != "software-module" || len(isa.Aspects) != 1 {
		t.Errorf("an isa module with code: %s %v", isa.Type, isa.Aspects)
	}
	doc := api.Object{ID: "meetings", Type: "module", Aspects: []string{"isa"}}
	if _, _ = typeAspectModules(&doc); doc.Type != "documentation" {
		t.Errorf("an isa module with no code: %s", doc.Type)
	}
	plain := api.Object{ID: "rules", Type: "module"}
	if changed, _ := typeAspectModules(&plain); changed {
		t.Error("a module with no aspect changed: version 17's")
	}
}

// : data version 19 types a bare project as a software project and
// leaves a project of another type.
func TestABareProjectIsASoftwareProject(t *testing.T) {
	p := api.Object{ID: "ktw-agents", Type: "project"}
	if changed, _ := typeSoftwareProject(&p); !changed || p.Type != "software-project" {
		t.Errorf("a bare project: %s", p.Type)
	}
	d := api.Object{ID: "demo", Type: "dossier-architecture"}
	if changed, _ := typeSoftwareProject(&d); changed {
		t.Error("a dossier project changed")
	}
}

// : a project whose type needs a solution the universe was not
// installed from is refused at opening, naming the solution; a type needing
// none, or the right universe, opens.
func TestAProjectWhoseTypeNeedsAnotherSolutionIsRefused(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, api.ProjectFileName), []byte("id: demo\ntype: dossier-architecture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	types := []api.ObjectType{{Name: "dossier-architecture", Label: "Dossier d'architecture", Solution: "archifactory"}, {Name: "software-project"}}
	if err := refuseMissingSolution(root, types, "archifactory@2026.10", "dev"); err != nil {
		t.Errorf("in an ArchiFactory universe: %v", err)
	}
	err := refuseMissingSolution(root, types, "", "dev")
	if err == nil || !strings.Contains(err.Error(), "archifactory") || !strings.Contains(err.Error(), "Dossier d'architecture") {
		t.Errorf("in a universe of no solution: %v; want refused naming archifactory", err)
	}
	if err := os.WriteFile(filepath.Join(root, api.ProjectFileName), []byte("id: demo\ntype: software-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refuseMissingSolution(root, types, "", "dev"); err != nil {
		t.Errorf("a software project: %v", err)
	}
}

// : a universe with no solution is told the verb that gives it
// one, naming the universe and the solution; a universe of another solution
// (which cannot take a second) is told to open the project elsewhere, with no
// verb.
func TestTheRefusalOfAProjectNeedingASolutionNamesTheVerb(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, api.ProjectFileName), []byte("id: demo\ntype: dossier-architecture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	types := []api.ObjectType{{Name: "dossier-architecture", Solution: "archifactory"}}
	err := refuseMissingSolution(root, types, "", "acme")
	if err == nil || !strings.Contains(err.Error(), "katyoti universe set-solution acme archifactory") {
		t.Errorf("in a universe of no solution: %v; want the verb katyoti universe set-solution acme archifactory", err)
	}
	err = refuseMissingSolution(root, types, "softwarefactory@1.007", "acme")
	if err == nil || !strings.Contains(err.Error(), "open it in a universe of archifactory") || strings.Contains(err.Error(), "set-solution") {
		t.Errorf("in a universe of another solution: %v; want open it in a universe of archifactory, no verb", err)
	}
}
