// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

func sampleObject() api.Object {
	return api.Object{
		ID:     "cwm.g1-module-self-contained",
		Type:   "feature",
		Name:   "A module is self-contained",
		Parent: "cwm",
		Capabilities: map[string]map[string]any{
			"want": {"reachability": "closable"},
		},
		Edges: []api.ObjectEdge{{Kind: "realizes", To: "internal/extensions/cwm"}},
	}
}

// TestObjectCreate_WritesObjectFile is the anti-facade check for p1c: the Object
// API must write a REAL file at the decided location, not wrap node internals.
// It asserts the file exists on disk at modules/<module>/objects/<id-minus-prefix>.yaml
// and that reading it back yields the same object.
func TestObjectCreate_WritesObjectFile(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	obj := sampleObject()
	out, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: obj})
	if err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}

	// The file is where the convention says, with the module prefix stripped.
	want := filepath.Join(root, "modules", "cwm", "objects", "g1-module-self-contained.cwm.yaml")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected a real object file at %s: %v", want, err)
	}
	if out.Path != "modules/cwm/objects/g1-module-self-contained.cwm.yaml" {
		t.Errorf("reported path = %q, want the repo-relative object path", out.Path)
	}

	// And it round-trips through the service.
	got, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "cwm", ID: obj.ID})
	if err != nil {
		t.Fatalf("ObjectGet: %v", err)
	}
	if got.Object.ID != obj.ID || got.Object.Capabilities["want"]["reachability"] != "closable" {
		t.Errorf("round-trip lost data: %+v", got.Object)
	}
}

// TestObjectCreate_RefusesDuplicate pins that Create does not overwrite — objects
// carry AUTHORED ids, so a collision is a real condition, not a uuid impossibility.
func TestObjectCreate_RefusesDuplicate(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	in := api.ObjectCreateInput{Module: "cwm", Object: sampleObject()}
	if _, err := svc.ObjectCreate(stdctx.Background(), in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err = svc.ObjectCreate(stdctx.Background(), in)
	if !errors.Is(err, api.ErrAlreadyExists) {
		t.Errorf("second create err = %v, want ErrAlreadyExists", err)
	}
}

// TestObjectUpdateDelete pins the other half of the lifecycle: Update requires an
// existing object (it does not create), and Delete removes the file.
func TestObjectUpdateDelete(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	obj := sampleObject()

	// Update before create must fail — Update does not create.
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: obj}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("update-before-create err = %v, want ErrNotFound", err)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj}); err != nil {
		t.Fatalf("create: %v", err)
	}

	obj.Capabilities["want"]["reachability"] = "asymptotic"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: obj}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: obj.ID})
	if got.Object.Capabilities["want"]["reachability"] != "asymptotic" {
		t.Errorf("update did not persist: %v", got.Object.Capabilities["want"])
	}

	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: obj.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: obj.ID}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("get after delete err = %v, want ErrNotFound", err)
	}
}

// TestObjectPath_RejectsTraversal pins that an id or module is a single segment —
// never a path. Without this, an authored id could escape the module folder.
func TestObjectPath_RejectsTraversal(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	for _, bad := range []struct{ module, id string }{
		{"cwm", "../../etc/passwd"},
		{"../..", "x"},
		{"cwm", "a/b"},
	} {
		_, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: bad.module, ID: bad.id})
		if !errors.Is(err, api.ErrInvalidInput) {
			t.Errorf("module=%q id=%q err = %v, want ErrInvalidInput", bad.module, bad.id, err)
		}
	}
}

// TestObjectCreate_QualifiesABareID: an authored id is module-qualified as a
// minted one is — `ac-001` given in module cwm is cwm.ac-001 — and an id naming
// another module is refused.
func TestObjectCreate_QualifiesABareID(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	obj := sampleObject()
	obj.ID = "ac-001"
	out, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: obj})
	if err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	if out.Object.ID != "cwm.ac-001" {
		t.Fatalf("created id = %q, want cwm.ac-001", out.Object.ID)
	}
	if _, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "cwm", ID: "cwm.ac-001"}); err != nil {
		t.Fatalf("read back cwm.ac-001: %v", err)
	}
	other := sampleObject()
	other.ID = "billing.ac-002"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: other}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("an id of another module = %v, want refused", err)
	}
}
