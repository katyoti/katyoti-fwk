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

// THE TYPE'S OWN FIELDS. What a kind of thing is, beyond the core every object
// has, lives under `fields:` — shaped by the type's `fields:` data-type and
// checked on every write like a capability's group. The body is prose only.

// TestTheTypesFieldsAreCheckedAgainstItsDataType: a type declaring `fields:`
// refuses an object whose fields do not conform, and accepts and reads back
// one that does.
func TestTheTypesFieldsAreCheckedAgainstItsDataType(t *testing.T) {
	compiled, err := schematest.CompileJSONSchema(stdctx.Background(), []byte(estimateDSL))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "estimate", Label: "Estimate", Fields: "x:/data-type/cap-estimate", Capabilities: []string{"sizing", "stamp"}}}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "sizing", Label: "Sizing", Description: "How big it is", Icon: "ruler", Schema: "x:/data-type/cap-estimate"},
		{Name: "stamp", Schema: "x:/data-type/cap-estimate"},
	}
	cfg.CatalogTypes.CapabilitySchemas = map[string]json.RawMessage{"x:/data-type/cap-estimate": compiled}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	bad := api.Object{ID: "m.e1", Type: "estimate", Name: "e1", Fields: map[string]any{"points": "three"}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: bad}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "/points") {
		t.Fatalf("a wrong field: err = %v, want refused naming /points", err)
	}
	good := api.Object{ID: "m.e2", Type: "estimate", Name: "e2", Body: "Why three.", Fields: map[string]any{"points": 3, "unit": "days"},
		Capabilities: map[string]map[string]any{"sizing": {"points": 5}}}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: good}); err != nil {
		t.Fatalf("valid fields refused: %v", err)
	}

	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.e2"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.Object.Body) != "Why three." || got.Object.Fields["unit"] != "days" {
		t.Errorf("read back: body %q fields %v", got.Object.Body, got.Object.Fields)
	}
}

// TestAStructuredBodyIsNoLongerRead: the old shape — data under `body:` — does
// not read; its data is the type's fields.
func TestAStructuredBodyIsNoLongerRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x"+ObjectFileSuffix)
	if err := os.WriteFile(p, []byte("id: m.x\ntype: note\nname: X\nbody:\n  amount: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadObjectFile(p); err == nil {
		t.Error("a structured body was read; its data belongs under fields:")
	}
}
