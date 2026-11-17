// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

func svcWithTypes(t *testing.T, mode string, types []api.ObjectType) *Service {
	t.Helper()
	root := freshWorkspace(t)
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ObjectTypeValidation: mode}
	cfg.CatalogTypes.Objects = types
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

var declared = []api.ObjectType{
	{Name: "use-case", Capabilities: []string{"container", "content", "checkable"}},
	{Name: "abstract-thing", Abstract: true},
}

// THE SAFETY RULE, and the most important test here: with NO declarations,
// validation is off in every mode. A workspace that has not migrated its type
// declarations must behave exactly as before — refusing every write the moment
// the feature ships is how a safety check gets permanently disabled.
func TestNoDeclarations_ValidationIsOff(t *testing.T) {
	for _, mode := range []string{"", api.TypeValidationWarn, api.TypeValidationError} {
		svc := svcWithTypes(t, mode, nil) // none declared
		o := obj("cwm.anything", "a-type-nobody-declared", "")
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
			t.Errorf("mode=%q: create refused with no declarations: %v", mode, err)
		}
	}
}

// warn accepts an undeclared type (and logs it — visible, not silent).
func TestWarnMode_AcceptsUndeclaredType(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationWarn, declared)
	o := obj("cwm.x", "not-declared", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Errorf("warn mode must ACCEPT an undeclared type, got %v", err)
	}
}

// error refuses it — the terminal state after migration (D5).
func TestErrorMode_RefusesUndeclaredType(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, declared)
	o := obj("cwm.x", "not-declared", "")
	_, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("error mode must REFUSE an undeclared type, got %v", err)
	}
}

// A declared type passes in the strictest mode — otherwise the check would be
// passing the earlier tests by refusing everything.
func TestErrorMode_AcceptsDeclaredType(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, declared)
	o := obj("cwm.ok", "use-case", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Errorf("a DECLARED type must be accepted in error mode, got %v", err)
	}
}

// Abstract is refused in BOTH modes: not a migration gap but an explicit
// statement that the type may not be instantiated.
func TestAbstractTypeRefusedInBothModes(t *testing.T) {
	for _, mode := range []string{api.TypeValidationWarn, api.TypeValidationError} {
		svc := svcWithTypes(t, mode, declared)
		o := obj("cwm.a", "abstract-thing", "")
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err == nil {
			t.Errorf("mode=%q: an abstract type must be refused", mode)
		}
	}
}

// An unrecognised mode — and an empty one — is ERROR: CWM guarantees a stored
// object is valid, and a typo in config must not relax that.
func TestUnrecognisedModeIsError(t *testing.T) {
	for _, mode := range []string{"yes-please", ""} {
		svc := svcWithTypes(t, mode, declared)
		if got := svc.typeValidationMode(); got != api.TypeValidationError {
			t.Errorf("mode %q = %q, want error — only an explicit warn or off relaxes validation", mode, got)
		}
	}
}

// Update validates too: an unvalidated update is a hole straight through a
// validated create.
func TestUpdateValidatesType(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, declared)
	ctx := stdctx.Background()
	o := obj("cwm.u", "use-case", "")
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("seed create: %v", err)
	}
	o.Type = "smuggled-in-by-update"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "cwm", Object: o}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("update must validate the type, got %v", err)
	}
}

// An empty type is wrong in every enforcing mode — nothing can resolve a bundle
// from it.
func TestEmptyTypeRefusedInErrorMode(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, declared)
	o := obj("cwm.e", "", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err == nil {
		t.Error("an object with no type must be refused in error mode")
	}
}

// A hot-deployed pack's object-types become available without a restart
// (RefreshCatalogTypes).
func TestRefreshCatalogTypes_AddsObjectTypes(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, declared)
	ctx := stdctx.Background()

	o := obj("cwm.later", "arrived-later", "")
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err == nil {
		t.Fatal("undeclared type accepted in error mode before refresh")
	}

	svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{{Name: "arrived-later"}}})

	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("type declared by refresh still refused: %v", err)
	}
}

// A refresh REPLACES a changed declaration: the type redeclared abstract is
// in force at once, without a restart.
func TestRefreshCatalogTypes_ReplacesAChangedType(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, declared)

	svc.RefreshCatalogTypes(CatalogTypeSet{Objects: []api.ObjectType{{Name: "use-case", Abstract: true}}})

	o := obj("cwm.uc-after-refresh", "use-case", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err == nil {
		t.Fatal("a type redeclared abstract is still instantiable after the refresh")
	}
}

// A nameless declaration never enters the set. It cannot match any object's
// type, but a NON-EMPTY set is what switches validation on — so admitting it
// would enable validation against nothing.
func TestNamelessDeclarationIsDropped(t *testing.T) {
	svc := svcWithTypes(t, api.TypeValidationError, []api.ObjectType{{Capabilities: []string{"container"}}})
	o := obj("cwm.anything", "a-type-nobody-declared", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Errorf("nameless declaration switched validation on: %v", err)
	}
}

// The description budget, ported from NodeCreate. Declared by 15 shipped types
// and enforced nowhere on the object path until now — a cap that is declared
// and unenforced is the same as no cap, except it reads as protection.
func TestDescriptionBudgetEnforced(t *testing.T) {
	types := []api.ObjectType{{Name: "capped", DescriptionMaxChars: 10}}
	svc := svcWithTypes(t, api.TypeValidationWarn, types)
	ctx := stdctx.Background()

	within := obj("cwm.short", "capped", "")
	within.Description = "0123456789" // exactly at the cap
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: within}); err != nil {
		t.Fatalf("description exactly at the cap was refused: %v", err)
	}

	over := obj("cwm.long", "capped", "")
	over.Description = "01234567890" // one over
	_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: over})
	if !errors.Is(err, api.ErrBudgetExceeded) {
		t.Fatalf("over-budget description accepted (or wrong error): %v", err)
	}
}

// Refused in WARN mode too. Warn exists for the migration gap — a type nobody
// declared yet — and a budget overrun is not that: the type is declared, the cap
// is declared, and the value breaks it.
func TestDescriptionBudgetRefusedInWarnMode(t *testing.T) {
	types := []api.ObjectType{{Name: "capped", DescriptionMaxChars: 5}}
	for _, mode := range []string{api.TypeValidationWarn, api.TypeValidationError} {
		svc := svcWithTypes(t, mode, types)
		o := obj("cwm.long", "capped", "")
		o.Description = "way too long"
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); !errors.Is(err, api.ErrBudgetExceeded) {
			t.Errorf("mode=%q: over-budget accepted: %v", mode, err)
		}
	}
}

// Counted in RUNES. The node path counted bytes, so an accented description was
// cut short of its own stated cap — the budget is about how much a reader has to
// read, and "é" is one character to them.
func TestDescriptionBudgetCountsRunesNotBytes(t *testing.T) {
	types := []api.ObjectType{{Name: "capped", DescriptionMaxChars: 5}}
	svc := svcWithTypes(t, api.TypeValidationWarn, types)
	o := obj("cwm.accents", "capped", "")
	o.Description = "éèêëà" // 5 runes, 10 bytes
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("5 accented chars refused against a 5-char cap (byte counting): %v", err)
	}
}

// A type with no cap declared is unrestricted — 0 means "use the default",
// never "allow nothing".
func TestNoBudgetDeclaredMeansUnrestricted(t *testing.T) {
	types := []api.ObjectType{{Name: "uncapped"}}
	svc := svcWithTypes(t, api.TypeValidationWarn, types)
	o := obj("cwm.essay", "uncapped", "")
	o.Description = strings.Repeat("x", 5000)
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("uncapped type refused a long description: %v", err)
	}
}

// --- containment (gate 2) -----------------------------------------

// The four containment cases, because the rule is a UNION and only one of the
// four refuses. Getting the direction wrong would either freeze a workspace
// that declares nothing, or wave through the one case that should be caught.
func TestParentChildContainment(t *testing.T) {
	types := []api.ObjectType{
		{Name: "folder", AllowedChildren: []string{"folder", "doc"}},
		{Name: "doc"},
		{Name: "stranger"}, // declares nothing either way
		{Name: "picky", AcceptParents: []string{"folder"}},   // child-side restriction
		{Name: "strict", AcceptParents: []string{"nowhere"}}, // child-side: under nothing that exists
		{Name: "anywhere", AcceptParents: []string{"*"}},     // explicit wildcard
		{Name: "open"}, // parent with no restriction
	}
	ctx := stdctx.Background()

	mk := func(t *testing.T, svc *Service, id, typ, parent string) error {
		t.Helper()
		o := obj(id, typ, "")
		o.Parent = parent
		_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o})
		return err
	}
	// seed makes a parent object of the given type to hang children from.
	seed := func(t *testing.T, svc *Service, id, typ string) {
		t.Helper()
		o := obj(id, typ, "")
		// (nothing to set)
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	t.Run("parent lists the child: allowed", func(t *testing.T) {
		svc := svcWithTypes(t, api.TypeValidationWarn, types)
		seed(t, svc, "cwm.f", "folder")
		if err := mk(t, svc, "cwm.d", "doc", "cwm.f"); err != nil {
			t.Errorf("declared child refused: %v", err)
		}
	})

	// BOTH sides must reject before a write is refused. `stranger` is not in
	// folder's allowed_children, but it declares no accept_parents — so the
	// child side does not reject, and the union allows it.
	t.Run("child declares nothing: allowed despite parent's list", func(t *testing.T) {
		svc := svcWithTypes(t, api.TypeValidationWarn, types)
		seed(t, svc, "cwm.f", "folder")
		if err := mk(t, svc, "cwm.s", "stranger", "cwm.f"); err != nil {
			t.Errorf("union rule broken — an unrestricted child was refused: %v", err)
		}
	})

	// The ONE refusing case: parent's list excludes the child AND the child's
	// list excludes the parent.
	// The ONLY refusing combination, and it needs BOTH halves: `folder` lists
	// [folder, doc] so it excludes `strict`, AND `strict` accepts only
	// "nowhere" so it excludes `folder`.
	//
	// The first version of this test used a parent with NO allowed_children and
	// expected a refusal. It passed the child-side check and was correctly
	// allowed — the test was wrong about the direction of the union, not the
	// code. The note stays: an empty list on either side means "no restriction
	// from here", never "nothing allowed".
	t.Run("both sides reject: refused", func(t *testing.T) {
		svc := svcWithTypes(t, api.TypeValidationWarn, types)
		seed(t, svc, "cwm.f", "folder")
		seed(t, svc, "cwm.o", "open")

		err := mk(t, svc, "cwm.sx", "strict", "cwm.f")
		if err == nil {
			t.Fatal("incompatible parent/child accepted")
		}
		// The message must name BOTH lists: with only one, fixing the
		// declaration is guesswork.
		for _, want := range []string{"allowed_children", "accept_parents", "strict", "folder"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal message missing %q: %v", want, err)
			}
		}
		// A parent with no list at all cannot reject, so the same child is
		// allowed under `open` — the half that makes the union permissive.
		if err := mk(t, svc, "cwm.so", "strict", "cwm.o"); err != nil {
			t.Errorf("unrestricted parent refused a child: %v", err)
		}
	})

	t.Run("wildcard accept_parents: allowed anywhere", func(t *testing.T) {
		svc := svcWithTypes(t, api.TypeValidationWarn, types)
		seed(t, svc, "cwm.f", "folder")
		if err := mk(t, svc, "cwm.a", "anywhere", "cwm.f"); err != nil {
			t.Errorf("wildcard child refused: %v", err)
		}
	})
}

// A root has no parent, so containment cannot apply. Refusing here would make
// every top-level object unwritable.
func TestRootObjectSkipsContainment(t *testing.T) {
	types := []api.ObjectType{{Name: "picky", AcceptParents: []string{"nothing-that-exists"}}}
	svc := svcWithTypes(t, api.TypeValidationWarn, types)
	o := obj("cwm.root", "picky", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("root object refused by containment: %v", err)
	}
}

// An UNDECLARED type imposes nothing and suffers nothing. A missing declaration
// is a migration gap, never a denial — otherwise switching the feature on would
// refuse every write in an unmigrated workspace.
func TestUndeclaredTypesImposeNoContainment(t *testing.T) {
	types := []api.ObjectType{{Name: "folder", AllowedChildren: []string{"doc"}}}
	svc := svcWithTypes(t, api.TypeValidationWarn, types)
	ctx := stdctx.Background()
	p := obj("cwm.f", "folder", "")
	// (nothing to set)
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: p}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := obj("cwm.u", "never-declared", "")
	c.Parent = "cwm.f"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: c}); err != nil {
		t.Errorf("undeclared child type refused by containment: %v", err)
	}
}

// A parent that cannot be resolved is NOT a containment failure. It is a
// dangling reference — a different defect, with a different report. Attributing
// it here would send a reader to the wrong declaration.
func TestUnresolvableParentIsNotAContainmentError(t *testing.T) {
	types := []api.ObjectType{{Name: "picky", AcceptParents: []string{"folder"}}}
	svc := svcWithTypes(t, api.TypeValidationWarn, types)
	o := obj("cwm.orphan", "picky", "")
	o.Parent = "cwm.does-not-exist"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Errorf("dangling parent reported as a containment failure: %v", err)
	}
}
