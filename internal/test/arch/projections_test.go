// SPDX-License-Identifier: Apache-2.0

// Package arch holds architecture guards over the whole tree: rules about who
// may import or declare what, read from the import graph (`go list`) and the
// syntax tree, not from any running application.
//
// The rules here are e17's — "the catalog is the discovery authority; runtime
// registries are projections or caches for execution"
// (modules/contributions/projections.md).
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const module = "github.com/katyoti/katyoti-fwk"

// projectionInternals are the runtime projections' own implementation trees
// (projections.md rows: SkillManager, TaskLibrary, CWM TypeRegistry,
// SchemaService). They read contributions through mbapi (the catalog's
// consumer surface) and never through the manifest extension's wiring or the
// hub transport.
var projectionInternals = []string{
	module + "/internal/extensions/skills/internal/...",
	module + "/internal/extensions/task/internal/...",
	// task/local wires TaskService's discovery library (WrapTaskLibraryWithRemotes);
	// it hands the service catalog projections only (e24 / N1).
	module + "/internal/extensions/task/local",
	module + "/internal/extensions/cwm/internal/...",
	module + "/internal/extensions/schema/internal/...",
}

// forbiddenForProjections: the manifest extension's wiring (`local`) and its
// hub client (`remote`). Reaching either from a projection's internals is
// discovering universe content outside the catalog contract.
var forbiddenForProjections = []string{
	module + "/internal/extensions/manifest/local",
	module + "/internal/extensions/manifest/remote",
}

// TestProjectionsConsumeThroughMbapi — guard (a): no package under a
// projection's internal tree imports manifest/local or manifest/remote, in
// production code or tests.
func TestProjectionsConsumeThroughMbapi(t *testing.T) {
	args := append([]string{"list", "-f",
		"{{.ImportPath}}|{{join .Imports \",\"}}|{{join .TestImports \",\"}}|{{join .XTestImports \",\"}}"},
		projectionInternals...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 10 {
		t.Fatalf("go list returned %d packages — the guard is not looking at the projection trees", len(lines))
	}
	var violations []string
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		for _, imp := range strings.Split(strings.ReplaceAll(parts[1], "|", ","), ",") {
			for _, f := range forbiddenForProjections {
				if imp == f || strings.HasPrefix(imp, f+"/") {
					violations = append(violations, parts[0]+" imports "+imp)
				}
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("a runtime projection reaches the manifest wiring directly (%d):\n  %s\n"+
			"consume contributions through mbapi (Query / GetOne / Open over the catalog) — see modules/contributions/projections.md",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// allowedProducers are the packages that may declare or register a
// ContributionProvider — the one advertisement path. Every `*/contrib`
// package, plus the producers inventory §C lists outside one (each named):
var allowedProducers = map[string]string{
	"internal/appcore":                     "sidecarInfoProvider — the sidecar-info entry (sidecar_ext.go)",
	"internal/appcore/sidecar":             "ConfigResourceProvider / ConfigShapeProvider — config-file, config-program, env, config shapes",
	"internal/core/contribution":           "systemContributionProvider — the system meta-model",
	"internal/core/contribution/builtin":   "kindProvider + GoTypeDataTypeProvider — the built-in leaf kinds and their data-types, published once per binary (publish.go)",
	"internal/core/publish":                "servicesProvider — one service entry per published service",
	"internal/extensions/personas/contrib": "PersonaProvider — persona (the generic producer, wired from the declared kind)",
	"internal/extensions/manifest/local":   "ContributeResources / ContributeConfigResources — the generic registration helpers",
}

func isAllowedProducer(pkgDir string) bool {
	if path.Base(pkgDir) == "contrib" {
		return true
	}
	_, ok := allowedProducers[pkgDir]
	return ok
}

// helperRegistrationDeviations are the `local` packages that register a
// provider THROUGH a contrib helper (`manifestcontrib.Contribute*`,
// `cwmcontrib.Contribute*`, …) instead of from a `contrib` package of their
// own. The audit (blind audit M8) found the guard blind to them: a helper
// call names no `ContributionProvider` and calls no `application.Contribute`,
// so the syntactic signals above never fired. Each is a deviation to move
// into the extension's `contrib` package; the list is meant to shrink, and
// a new entry needs a reason here.
var helperRegistrationDeviations = map[string]string{
	"internal/extensions/cwm/local":  "ContributeCWMTypes + ContributeDataTypeResources over the TypesFS (services.go)",
	"internal/extensions/task/local": "ContributeDataTypeResources over the task data-types (services.go)",
}

// TestOneAdvertisementPath — guard (b): only an allowed producer package
// (a `contrib` package, or one listed in allowedProducers) may
//
//  1. declare a provider — a method `Enumerate(ctx) ([]<pkg>.Contribution, error)`
//     on a non-test type, or
//  2. register one — call `application.Contribute(...)` in a file that names
//     `ContributionProvider`, or
//  3. register one through a helper — call `<x>.Contribute*(…)` where `<x>`
//     is an import of a `contrib` package (the helpers return an Extension
//     that contributes a provider), unless the package is a named deviation
//     in helperRegistrationDeviations.
//
// A runtime service doing any of these would be publishing a second
// advertisement path beside the catalog's. Syntax-level on purpose: no type
// checking, so the rule is exactly what is written above.
func TestOneAdvertisementPath(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..")
	var violations []string
	declared := map[string]bool{}
	helperCallers := map[string]bool{}
	scanned := 0
	for _, top := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "frontend", "testdata":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			scanned++
			rel, _ := filepath.Rel(root, filepath.Dir(p))
			pkgDir := filepath.ToSlash(rel)
			src, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			declares, registers, viaHelper := producerSignals(t, p, src)
			if declares {
				declared[pkgDir] = true
			}
			if viaHelper != "" {
				helperCallers[pkgDir] = true
			}
			if (declares || registers) && !isAllowedProducer(pkgDir) {
				what := "registers a ContributionProvider"
				if declares {
					what = "declares a ContributionProvider (Enumerate)"
				}
				fileRel, _ := filepath.Rel(root, p)
				violations = append(violations, filepath.ToSlash(fileRel)+": "+what)
			}
			// The helper signal is judged inside `internal/extensions/` only: a
			// binary's cmd/bootstrap.go, the sidecar kit and a test fixture
			// COMPOSE the helpers over the binary's own files, which is what
			// the helpers exist for; an extension package doing it is
			// registering a provider outside its contrib package.
			if _, deviation := helperRegistrationDeviations[pkgDir]; viaHelper != "" && strings.HasPrefix(pkgDir, "internal/extensions/") && !isAllowedProducer(pkgDir) && !deviation {
				fileRel, _ := filepath.Rel(root, p)
				violations = append(violations, filepath.ToSlash(fileRel)+": registers a provider through "+viaHelper+" (a contrib helper) outside a contrib package")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}
	if scanned < 500 || len(declared) < 5 {
		t.Fatalf("scanned %d files, found providers in %d packages — the guard is not seeing the tree", scanned, len(declared))
	}
	if len(helperCallers) < 2 {
		t.Fatalf("found helper registrations in %d packages — the helper signal is not seeing the tree", len(helperCallers))
	}
	// A deviation that no longer registers through a helper has been fixed:
	// drop it from the list, or the list stops describing the tree.
	for pkg := range helperRegistrationDeviations {
		if !helperCallers[pkg] {
			t.Errorf("helperRegistrationDeviations names %s, which registers no provider through a contrib helper any more — remove the entry", pkg)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("a package outside the producer set publishes contributions (%d):\n  %s\n"+
			"move the provider into the owning extension's `contrib` package; runtime services are projections, not advertisers "+
			"(modules/contributions/projections.md)", len(violations), strings.Join(violations, "\n  "))
	}
}

// producerSignals parses one file and reports whether it declares an
// Enumerate method returning []…Contribution, whether it calls
// application.Contribute in a file that names ContributionProvider, and —
// viaHelper, the qualified call when it does — whether it calls a
// `Contribute*` function of an imported `contrib` package.
func producerSignals(t *testing.T, file string, src []byte) (declares, registers bool, viaHelper string) {
	t.Helper()
	s := string(src)
	if !strings.Contains(s, "Enumerate(") && !strings.Contains(s, ".Contribute") {
		return false, false, ""
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	namesProvider := strings.Contains(s, "ContributionProvider")
	// Local name → import path, for every import of a `contrib` package: the
	// helpers are called through whatever alias the file chose.
	contribImports := map[string]string{}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if path.Base(p) != "contrib" {
			continue
		}
		name := "contrib"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		contribImports[name] = p
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Recv != nil && x.Name.Name == "Enumerate" && returnsContributions(x.Type) {
				declares = true
			}
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Contribute" && id.Name == "application" && namesProvider {
				registers = true
			}
			if _, isContrib := contribImports[id.Name]; isContrib && strings.HasPrefix(sel.Sel.Name, "Contribute") && viaHelper == "" {
				viaHelper = id.Name + "." + sel.Sel.Name
			}
		}
		return true
	})
	return declares, registers, viaHelper
}

func returnsContributions(ft *ast.FuncType) bool {
	if ft.Results == nil || len(ft.Results.List) == 0 {
		return false
	}
	arr, ok := ft.Results.List[0].Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	switch el := arr.Elt.(type) {
	case *ast.SelectorExpr:
		return el.Sel.Name == "Contribution"
	case *ast.Ident:
		return el.Name == "Contribution"
	}
	return false
}
