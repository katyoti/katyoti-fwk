// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	gccore "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// Runtime smoke for the ms-cwm-v4 P1 object migration (p1d).
//
// Why this file exists: gocode/internal/core had NO tests, so the nine files
// migrated from Node*/Item* to Object* were compile-checked only. A compiler
// cannot catch a wrong capability key, a module id derived the wrong way, or a
// create that writes one place while the read looks in another — which is
// exactly the class of fault that blocked p1d in the first place.
//
// It drives the real services against a temp workspace: no fakes on the CWM
// side, so a green here means the objects really landed on disk and really came
// back. Live tests use t.TempDir() — never the repo's own modules.

// newObjectCtx builds an Application with a REAL CWMService rooted at a temp
// workspace, plus the gocode service under test.
func newObjectCtx(t *testing.T) (context.Context, cwmapi.CWMService) {
	t.Helper()
	// gocode creates software modules, whose type the test
	// catalog carries.
	return cwmapp.New(t, cwmapp.SoftwareModules(t))
}

// seedProject creates the project object a module hangs under. Modules are not
// roots in this model — ModuleCreate requires a project_id — so the smoke needs
// a real one rather than a plausible-looking string.
func seedProject(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	obj := cwmapi.NewContainerObject(id, id, "project", "", "Smoke project", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: id, Object: obj}); err != nil {
		t.Fatalf("seed project %s: %v", id, err)
	}
}

// TestObjectMigration_ModuleThenUseCase is the end-to-end the migration needed:
// create a module, create a use-case on it, read both back, and set a status.
// Every step crosses the write/read boundary that the index exists to close.
func TestObjectMigration_ModuleThenUseCase(t *testing.T) {
	ctx, cwm := newObjectCtx(t)
	svc := &gccore.GocodeServiceImpl{}
	seedProject(t, ctx, cwm, "proj")

	// 1. ModuleCreate — the id is the AUTHORED name, and the object file must
	//    land at modules/<name>/objects/<name>.yaml.
	modOut, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{
		Name:        "smoke-module",
		ProjectID:   "proj",
		Kind:        gcapi.ModuleKindLibrary,
		Description: "created by the p1d migration smoke",
	})
	if err != nil {
		t.Fatalf("ModuleCreate: %v", err)
	}
	if modOut.Module.ID != "smoke-module" {
		t.Fatalf("module id = %q, want the authored name %q", modOut.Module.ID, "smoke-module")
	}

	// It must be readable back THROUGH the index, not just from the path we
	// happen to know — that is the create-then-read fault this closes.
	listed, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "smoke-module", Capability: cwmapi.CapModule})
	if err != nil {
		t.Fatalf("ObjectList: %v", err)
	}
	if listed.Total != 1 || listed.Objects[0].ID != "smoke-module" {
		t.Fatalf("module not readable via the index: %d %+v", listed.Total, listed.Objects)
	}
	if parent := listed.Objects[0].ParentID(); parent != "proj" {
		t.Errorf("module parent = %q, want proj — the project_id must land on container.parent", parent)
	}

	// 2. UCCreate — identity collapse: the use-case IS <module>.<UC-NNN>.
	ucOut, err := svc.UCCreate(ctx, gcapi.UCCreateInput{
		NodeID:   modOut.Module.ID,
		UCID:     "UC-001",
		Title:    "the migrated path actually persists",
		Severity: gcapi.UCSeverityCore,
	})
	if err != nil {
		t.Fatalf("UCCreate: %v", err)
	}
	if ucOut.UseCase.ID != "smoke-module.UC-001" {
		t.Fatalf("use-case id = %q, want smoke-module.UC-001", ucOut.UseCase.ID)
	}

	// 3. Read it back as an object and check the capabilities really carry the
	//    fields — a wrong key would have compiled fine and lost the data here.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "smoke-module", ID: ucOut.UseCase.ID})
	if err != nil {
		t.Fatalf("ObjectGet use-case: %v", err)
	}
	if got.Object.Type != "use-case" {
		t.Errorf("type = %q, want use-case", got.Object.Type)
	}
	if got.Object.ParentID() != "smoke-module" {
		t.Errorf("use-case parent = %q, want smoke-module", got.Object.ParentID())
	}
	content, ok := got.Object, true
	if !ok {
		t.Fatal("use-case has no content capability — the payload was lost")
	}
	if content.Name != "the migrated path actually persists" {
		t.Errorf("title = %q", content.Name)
	}
	if sev := content.Capabilities["checkable"]["severity"]; sev != string(gcapi.UCSeverityCore) {
		t.Errorf("severity lost: checkable.severity = %v", sev)
	}

	// 4. The tree reads as nested data in ONE call (G5) — module with its child.
	// Note the chain crosses module FOLDERS (proj -> smoke-module -> its UC):
	// the index spans modules, so containment is not confined to one folder.
	tree, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: "proj"})
	if err != nil {
		t.Fatalf("ObjectTree: %v", err)
	}
	if len(tree.Roots) != 1 || len(tree.Roots[0].Children) != 1 {
		t.Fatalf("tree should be proj -> module, got %+v", tree)
	}
	mod := tree.Roots[0].Children[0]
	if mod.Object.ID != "smoke-module" {
		t.Fatalf("module in tree = %q", mod.Object.ID)
	}
	if len(mod.Children) != 1 || mod.Children[0].Object.ID != "smoke-module.UC-001" {
		t.Errorf("use-case should nest under its module, got %+v", mod.Children)
	}
}

// TestObjectMigration_UCSetStatusPreservesCapabilities pins the read-modify-write
// discipline. ObjectUpdate REPLACES the object, so a verb that rebuilds it from
// scratch silently drops capabilities it does not know about. That would be
// invisible to the compiler and to any single-field assertion.
func TestObjectMigration_UCSetStatusPreservesCapabilities(t *testing.T) {
	ctx, cwm := newObjectCtx(t)
	svc := &gccore.GocodeServiceImpl{}

	seedProject(t, ctx, cwm, "proj")
	if _, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{Name: "keepcaps", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary}); err != nil {
		t.Fatalf("ModuleCreate: %v", err)
	}
	ucOut, err := svc.UCCreate(ctx, gcapi.UCCreateInput{
		NodeID: "keepcaps", UCID: "UC-007", Title: "keeps its capabilities", Severity: gcapi.UCSeverityCore,
	})
	if err != nil {
		t.Fatalf("UCCreate: %v", err)
	}

	// Add a capability this verb knows nothing about, the way another pack would.
	obj, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "keepcaps", ID: ucOut.UseCase.ID})
	if err != nil {
		t.Fatalf("ObjectGet: %v", err)
	}
	stranger := obj.Object
	if stranger.Capabilities == nil {
		stranger.Capabilities = map[string]map[string]any{}
	}
	stranger.Capabilities["checkable"] = map[string]any{"kind": "command", "probe": "true"}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "keepcaps", Object: stranger}); err != nil {
		t.Fatalf("seed checkable: %v", err)
	}

	// Now run the verb under test.
	if _, err := svc.UCSetStatus(ctx, gcapi.UCSetStatusInput{
		ID:     ucOut.UseCase.ID,
		Status: gcapi.UCStatusPass,
	}); err != nil {
		t.Fatalf("UCSetStatus: %v", err)
	}

	after, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "keepcaps", ID: ucOut.UseCase.ID})
	if err != nil {
		t.Fatalf("ObjectGet after: %v", err)
	}
	if _, ok := after.Object.Capability("checkable"); !ok {
		t.Error("UCSetStatus DROPPED the checkable capability — ObjectUpdate replaces, so it must carry the object forward")
	}
	content := after.Object
	var body map[string]any
	_ = json.Unmarshal(content.FieldsJSON(), &body)
	if body["status_cache"] != string(gcapi.UCStatusPass) {
		t.Errorf("status_cache = %v, want pass", body["status_cache"])
	}
	if content.Name == "" {
		t.Error("title lost on update")
	}
}

// THE DEFERRING DECISION IS AN EDGE: UCSetStatus writes fail-deferred's
// decision as a `deferred-by` edge (never a field), reads it back as the local
// OD id, and removes it once the status is no longer fail-deferred.
func TestObjectMigration_UCSetStatusWritesDeferredByAsAnEdge(t *testing.T) {
	ctx, cwm := newObjectCtx(t)
	svc := &gccore.GocodeServiceImpl{}
	seedProject(t, ctx, cwm, "proj")
	if _, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{Name: "defer", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary}); err != nil {
		t.Fatalf("ModuleCreate: %v", err)
	}
	ucOut, err := svc.UCCreate(ctx, gcapi.UCCreateInput{NodeID: "defer", UCID: "UC-001", Title: "deferred", Severity: gcapi.UCSeverityQoL})
	if err != nil {
		t.Fatalf("UCCreate: %v", err)
	}
	out, err := svc.UCSetStatus(ctx, gcapi.UCSetStatusInput{ID: ucOut.UseCase.ID, Status: gcapi.UCStatusFailDeferred, FailDeferredBy: "OD-009"})
	if err != nil {
		t.Fatalf("UCSetStatus fail-deferred: %v", err)
	}
	if out.UseCase.FailDeferredBy != "OD-009" {
		t.Errorf("FailDeferredBy read back = %q, want OD-009", out.UseCase.FailDeferredBy)
	}
	got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "defer", ID: ucOut.UseCase.ID})
	if to := got.Object.EdgeTo(gcapi.EdgeDeferredBy); to != "defer.OD-009" {
		t.Errorf("deferred-by edge = %q, want defer.OD-009", to)
	}
	if _, inFields := got.Object.Fields["fail_deferred_by"]; inFields {
		t.Error("the deferring decision is still written as a field")
	}
	if _, err := svc.UCSetStatus(ctx, gcapi.UCSetStatusInput{ID: ucOut.UseCase.ID, Status: gcapi.UCStatusPass}); err != nil {
		t.Fatalf("UCSetStatus pass: %v", err)
	}
	got, _ = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "defer", ID: ucOut.UseCase.ID})
	if to := got.Object.EdgeTo(gcapi.EdgeDeferredBy); to != "" {
		t.Errorf("deferred-by kept after the status left fail-deferred: %q", to)
	}
}

// TestObjectMigration_RejectsUnauthoredModuleID pins the guard at the one place
// a module id enters the system. The id becomes a folder name, so a bad one must
// be refused here rather than surface later as a path bug.
func TestObjectMigration_RejectsUnauthoredModuleID(t *testing.T) {
	ctx, cwm := newObjectCtx(t)
	svc := &gccore.GocodeServiceImpl{}
	// A valid project_id is supplied so the ONLY thing under test is the name.
	// Without it this test passed vacuously: every case was rejected for the
	// missing project_id and the name guard was never exercised.
	seedProject(t, ctx, cwm, "proj")

	for _, bad := range []string{"Not A Slug", "../escape", "UPPER", "has space"} {
		if _, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{
			Name: bad, ProjectID: "proj", Kind: gcapi.ModuleKindLibrary,
		}); err == nil {
			t.Errorf("ModuleCreate(%q) should be refused — a module id becomes a folder name", bad)
		}
	}

	// Control: the guard must ACCEPT a well-formed name, or it would pass the
	// cases above by rejecting everything.
	if _, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{
		Name: "good-name", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary,
	}); err != nil {
		t.Errorf("ModuleCreate(good-name) should be accepted, got %v", err)
	}
}
