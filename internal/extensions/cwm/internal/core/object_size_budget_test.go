// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE SIZE BUDGET (max_size_bytes): the object file as the write leaves it,
// in BYTES. Declared by every pack that ships a budget, and enforced nowhere
// until it was checked here — a declared cap nothing enforces is a promise.

func budgeted(id, desc string) api.Object {
	return api.Object{ID: id, Type: "budgeted", Name: id, Description: desc}
}

func storedSize(t *testing.T, svc *Service, id string) int {
	t.Helper()
	path, ok := svc.index().pathOf(id)
	if !ok {
		t.Fatalf("%s is not indexed", id)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return int(st.Size())
}

// Bytes, not characters: 150 "é" are 150 characters and 300 bytes. A budget
// that fits the characters and not the bytes refuses, and the refusal carries
// the size the file would have had — the size the write path writes.
func TestSizeBudgetCountsTheFileInBytes(t *testing.T) {
	const max = 300
	svc := svcWithTypes(t, api.TypeValidationError, []api.ObjectType{{Name: "budgeted", MaxSizeBytes: max}})
	ctx := stdctx.Background()

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: budgeted("cwm.small", "court")}); err != nil {
		t.Fatalf("a small object was refused: %v", err)
	}
	if n := storedSize(t, svc, "cwm.small"); n > max {
		t.Fatalf("an accepted object is %d bytes, over %d", n, max)
	}

	_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: budgeted("cwm.big", strings.Repeat("é", 150))})
	var be *api.SizeBudgetError
	if !errors.Is(err, api.ErrBudgetExceeded) || !errors.As(err, &be) {
		t.Fatalf("an object over its budget in bytes was accepted (or wrong error): %v", err)
	}
	if be.Max != max || be.Over() <= 0 || be.ID != "cwm.big" || be.Verb != "ObjectCreate" {
		t.Errorf("refusal = %+v", be)
	}
	want, _ := encodeObjectFile(t.TempDir()+"/x"+ObjectFileSuffix, budgeted("cwm.big", strings.Repeat("é", 150)))
	if be.Bytes != len(want) {
		t.Errorf("refusal says %d bytes, the file would be %d", be.Bytes, len(want))
	}
	if _, ok := svc.index().pathOf("cwm.big"); ok {
		t.Error("a refused create left a file")
	}
}

// An update is checked too; an object already over budget (a type tightened
// under it) is repaired one write at a time — a write that does not grow it
// goes through, one that grows it does not. Its read carries the error.
func TestSizeBudgetOnUpdateAndRead(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, []api.ObjectType{{Name: "budgeted", MaxSizeBytes: 4096}})
	ctx := stdctx.Background()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: budgeted("cwm.o", strings.Repeat("漢", 100))}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: budgeted("cwm.o", strings.Repeat("漢", 2000))}); !errors.Is(err, api.ErrBudgetExceeded) {
		t.Fatalf("an update over budget was accepted: %v", err)
	}

	// The type tightens under the stored object.
	tight := reopen(t, svc, []api.ObjectType{{Name: "budgeted", MaxSizeBytes: 200}})
	got, err := tight.ObjectGet(ctx, api.ObjectGetInput{Module: "cwm", ID: "cwm.o"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiagnosticLabel(got.Errors, "over the 200 bytes") {
		t.Errorf("an object read over budget carries no error: %+v", got.Errors)
	}
	if _, err := tight.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: budgeted("cwm.o", strings.Repeat("漢", 90))}); err != nil {
		t.Errorf("a write that shrinks an object over budget was refused: %v", err)
	}
	if _, err := tight.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: budgeted("cwm.o", strings.Repeat("漢", 95))}); !errors.Is(err, api.ErrBudgetExceeded) {
		t.Errorf("a write that grows an object over budget was accepted: %v", err)
	}
}

// Refused in warn mode too, like the description budget: warn is for a type
// nobody declared, and this type declares its cap. Off checks nothing.
func TestSizeBudgetRefusedInWarnModeNotWhenOff(t *testing.T) {
	for mode, refused := range map[string]bool{api.TypeValidationWarn: true, api.TypeValidationOff: false} {
		svc := svcWithTypes(t, mode, []api.ObjectType{{Name: "budgeted", MaxSizeBytes: 100}})
		_, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: budgeted("cwm.big", strings.Repeat("é", 200))})
		if errors.Is(err, api.ErrBudgetExceeded) != refused {
			t.Errorf("mode=%q: err = %v, want refused=%v", mode, err, refused)
		}
	}
}

// reopen is a second service over the same workspace with other types — a
// type tightened between two runs.
func reopen(t *testing.T, svc *Service, types []api.ObjectType) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: svc.Config.WorkspaceRoot, ProjectRoot: svc.Config.ProjectRoot, ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = types
	_ = svc.Close()
	out, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func hasDiagnosticLabel(ds []api.ObjectDiagnostic, part string) bool {
	for _, d := range ds {
		if strings.Contains(d.Label, part) {
			return true
		}
	}
	return false
}
