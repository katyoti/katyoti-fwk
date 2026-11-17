// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a repository not yet converted - its data older than the typing
// of its modules - opens and works as before: its module typed module is read
// with no error, at the module level, carrying its lifecycle; a new object of
// the retired type is refused; DeclarationFindings says the conversion is
// waiting. Once `cwm migrate` converted it, the module is typed documentation,
// the finding is gone and the retired type is undeclared again.
func TestAnUnconvertedRepositoryWorksAsBefore(t *testing.T) {
	types := []api.ObjectType{
		{Name: "software-project", Capabilities: []string{api.CapProject}},
		{Name: "documentation", Capabilities: []string{api.CapModule, "lifecycle"}, Tree: "root"},
		{Name: "software-module", Capabilities: []string{api.CapModule, "code", "lifecycle"}, Tree: "root"},
		{Name: "note"},
	}
	svc, root := projectFixture(t, types)
	ctx := stdctx.Background()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Data version 16: before IN-036 typed the modules.
	write(api.ProjectFileName, "id: old\ntype: project\nname: Old\ncapabilities:\n  project:\n    uuid: 00000000-0000-7000-8000-0000000000c3\n    data_version: 16\n")
	write("modules/billing/objects/billing.cwm.yaml", "id: billing\ntype: module\nname: Billing\ncapabilities:\n  module: {}\n")
	write("modules/billing/notes/N-001.cwm.yaml", "id: billing.N-001\ntype: note\nname: a note\nparent: billing\n")
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing"})
	if err != nil || got.Object.Type != "module" {
		t.Fatalf("the unconverted module is not read: %v %+v", err, got.Object)
	}
	errs, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{Module: "billing"})
	if err != nil || errs.Total != 0 {
		t.Errorf("the unconverted module reads with errors: %v %+v", err, errs)
	}
	eff, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: "billing"})
	var caps []string
	for _, c := range eff.Capabilities {
		caps = append(caps, c.ID)
	}
	if err != nil || !strings.Contains(strings.Join(caps, ","), api.CapModule) || !strings.Contains(strings.Join(caps, ","), "lifecycle") {
		t.Errorf("the unconverted module's capabilities: %v %v; want module and lifecycle", err, caps)
	}
	mods, err := svc.ObjectList(ctx, api.ObjectListInput{Capability: api.CapModule})
	if err != nil || len(mods.Objects) != 1 {
		t.Errorf("modules listed by the level capability: %v %+v", err, mods.Objects)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "shop", Object: api.Object{ID: "shop", Type: "module", Name: "Shop"}}); err == nil || !strings.Contains(err.Error(), "retired type") {
		t.Errorf("a new object of the retired type: %v; want refused", err)
	}
	// Its project, of the retired bare project type, reads too.
	all, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil || all.Total != 0 {
		t.Errorf("the unconverted repository reads with errors: %v %+v", err, all)
	}
	if p, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "old"}); err != nil || p.Object.Type != "project" {
		t.Errorf("the unconverted project is not read: %v %+v", err, p.Object)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "tools", Object: api.Object{ID: "tools", Type: "project", Name: "Tools"}}); err == nil || !strings.Contains(err.Error(), "software-project") {
		t.Errorf("a new object of the retired project type: %v; want refused naming what replaces it", err)
	}
	f, _ := svc.DeclarationFindings(ctx, api.DeclarationFindingsInput{})
	if !hasRule(f.Findings, "conversion-waiting") {
		t.Errorf("no finding says the conversion waits: %+v", f.Findings)
	}

	if _, err := svc.Migrate(ctx, api.MigrateInput{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing"})
	if err != nil || got.Object.Type != "documentation" {
		t.Errorf("after the conversion: %v %+v; want typed documentation", err, got.Object)
	}
	f, _ = svc.DeclarationFindings(ctx, api.DeclarationFindingsInput{})
	if hasRule(f.Findings, "conversion-waiting") {
		t.Error("the conversion is done, still said waiting")
	}
	if p, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "old"}); err != nil || p.Object.Type != "software-project" {
		t.Errorf("the project after the conversion: %v %+v; want typed software-project", err, p.Object)
	}
	for _, retired := range []string{"module", "code-module", "project"} {
		if _, ok := svc.lookupObjectType(retired); ok {
			t.Errorf("after the conversion the retired type %s still resolves", retired)
		}
	}
	if v := svc.projectDataVersion(); v != dataVersionOf(svc.steps) {
		t.Errorf("data version after the conversion: %s", strconv.Itoa(v))
	}
}

func hasRule(fs []api.DeclarationFinding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// : the refusal of the retired project type names the neutral
// replacement only - no type a solution declares (dossier-architecture is
// ArchiFactory's) in cwm's own message or in its replaced detail.
func TestTheRetiredProjectRefusalNamesNoSolutionType(t *testing.T) {
	err := legacyCreate(api.Object{ID: "demo", Type: "project"}, "create")
	if err == nil {
		t.Fatal("a new object of the retired project type is not refused")
	}
	if !strings.Contains(err.Error(), "software-project") {
		t.Errorf("the refusal does not name software-project: %v", err)
	}
	if strings.Contains(err.Error(), "dossier-architecture") {
		t.Errorf("the refusal names a solution's type: %v", err)
	}
	if r := legacyObjectTypes["project"].replaced; strings.Contains(r, "dossier-architecture") {
		t.Errorf("the replaced detail names a solution's type: %q", r)
	}
}
