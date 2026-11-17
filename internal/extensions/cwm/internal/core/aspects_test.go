// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ASPECTS (7g): a named list of capabilities added in one move. The guard:
// `aspects: [isa]` is effective-equal to the list written out.

func aspectsFixture(t *testing.T, mode string, aspects []api.Aspect) *Service {
	t.Helper()
	root := freshWorkspace(t)
	project := t.TempDir()
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project, ObjectTypeValidation: mode}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "note", Capabilities: []string{"container", "content"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "content"}, {Name: "want"},
		{Name: "checkable", Hooks: api.Hooks{api.HookVerify: {{URI: "x:acceptance/verify"}}}},
	}
	cfg.CatalogTypes.Aspects = aspects
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

var isa = api.Aspect{Name: "isa", Capabilities: []string{"want", "checkable"}}

func aspectIDsOf(t *testing.T, svc *Service, id string) []string {
	t.Helper()
	out, err := svc.ObjectEffectiveCapabilities(stdctx.Background(), api.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		t.Fatalf("ObjectEffectiveCapabilities %s: %v", id, err)
	}
	var ids []string
	for _, a := range out.Capabilities {
		ids = append(ids, a.ID)
	}
	return ids
}

// THE GUARD: with the aspect ≡ with the list written out — same effective set,
// same runnable aspects, same drawing.
func TestAspectIsEffectiveEqualToItsListWrittenOut(t *testing.T) {
	svc := aspectsFixture(t, api.TypeValidationError, []api.Aspect{isa})
	ctx := stdctx.Background()
	withAspect := api.Object{ID: "m.a", Type: "note", Aspects: []string{"isa"}}
	withAspect.Name = "a"
	written := api.Object{ID: "m.b", Type: "note"}
	written.Name = "b"
	_ = written.SetCapability("want", map[string]any{})
	_ = written.SetCapability("checkable", map[string]any{})
	for _, o := range []api.Object{withAspect, written} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	a, b := aspectIDsOf(t, svc, "m.a"), aspectIDsOf(t, svc, "m.b")
	sort.Strings(a) // the effective SET is what is equal; order follows the source (aspect order vs sorted extras)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("aspects: [isa] gives %v; the list written out gives %v — they must be the same set", a, b)
	}
	if !strings.Contains(strings.Join(a, ","), "checkable") {
		t.Errorf("isa did not bring checkable: %v", a)
	}
	got, _ := svc.ObjectList(ctx, api.ObjectListInput{Capability: "want"})
	if len(got.Objects) != 2 {
		t.Errorf("ObjectList{Capability: want} = %v, want both", ids(got.Objects))
	}
}

// AN UNDECLARED ASPECT is refused in error mode, logged in warn, off with none
// declared — the instance-group rule, applied to aspects.
func TestUndeclaredAspectIsRefused(t *testing.T) {
	o := api.Object{ID: "m.x", Type: "note", Aspects: []string{"nobody-declares-this"}}
	o.Name = "x"
	strict := aspectsFixture(t, api.TypeValidationError, []api.Aspect{isa})
	if _, err := strict.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "nobody-declares-this") {
		t.Errorf("error mode: %v — must refuse naming the aspect", err)
	}
	warn := aspectsFixture(t, api.TypeValidationWarn, []api.Aspect{isa})
	if _, err := warn.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Errorf("warn mode must accept: %v", err)
	}
	none := aspectsFixture(t, api.TypeValidationError, nil)
	if _, err := none.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Errorf("with no aspects declared the check is off: %v", err)
	}
}

// A ASPECT NAMING AN UNDECLARED CAPABILITY IS DROPPED AT LOAD — it never
// expands to a group nobody can read.
func TestAspectNamingUndeclaredCapabilityIsDroppedAtLoad(t *testing.T) {
	bad := api.Aspect{Name: "ghost", Capabilities: []string{"want", "nobody-declares-this"}}
	svc := aspectsFixture(t, api.TypeValidationWarn, []api.Aspect{isa, bad})
	if _, ok := svc.lookupAspect("ghost"); ok {
		t.Fatal("an aspect naming an undeclared capability survived the load")
	}
	if _, ok := svc.lookupAspect("isa"); !ok {
		t.Fatal("the good aspect was dropped with the bad one")
	}
}

// A GROUP THE INSTANCE WRITES THAT A ASPECT ALSO PROVIDES IS ONE GROUP: the
// object's field-group is the data; the aspect only says it is in force.
func TestAspectAndInstanceGroupCollapseToOne(t *testing.T) {
	svc := aspectsFixture(t, api.TypeValidationError, []api.Aspect{isa})
	o := api.Object{ID: "m.c", Type: "note", Aspects: []string{"isa"}}
	o.Name = "c"
	_ = o.SetCapability("want", map[string]any{"reachability": "asymptotic"})
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	a := aspectIDsOf(t, svc, "m.c")
	seen := map[string]int{}
	for _, id := range a {
		seen[id]++
	}
	if seen["want"] != 1 {
		t.Errorf("want appears %d times: %v", seen["want"], a)
	}
	got, _ := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "m", ID: "m.c"})
	if g, _ := got.Object.Capability("want"); g["reachability"] != "asymptotic" {
		t.Errorf("the instance's want group was not the data: %v", g)
	}
}
