// SPDX-License-Identifier: Apache-2.0

package discovery

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDiscover_AcceptanceFixture runs the discovery walker against
// the purpose-built throwaway module at
// `internal/test-modules/gocode-acceptance/` (created by
// ktw-gocode-acceptance-test-module) and pins the expected
// inventory shape. This is also the v1 self-test for the §13
// LOCKED determinism contract — running twice must produce the
// same Inventory.
func TestDiscover_AcceptanceFixture(t *testing.T) {
	t.Parallel()
	root := fixtureRoot(t)

	first, err := Discover(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatalf("Discover (first run): %v", err)
	}

	// Three packages: public, internal, local.
	wantPkgs := map[string]bool{"public": false, "internal": false, "local": false}
	for _, p := range first.Packages {
		if _, ok := wantPkgs[p.Path]; ok {
			wantPkgs[p.Path] = true
		}
	}
	for pkg, seen := range wantPkgs {
		if !seen {
			t.Errorf("expected to discover package %q under acceptance fixture", pkg)
		}
	}

	// Public symbols: Add + Subtract from public/calc.go + ErrOverflow.
	var publicSymbols []string
	for _, s := range first.PublicSymbols {
		if s.Package == "public" {
			publicSymbols = append(publicSymbols, s.Name)
		}
	}
	for _, want := range []string{"Add", "Subtract", "ErrOverflow"} {
		if !contains(publicSymbols, want) {
			t.Errorf("expected exported symbol %q in public package; got %v", want, publicSymbols)
		}
	}

	// Tests: should find at least the 4 known tests
	// (TestAdd, TestSubtract, TestErrOverflow_message + TestValidatePair).
	if len(first.TestFunctions) < 4 {
		t.Errorf("expected ≥ 4 test functions; got %d", len(first.TestFunctions))
	}

	// LayoutHints. Fixture is a library with public/internal/local.
	if first.LayoutHints == nil {
		t.Fatal("LayoutHints not populated")
	}
	if !first.LayoutHints.HasPublicPkg {
		t.Error("HasPublicPkg = false; expected true")
	}
	if !first.LayoutHints.HasInternalPkg {
		t.Error("HasInternalPkg = false; expected true")
	}
	if !first.LayoutHints.HasLocalPkg {
		t.Error("HasLocalPkg = false; expected true")
	}
	if first.LayoutHints.SuggestedKind != "library" {
		t.Errorf("SuggestedKind = %q; want library", first.LayoutHints.SuggestedKind)
	}

	// Determinism — second run must produce the same inventory.
	second, err := Discover(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatalf("Discover (second run): %v", err)
	}
	if len(first.PublicSymbols) != len(second.PublicSymbols) {
		t.Errorf("non-deterministic PublicSymbols: %d vs %d", len(first.PublicSymbols), len(second.PublicSymbols))
	}
	for i := range first.PublicSymbols {
		if i >= len(second.PublicSymbols) {
			break
		}
		if first.PublicSymbols[i].Name != second.PublicSymbols[i].Name {
			t.Errorf("PublicSymbols[%d] differs between runs: %q vs %q",
				i, first.PublicSymbols[i].Name, second.PublicSymbols[i].Name)
		}
	}
	for i := range first.TestFunctions {
		if i >= len(second.TestFunctions) {
			break
		}
		if first.TestFunctions[i].TestRef != second.TestFunctions[i].TestRef {
			t.Errorf("TestFunctions[%d] differs between runs: %q vs %q",
				i, first.TestFunctions[i].TestRef, second.TestFunctions[i].TestRef)
		}
	}
}

func TestDiscover_SymbolKinds(t *testing.T) {
	t.Parallel()
	root := fixtureRoot(t)
	inv, err := Discover(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// Verify SymbolKind classifies funcs vs types vs vars correctly.
	kinds := map[SymbolKind]int{}
	for _, s := range inv.PublicSymbols {
		kinds[s.Kind]++
	}
	if kinds[SymbolKindFunc] == 0 {
		t.Error("expected ≥ 1 func symbol (Add/Subtract should classify as func)")
	}
	if kinds[SymbolKindVar] == 0 {
		t.Error("expected ≥ 1 var symbol (ErrOverflow should classify as var)")
	}
}

func TestDiscover_TestKindClassification(t *testing.T) {
	t.Parallel()
	root := fixtureRoot(t)
	inv, err := Discover(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, tf := range inv.TestFunctions {
		// All discovered tests in the fixture are TestKindTest.
		// If the fixture grows benchmarks/examples this test extends naturally.
		switch tf.Kind {
		case TestKindTest, TestKindBenchmark, TestKindExample, TestKindFuzz:
			// ok
		default:
			t.Errorf("unknown test kind %q for %s", tf.Kind, tf.Name)
		}
	}
}

func TestDiscover_ExcludeTests(t *testing.T) {
	t.Parallel()
	root := fixtureRoot(t)
	opts := DefaultOptions()
	opts.IncludeTests = false
	inv, err := Discover(context.Background(), root, opts)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(inv.TestFunctions) != 0 {
		t.Errorf("IncludeTests=false should yield 0 test functions; got %d", len(inv.TestFunctions))
	}
}

// fixtureRoot returns the absolute path to the gocode-acceptance
// throwaway module. We resolve relative to this test file so the
// walk works regardless of `go test` cwd quirks.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path via runtime.Caller")
	}
	// .../internal/extensions/gocode/internal/discovery/discovery_test.go
	// → up 4 to internal/, sideways to test-modules/gocode-acceptance/
	dir := filepath.Dir(thisFile) // discovery
	dir = filepath.Dir(dir)       // internal
	dir = filepath.Dir(dir)       // gocode
	dir = filepath.Dir(dir)       // extensions
	dir = filepath.Dir(dir)       // internal (top-level)
	return filepath.Join(dir, "test-modules", "gocode-acceptance")
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
