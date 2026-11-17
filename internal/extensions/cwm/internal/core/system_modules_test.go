// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// SYSTEM MODULES (2026-09-26): `sys-<name>` holds the platform's committed
// state; the name is reserved both ways. (Creating the inbox is the inbox
// extension's — its Ensure, dispatched by the `bootstrap` hook.)

// systemTypes declares the SHIPPED system-module type (ktw-agents' pack), so
// the fixture cannot drift from it, beside plain test types.
func systemTypes(t *testing.T) []api.ObjectType {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm", "object-types", "system-module.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var shipped api.ObjectType
	if err := config.DecodeBytes(raw, marshal.YAML, &shipped); err != nil {
		t.Fatal(err)
	}
	return []api.ObjectType{
		shipped,
		{Name: "module", Capabilities: []string{"container", "module"}, Tree: api.TreeRoot},
		{Name: "inbox", Capabilities: []string{"container"}, Tree: api.TreeNode},
		{Name: "approval-request", Capabilities: []string{"container", "content"}, Folder: "approvals"},
	}
}

func systemFixture(t *testing.T, root, project string, extra ...api.Object) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project}
	cfg.CatalogTypes.Objects = systemTypes(t)
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}, {Name: "module"}, {Name: "system-module"}, {Name: "contributions"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// TestApprovalRequestsAreCommittedInTheSystemInbox — what an agent files into
// the inbox lands in modules/sys-inbox/approvals/, not in .katyoti.
func TestApprovalRequestsAreCommittedInTheSystemInbox(t *testing.T) {
	root, project := freshWorkspace(t), t.TempDir()
	svc := systemFixture(t, root, project)
	// The module first, as the inbox extension's Ensure does: a parent names
	// an object of the project.
	mod := api.Object{ID: "sys-inbox", Type: "system-module"}
	mod.Name = "sys-inbox"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "sys-inbox", Object: mod}); err != nil {
		t.Fatalf("seed the module: %v", err)
	}
	inbox := api.Object{ID: "sys-inbox.inbox", Type: "inbox"}
	inbox.Name = "Inbox"
	inbox.Parent = "sys-inbox"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "sys-inbox", Object: inbox}); err != nil {
		t.Fatalf("seed the inbox: %v", err)
	}
	req := api.NewRecordObject("sys-inbox", "approval-1", "approval-request", "sys-inbox.inbox", "Q?", json.RawMessage(`{}`))
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "sys-inbox", Object: req}); err != nil {
		t.Fatalf("file a request: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(project, "modules", "sys-inbox", "approvals", "*.cwm.*"))
	if len(matches) != 1 {
		t.Errorf("the request must be committed under modules/sys-inbox/approvals/, found %v", matches)
	}
}

// TestSystemModuleNameIsReservedBothWays — `sys-*` needs the capability, and
// the capability needs the name.
func TestSystemModuleNameIsReservedBothWays(t *testing.T) {
	root, project := freshWorkspace(t), t.TempDir()
	svc := systemFixture(t, root, project)
	ctx := stdctx.Background()
	squat := api.Object{ID: "sys-mine", Type: "module"}
	squat.Name = "sys-mine"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "sys-mine", Object: squat}); err == nil || !strings.Contains(err.Error(), "reserved prefix") {
		t.Errorf("a plain module may not take a sys- name: %v", err)
	}
	hidden := api.Object{ID: "journal", Type: "system-module"}
	hidden.Name = "journal"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "journal", Object: hidden}); err == nil || !strings.Contains(err.Error(), "must be named sys-") {
		t.Errorf("a system module must take a sys- name: %v", err)
	}
	ok := api.Object{ID: "sys-journal", Type: "system-module"}
	ok.Name = "sys-journal"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "sys-journal", Object: ok}); err != nil {
		t.Errorf("a system module named sys-*: %v", err)
	}
}

// TestAHandWrittenModuleBreakingTheReservationIsReported — a module file
// written by hand bypasses the write path; the read reports what the write
// would have refused: a plain module named sys-*, and a system module that is
// not.
func TestAHandWrittenModuleBreakingTheReservationIsReported(t *testing.T) {
	root, project := freshWorkspace(t), t.TempDir()
	svc := systemFixture(t, root, project)
	ctx := stdctx.Background()
	hand := map[string]string{
		"sys-x":   "id: sys-x\ntype: module\nname: sys-x\n",
		"journal": "id: journal\ntype: system-module\nname: journal\n",
		"sys-ok":  "id: sys-ok\ntype: system-module\nname: sys-ok\n",
		"billing": "id: billing\ntype: module\nname: billing\n",
	}
	for m, body := range hand {
		dir := filepath.Join(project, "modules", m, "objects")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, m+".cwm.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	list, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, o := range list.Objects {
		for _, e := range o.Errors {
			got[o.ID] += e.Label + "; "
		}
	}
	if !strings.Contains(got["sys-x"], "reserved prefix") {
		t.Errorf("a hand-written plain module named sys-x is not reported: %q", got["sys-x"])
	}
	if !strings.Contains(got["journal"], "must be named sys-") {
		t.Errorf("a hand-written system module not named sys-* is not reported: %q", got["journal"])
	}
	for _, ok := range []string{"sys-ok", "billing"} {
		if got[ok] != "" {
			t.Errorf("%s keeps the rule and is reported: %q", ok, got[ok])
		}
	}
}

// TestASystemModuleOperatesADeclaredCapability — a system module may say
// which capability it operates (sys-contributions operates contributions):
// a declared capability is accepted, a name no capability answers to is
// refused on the write and reported on the read.
func TestASystemModuleOperatesADeclaredCapability(t *testing.T) {
	root, project := freshWorkspace(t), t.TempDir()
	svc := systemFixture(t, root, project)
	ctx := stdctx.Background()
	operating := func(id, capability string) api.Object {
		o := api.Object{ID: id, Type: "system-module", Capabilities: map[string]map[string]any{"system-module": {"operates": capability}}}
		o.Name = id
		return o
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "sys-contributions", Object: operating("sys-contributions", "contributions")}); err != nil {
		t.Fatalf("a system module operating a declared capability: %v", err)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "sys-contributions"})
	if err != nil || api.SystemModuleOperates(got.Object) != "contributions" {
		t.Fatalf("operates = %q (%v), want contributions", api.SystemModuleOperates(got.Object), err)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "sys-ghost", Object: operating("sys-ghost", "ghosts")}); err == nil || !strings.Contains(err.Error(), `operates "ghosts"`) {
		t.Errorf("a system module operating an undeclared capability: %v; want refused", err)
	}
	dir := filepath.Join(project, "modules", "sys-ghost2", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sys-ghost2.cwm.yaml"), []byte("id: sys-ghost2\ntype: system-module\nname: sys-ghost2\ncapabilities:\n  system-module:\n    operates: ghosts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	reported := false
	for _, o := range list.Objects {
		for _, e := range o.Errors {
			if o.ID == "sys-ghost2" && strings.Contains(e.Label+e.Description, "ghosts") {
				reported = true
			}
		}
	}
	if !reported {
		t.Errorf("a hand-written system module operating an undeclared capability is not reported: %+v", list.Objects)
	}
}
