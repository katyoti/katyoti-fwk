// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/santhosh-tekuri/jsonschema/v5"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The `contributions` capability is attached to every project object: a
// project with its settings (a provider token, a folder) is written, and the
// capability is in force on it — its type need not name it. The field-group's
// schema is held by internal/test/cwmapp (TestContributionsIsHeldByItsSchema).
func TestAProjectCarriesItsContributionsSettings(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		cwmapp.Pack(fstest.MapFS{"object-types/project.yaml": {Data: []byte("name: project\ncapabilities: [project]\n")}}),
		Pack(""),
	)
	p := cwmapi.Object{ID: "acme", Type: cwmapi.CapProject, Name: "acme",
		Capabilities: map[string]map[string]any{"contributions": {"provider": "acme", "root": "packs/acme"}}}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "acme", Object: p})
	if err != nil {
		t.Fatalf("a project with its contributions settings is refused: %v", err)
	}
	_ = created
	// A project that says nothing of its contributions carries the capability
	// all the same (its defaults apply): attached, not declared by the type.
	bare := cwmapi.Object{ID: "bare", Type: cwmapi.CapProject, Name: "bare"}
	plain, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "bare", Object: bare})
	if err != nil {
		t.Fatal(err)
	}
	surf, err := cwm.ObjectEffectiveSurface(ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: plain.Object.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(surf.Capabilities, "contributions") {
		t.Fatalf("capabilities in force on a project = %v, want contributions (attached to every project)", surf.Capabilities)
	}
}

// sys-contributions is never created for a project: a universe with the
// system modules and the contributions capability boots, and a project is
// written, with no system module made. Its first authoring verb creates it;
// absent is normal.
func TestSysContributionsIsNeverAutoCreated(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		cwmapp.SystemModules(t),
		cwmapp.Pack(fstest.MapFS{"object-types/project.yaml": {Data: []byte("name: project\ncapabilities: [project]\n")}}),
		Pack(""),
	)
	p := cwmapi.Object{ID: "acme", Type: cwmapi.CapProject, Name: "acme"}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "acme", Object: p}); err != nil {
		t.Fatal(err)
	}
	sys, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapSystemModule})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range sys.Objects {
		if o.ID == "sys-contributions" {
			t.Fatal("sys-contributions was created without an authoring verb")
		}
	}
}

// Every project offers Start contributing: the contributions capability,
// attached to the project type, declares the action that gives the project
// its sys-contributions module - Compose's ensure method, never an init of
// this package's own.
func TestAProjectOffersStartContributing(t *testing.T) {
	raw, err := packFS.ReadFile("embed/cwm/capabilities/contributions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var c cwmapi.Capability
	if err := config.DecodeBytes(raw, marshal.YAML, &c); err != nil {
		t.Fatal(err)
	}
	for _, a := range c.Actions {
		if a.URI == "ktw-agents:project-contributions/init" {
			t.Fatalf("the interim init action is still declared: %+v", a)
		}
	}
	for _, a := range c.Actions {
		if a.URI == "ktw-compose:compose/ensure-contributions" && a.Label != "" {
			return
		}
	}
	t.Fatalf("actions = %+v; want Start contributing", c.Actions)
}

// Any object may carry `contributes`: an object of a type that does not name
// the capability publishes itself as a deck theme.
func TestAnyObjectMayCarryContributes(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		cwmapp.Pack(fstest.MapFS{"object-types/deck-theme.yaml": {Data: []byte("name: deck-theme\n")}}),
		Pack(""),
	)
	o := cwmapi.Object{ID: "acme-blue", Type: "deck-theme", Name: "Acme blue", Body: "# Acme blue",
		Capabilities: map[string]map[string]any{"contributes": {"kind": "deck-theme", "layers": map[string]any{"source": "body"}}}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "themes", Object: o}); err != nil {
		t.Fatalf("an object publishing itself as a deck theme is refused: %v", err)
	}
}

// The `contributes` field-group is held by its schema: a kind and layers
// named by source pass; no kind, or a layer source that is not text, is
// refused.
func TestContributesIsHeldByItsSchema(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("embed", "cwm", "data-types", "cap-contributes.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := schemaparser.CompileJSONSchema(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("cap-contributes", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("cap-contributes")
	if err != nil {
		t.Fatal(err)
	}
	ok := map[string]any{"kind": "persona", "overrides": "ktw-agents:/persona/reviewer", "layers": map[string]any{"i18n.fr": "body"}}
	if err := sch.Validate(ok); err != nil {
		t.Errorf("an override declaration is refused: %v", err)
	}
	for name, v := range map[string]map[string]any{
		"no kind":          {"layers": map[string]any{"source": "body"}},
		"a numeric source": {"kind": "persona", "layers": map[string]any{"source": 3}},
	} {
		if err := sch.Validate(v); err == nil {
			t.Errorf("%s is accepted: the schema does not hold the field-group", name)
		}
	}
}
