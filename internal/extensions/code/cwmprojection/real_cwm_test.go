// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucobj"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The charter is built against a REAL CWM service. Its ObjectList / ObjectTree
// answer from the index, whose copies carry no fields and no body (the index is
// lean by design; ObjectGet reads the file). A fake that returns full objects
// from every read hid exactly that: the loader read the verdicts, the criteria
// and the charter prose off index copies, and in production saw none of them.
func TestTheCharterReadsFieldsAndBodiesThroughARealCWM(t *testing.T) {
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(), cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":               {Data: []byte("name: module\ncapabilities: [container, module]\n")},
		"object-types/use-case.yaml":             {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-types/use-case.yaml")},
		"object-types/acceptance-criterion.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-types/acceptance-criterion.yaml")},
		"object-types/test-run.yaml":             {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/test-run.yaml")},
		"object-types/charter.yaml":              {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/charter.yaml")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")

	create := func(o cwmapi.Object) {
		t.Helper()
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	uc := cwmapi.NewContainerObject("m", "UC-001", "use-case", "m", "Read a module", "")
	uc.Body = "The module reads back whole."
	uc.Capabilities["checkable"] = map[string]any{"severity": "core"}
	create(uc)
	acFields, _ := json.Marshal(map[string]any{"layer": "L1", "test_ref": "TestRead"})
	create(cwmapi.NewRecordObject("m", "UC-001-AC1", "acceptance-criterion", "m.UC-001", "it reads", acFields))
	chFields, _ := json.Marshal(map[string]any{"version": "1.2", "concepts_md": "A **module** is a unit of work."})
	create(cwmapi.NewRecordObject("m", "charter", "charter", "m", "Charter", chFields))

	ranAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	if _, err := ucobj.WriteRun(ctx, cwm, "m", ucobj.Run{RanAt: ranAt, LayerFilter: "L1", UCTotal: 1,
		Verdicts: map[string]gcapi.TestRunVerdict{"UC-001": {Verdict: "pass",
			ACs: map[string]gcapi.UCStatus{"UC-001-AC1": "pass"}}}}); err != nil {
		t.Fatal(err)
	}

	in, err := LoadCharterInputs(ctx, cwm, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(in.UCFiles) != 1 {
		t.Fatalf("use cases = %d, want 1", len(in.UCFiles))
	}
	f := in.UCFiles[0]
	if f.Meta.StatusCache != "pass" {
		t.Errorf("UC-001 status = %q, want the test-run verdict pass", f.Meta.StatusCache)
	}
	if len(f.Meta.AcceptanceCriteria) != 1 || f.Meta.AcceptanceCriteria[0].Layer != "L1" ||
		f.Meta.AcceptanceCriteria[0].TestRef != "TestRead" || f.Meta.AcceptanceCriteria[0].Status != "pass" {
		t.Errorf("criteria = %+v, want AC1 at L1 probing TestRead, pass", f.Meta.AcceptanceCriteria)
	}
	if in.LatestTestRun == nil || !in.LatestTestRun.RanAt.Equal(ranAt) {
		t.Errorf("latest test-run = %+v, want the one recorded", in.LatestTestRun)
	}
	if in.CharterItem == nil || in.CharterItem.Version != "1.2" {
		t.Errorf("charter item = %+v, want version 1.2", in.CharterItem)
	}

	md := BuildCharter(in)
	for _, want := range []string{
		"| `UC-001` | Read a module | `core` | 🟢 pass |",
		"The module reads back whole.",
		"A **module** is a unit of work.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("charter misses %q:\n%s", want, md)
		}
	}
}

// A software module IS a module: it carries the `module` capability (and its
// code). The module dashboard loads its manifest; a test on the
// type's name refused it ("want module").
func TestASoftwareModuleLoadsItsManifestThroughARealCWM(t *testing.T) {
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(), cwmapp.Pack(fstest.MapFS{
		"object-types/software-module.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/software-module.yaml")},
		"capabilities/code.yaml":            {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"object-types/project.yaml":         {Data: []byte("name: project\ntree: root\n")},
	}))
	mod := cwmapi.NewContainerObject("shop-api", "shop-api", "software-module", "", "Shop API", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shop-api", Object: mod}); err != nil {
		t.Fatalf("create software-module: %v", err)
	}
	in, err := LoadManifestInputs(ctx, cwm, "shop-api")
	if err != nil {
		t.Fatalf("LoadManifestInputs on a software-module: %v", err)
	}
	if in.Module.ID != "shop-api" {
		t.Fatalf("module id = %q, want shop-api", in.Module.ID)
	}

	// And an object that does not carry `module` is still refused.
	p := cwmapi.NewContainerObject("shop-api", "proj", "project", "", "Project", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shop-api", Object: p}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := LoadManifestInputs(ctx, cwm, "shop-api.proj"); err == nil {
		t.Fatal("LoadManifestInputs on a project: want an error, got none")
	}
}
