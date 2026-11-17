// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/schematest"
)

// CWM GUARANTEES A STORED OBJECT IS VALID: a capability's group is checked
// against its schema on every write, and the scan reports a stored object
// that is not valid.

const estimateDSL = `$schema: ktw-schema/v1
name: cap-estimate
version: "1.0.0"
root: CapEstimate
types:
  CapEstimate:
    type: object
    fields:
      points:
        type: number
      unit:
        type: string@enum(days, points)
        optional: true
`

func validationFixture(t *testing.T, mode string) *Service {
	t.Helper()
	compiled, err := schematest.CompileJSONSchema(stdctx.Background(), []byte(estimateDSL))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: mode}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		{Name: "estimate", Schema: "x:/data-type/cap-estimate"},
		{Name: "unresolved", Schema: "x:/data-type/nowhere"},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "task", Capabilities: []string{"container", "estimate", "unresolved"}}}
	cfg.CatalogTypes.CapabilitySchemas = map[string]json.RawMessage{"x:/data-type/cap-estimate": compiled}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func taskWith(id string, estimate map[string]any) api.Object {
	o := api.Object{ID: id, Type: "task"}
	o.Name = id
	if estimate != nil {
		_ = o.SetCapability("estimate", estimate)
	}
	return o
}

func TestAnInvalidGroupIsRefusedWithTheReason(t *testing.T) {
	svc := validationFixture(t, api.TypeValidationError)
	ctx := stdctx.Background()
	_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: taskWith("m.t1", map[string]any{"points": "three"})})
	if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), `capability "estimate"`) || !strings.Contains(err.Error(), "/points") {
		t.Fatalf("a wrong field type: err = %v, want refused naming estimate /points", err)
	}
	_, err = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: taskWith("m.t2", map[string]any{"unit": "days"})})
	if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "points") {
		t.Fatalf("a missing required field: err = %v, want refused naming points", err)
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: taskWith("m.t3", map[string]any{"points": 3, "unit": "days"})}); err != nil {
		t.Fatalf("a valid group was refused: %v", err)
	}
	// An update that breaks a valid object is refused too.
	bad := taskWith("m.t3", map[string]any{"points": 3, "unit": "weeks"})
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: bad}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("an update out of the enum: err = %v, want refused", err)
	}
	// A group whose schema is not resolved cannot be checked: never refused for it.
	o := taskWith("m.t4", nil)
	_ = o.SetCapability("unresolved", map[string]any{"anything": true})
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("an unresolved schema refused the write: %v", err)
	}
}

func TestWarnModeLogsAndAccepts(t *testing.T) {
	svc := validationFixture(t, api.TypeValidationWarn)
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: taskWith("m.t1", map[string]any{"points": "three"})}); err != nil {
		t.Fatalf("warn mode refused: %v", err)
	}
}

// TestTheScanReportsAStoredObjectThatIsNotValid: an object edited on disk into
// an invalid state is found by the validating scan, with the reason.
func TestTheScanReportsAStoredObjectThatIsNotValid(t *testing.T) {
	svc := validationFixture(t, api.TypeValidationError)
	ctx := stdctx.Background()
	out, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: taskWith("m.t1", map[string]any{"points": 3})})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(svc.Config.ObjectsRoot(), filepath.FromSlash(out.Path))
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(svc.Config.ProjectRoot, filepath.FromSlash(out.Path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the object file: %v", err)
	}
	edited := strings.Replace(string(raw), "points: 3", "points: three", 1)
	if edited == string(raw) {
		t.Fatalf("the fixture did not write `points: 3`:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	scan, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true, Validate: true})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Checked != 1 || len(scan.Invalid) != 1 || scan.Invalid[0].ID != "m.t1" || !strings.Contains(strings.Join(scan.Invalid[0].Problems, " "), "/points") {
		t.Fatalf("scan = %+v, want m.t1 reported for /points", scan)
	}
}
