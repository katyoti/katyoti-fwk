// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
	"github.com/katyoti/katyoti-fwk/internal/test/schematest"
)

// realCorpusConfig assembles what a running universe gives CWM for THIS
// repository: every pack's object types (with the legacy derivation),
// capabilities, workflows and edge kinds, and each capability's schema compiled
// from its data-type — first declaration wins, as in the catalog.
func realCorpusConfig(t *testing.T) (*CWMServiceConfig, string) {
	t.Helper()
	return realCorpusConfigOf(t, nil)
}

// realCorpusConfigOf is realCorpusConfig without the packs named in skip: their
// pack folder and the data-types their sidecar publishes beside it are left
// out, as if the repository did not ship them (the public shape,
// data_shape_test.go).
func realCorpusConfigOf(t *testing.T, skip []string) (*CWMServiceConfig, string) {
	t.Helper()
	up := filepath.Join("..", "..", "..", "..", "..")
	if _, err := os.Stat(filepath.Join(up, api.ProjectFileName)); err != nil {
		t.Skipf("no project object at the repo root: %v", err)
	}
	var packs []string
	for _, p := range shippedPacks(up) {
		if !slices.Contains(skip, p.name) {
			packs = append(packs, p.dir)
		}
	}
	if len(packs) < 6 {
		t.Fatalf("found only %d type packs — the glob is wrong, not the corpus", len(packs))
	}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: up, ObjectTypeValidation: api.TypeValidationError}
	seenType, seenCap, seenWf, seenAspect := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	seenEdge := map[api.EdgeType]bool{}
	dsl := map[string]string{} // data-type name → its DSL file
	// Explicit object types first, from every pack: in the catalog an explicit
	// declaration shadows a type derived from a legacy node / item type of the
	// same name (`goal` is both), whatever pack declares which.
	for _, dir := range packs {
		ots, err := storage.LoadObjectTypesLayered(os.DirFS(dir))
		if err != nil {
			t.Fatalf("load object types %s: %v", dir, err)
		}
		for _, ot := range ots {
			if !seenType[ot.Name] {
				seenType[ot.Name] = true
				cfg.CatalogTypes.Objects = append(cfg.CatalogTypes.Objects, ot)
			}
		}
	}
	for _, dir := range packs {
		fsys := os.DirFS(dir)
		reg, err := storage.LoadTypesLayered(fsys)
		if err != nil {
			t.Fatalf("load types %s: %v", dir, err)
		}
		caps, err := storage.LoadCapabilitiesLayered(fsys)
		if err != nil {
			t.Fatalf("load capabilities %s: %v", dir, err)
		}
		for _, c := range caps {
			if !seenCap[c.Name] {
				seenCap[c.Name] = true
				cfg.CatalogTypes.Capabilities = append(cfg.CatalogTypes.Capabilities, c)
			}
		}
		// The aspects too: an object naming an aspect no pack declares is
		// refused, so the corpus is judged with the packs' aspects in force.
		aspects, err := storage.LoadAspectsLayered(fsys)
		if err != nil {
			t.Fatalf("load aspects %s: %v", dir, err)
		}
		for _, a := range aspects {
			if !seenAspect[a.Name] {
				seenAspect[a.Name] = true
				cfg.CatalogTypes.Aspects = append(cfg.CatalogTypes.Aspects, a)
			}
		}
		wfs, err := storage.LoadWorkflowsLayered(fsys)
		if err != nil {
			t.Fatalf("load workflows %s: %v", dir, err)
		}
		for _, w := range wfs {
			if !seenWf[w.Name] {
				seenWf[w.Name] = true
				cfg.CatalogTypes.Workflows = append(cfg.CatalogTypes.Workflows, w)
			}
		}
		for name, e := range reg.EdgeTypes() {
			if !seenEdge[name] {
				seenEdge[name] = true
				cfg.CatalogTypes.Edges = append(cfg.CatalogTypes.Edges, e)
			}
		}
		files, _ := filepath.Glob(filepath.Join(dir, "data-types", "*", "*.dsl.yaml"))
		top, _ := filepath.Glob(filepath.Join(dir, "data-types", "*.dsl.yaml"))
		for _, f := range append(files, top...) {
			name := strings.TrimSuffix(filepath.Base(f), ".dsl.yaml")
			if _, ok := dsl[name]; !ok {
				dsl[name] = f
			}
		}
	}
	// Data-types published beside a pack rather than in it (a sidecar's own
	// `embed/data-types`, the system kinds), after the packs': first wins.
	side, _ := filepath.Glob(filepath.Join(up, "app", "*", "embed", "data-types", "*.dsl.yaml"))
	for _, f := range side {
		if sidecar := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(f)))); slices.Contains(skip, sidecar) {
			continue
		}
		if name := strings.TrimSuffix(filepath.Base(f), ".dsl.yaml"); dsl[name] == "" {
			dsl[name] = f
		}
	}
	system, _ := filepath.Glob(filepath.Join(up, "internal", "core", "contribution", "embed", "data-type", "*.yaml"))
	for _, f := range system {
		if name := strings.TrimSuffix(filepath.Base(f), ".yaml"); dsl[name] == "" {
			dsl[name] = f
		}
	}
	// Data-types a binary renders from Go rather than ships as a file: the
	// schemas of the pack-owned leaf kinds (builtin.ContributeLeafKinds
	// publishes each at `<provider>:/data-type/<id>`), after the files.
	generated := map[string]leaf.Declaration{}
	for _, l := range builtin.Leaves() {
		if dsl[l.Schema.ID] == "" {
			generated[l.Schema.ID] = l
		}
	}
	cfg.CatalogTypes.CapabilitySchemas = map[string]json.RawMessage{}
	compile := func(owner, uri string) {
		if uri == "" || cfg.CatalogTypes.CapabilitySchemas[uri] != nil {
			return
		}
		name := uri[strings.LastIndex(uri, "/")+1:]
		var src []byte
		var err error
		if f, ok := dsl[name]; ok {
			src, err = os.ReadFile(f)
		} else if l, ok := generated[name]; ok {
			src, err = l.Schema.Document()
		} else {
			t.Errorf("%s: schema %s has no data-type file in any pack", owner, uri)
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		compiled, err := schematest.CompileJSONSchema(stdctx.Background(), src)
		if err != nil {
			t.Errorf("%s: schema %s does not compile: %v", owner, uri, err)
			return
		}
		cfg.CatalogTypes.CapabilitySchemas[uri] = compiled
	}
	for _, c := range cfg.CatalogTypes.Capabilities {
		compile("capability "+c.Name, c.Schema)
		compile("capability "+c.Name+" (type_schema)", c.TypeSchema)
	}
	// A type's own fields are checked like a capability's group.
	for _, ot := range cfg.CatalogTypes.Objects {
		compile("object type "+ot.Name, ot.Fields)
	}
	return cfg, up
}

// TestRealCorpusIsValid: every object this repository stores is valid as CWM
// defines it — its type, its capability groups against their schemas, its
// edges, its lifecycle — checked by the validating scan in error mode. The
// corpus proves the rule breaks nothing that is right, and names what is not.
func TestRealCorpusIsValid(t *testing.T) {
	long.Test(t)
	t.Parallel() // waits mostly on the disk, beside the other corpus tests
	cfg, up := realCorpusConfig(t)
	// Judged at the code's data version: on a migrated copy when the
	// project's data is behind it (data_shape_test.go), never written.
	cfg.ProjectRoot = migratedCorpus(t, up, migrationSteps)
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	out, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{Force: true, Validate: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Checked < 100 {
		t.Fatalf("only %d objects checked — a vacuous pass is worse than a failure", out.Checked)
	}
	for _, inv := range out.Invalid {
		problems := inv.Problems
		if cfg.ProjectRoot != up {
			problems = shapeProblems(problems) // a migrated copy holds no code
		}
		if len(problems) > 0 {
			t.Errorf("%s (%s): %s", inv.ID, inv.Path, strings.Join(problems, " || "))
		}
	}
	t.Logf("corpus: %d objects checked, %d invalid, %d capability schemas", out.Checked, len(out.Invalid), len(cfg.CatalogTypes.CapabilitySchemas))
}

// TestEveryShippedTypeSectionChecks: every section a shipped type writes
// (`context:`, `pane:`, `verification:`) names a capability a pack declares
// with a type_schema, and conforms to it — else CWM refuses the type in error
// mode, and every object of it with it.
func TestEveryShippedTypeSectionChecks(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	withSections := 0
	for _, ot := range cfg.CatalogTypes.Objects {
		if len(ot.Sections) == 0 {
			continue
		}
		withSections++
		for _, p := range svc.sectionProblems(ot, cfg.CatalogTypes.Capabilities) {
			t.Errorf("type %s: %s", ot.Name, p)
		}
		if _, ok := svc.lookupObjectType(ot.Name); !ok {
			t.Errorf("type %s is not registered", ot.Name)
		}
	}
	// A floor: with the sections not captured this would pass on nothing. The
	// packs ship 17 types that write one.
	if withSections < 15 {
		t.Fatalf("only %d shipped types write a section — the capture is broken, not the packs", withSections)
	}
}
