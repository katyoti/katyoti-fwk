// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// shippedModulePack is the `software-module` type (ktw-code) and the module capability as shipped:
// a test application's catalog does not carry the data-type's DSL, so
// CWM leaves the group unchecked here; TestCodePackagesIsHeldByTheCapCodeSchema
// checks the field against the shipped schema, compiled as CWM compiles it.
func shippedModulePack(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"object-types/software-module.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/software-module.yaml")},
		"capabilities/planned.yaml":         {Data: []byte("name: planned\n")},
		"capabilities/module.yaml":          {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/capabilities/module.yaml")},
		"capabilities/code.yaml":            {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"workflows/manual.yaml":             {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/manual.yaml")},
		"capabilities/lifecycle.yaml":       {Data: []byte("name: lifecycle\n")},
	}
}

// A module's code packages are the `code` capability's DECLARED
// `code_packages`: on a real CWM, with the shipped type and capability, the
// manifest projected from a module object carrying them lists them as
// source_paths, and verify — which reads that manifest — runs its L1 tests
// in exactly those packages.
func TestSourcePathsComeFromTheDeclaredCodePackages(t *testing.T) {
	dir := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, dir, manifestcontrib.ContributePackPart("ktw-methods", "methods", "cwm", shippedModulePack(t)))

	mod := cwmapi.NewContainerObject("m", "m", "software-module", "", "m", "")
	mod.Capabilities["code"] = map[string]any{
		"kind":                  "library",
		gcapi.CodePackagesField: []any{"internal/extensions/m/", "app/ktw-m/"},
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: mod}); err != nil {
		t.Fatalf("create the module with its declared code_packages: %v", err)
	}

	in, err := projection.LoadManifestInputs(ctx, cwm, "m")
	if err != nil {
		t.Fatal(err)
	}
	manifest := projection.BuildManifest(in)
	if want := []string{"app/ktw-m/", "internal/extensions/m/"}; !reflect.DeepEqual(manifest.SourcePaths, want) {
		t.Fatalf("manifest source_paths = %v, want %v (the module's code_packages)", manifest.SourcePaths, want)
	}

	modulesRoot := filepath.Join(dir, "modules")
	if _, err := projection.WriteManifestJSON(manifest, filepath.Join(modulesRoot, "m", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(modulesRoot, "m")
	if err != nil {
		t.Fatal(err)
	}
	runner := buildRunner(Options{Module: "m", ProjectRoot: dir}, loaded)
	if want := []string{"./app/ktw-m/...", "./internal/extensions/m/..."}; !reflect.DeepEqual(runner.SourceRoots, want) {
		t.Fatalf("verify runs in %v, want %v", runner.SourceRoots, want)
	}
}

// Code has ONE home: a `code` path under the undeclared `facets` group never
// reaches the manifest, so a module declaring no code_packages projects no
// source_paths (and ManifestWrite then refuses to drop a written manifest's).
func TestAFacetsCodePathIsNotASourcePath(t *testing.T) {
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(), manifestcontrib.ContributePackPart("ktw-methods", "methods", "cwm", shippedModulePack(t)))

	legacy := cwmapi.NewContainerObject("f", "f", "software-module", "", "f", "")
	legacy.Capabilities["code"] = map[string]any{
		"kind":   "library",
		"facets": []any{map[string]any{"kind": "code", "path": "internal/extensions/f/"}},
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "f", Object: legacy}); err != nil {
		t.Fatal(err)
	}
	in, err := projection.LoadManifestInputs(ctx, cwm, "f")
	if err != nil {
		t.Fatal(err)
	}
	if got := projection.BuildManifest(in).SourcePaths; len(got) != 0 {
		t.Errorf("source_paths = %v from a facets code entry, want none: code_packages is the one source", got)
	}
}
