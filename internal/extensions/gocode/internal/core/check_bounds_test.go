// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// A perspective that does not end within its bound answers a finding naming
// it — the check never hangs on it — while one that ends in time answers what
// it found.
func TestAPerspectiveThatOverrunsIsAFinding(t *testing.T) {
	start := time.Now()
	got := bounded(context.Background(), api.AuditPerspectiveQuality, 50*time.Millisecond, func(ctx context.Context) []api.AuditFinding {
		select {
		case <-time.After(10 * time.Second): // a test that hangs, ignoring its context
		case <-ctx.Done():
			time.Sleep(10 * time.Second) // and even past its cancellation
		}
		return nil
	})
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the overrun was awaited: %s", took)
	}
	if len(got) != 1 || got[0].Perspective != api.AuditPerspectiveQuality || got[0].Severity != api.FindingSeverityHigh ||
		!strings.Contains(got[0].Title, "quality did not finish within") {
		t.Fatalf("an overrun: %+v", got)
	}
	ok := bounded(context.Background(), api.AuditPerspectiveLint, time.Second, func(context.Context) []api.AuditFinding {
		return []api.AuditFinding{{Title: "found"}}
	})
	if len(ok) != 1 || ok[0].Title != "found" {
		t.Errorf("a perspective in time answers what it found: %+v", ok)
	}
}

// A change's scope is the module's packages its .go paths touch — not the
// whole module, not another module's packages, nothing for a change without
// Go files.
func TestAChangeScopesTheChecksToThePackagesItTouches(t *testing.T) {
	mod := api.Module{Facets: []api.Facet{{Kind: api.FacetCode, Path: "internal/extensions/p"}}}
	root := t.TempDir()
	for _, f := range []string{"internal/extensions/p/internal/core/a.go", "internal/extensions/p/api/x.go", "internal/extensions/q/other.go"} {
		writeGoFile(t, root, f)
	}
	sc := changeScope(mod, root, []string{
		"internal/extensions/p/internal/core/a.go",
		"internal/extensions/p/internal/core/b_test.go",
		"internal/extensions/p/api/x.go",
		"internal/extensions/q/other.go",
		"modules/p/concepts.md",
	})
	if got := strings.Join(sc.patterns, " "); got != "./internal/extensions/p/api ./internal/extensions/p/internal/core" {
		t.Errorf("patterns: %q", got)
	}
	if got := strings.Join(sc.roots, " "); got != "./internal/extensions/p/api ./internal/extensions/p/internal/core" {
		t.Errorf("roots: %q", got)
	}
	if sc.dir != root {
		t.Errorf("dir: %q", sc.dir)
	}
	if none := changeScope(mod, root, []string{"modules/p/concepts.md"}); len(none.patterns) != 0 {
		t.Errorf("a change with no Go file has nothing to check: %+v", none)
	}
	if whole := moduleScope(mod); strings.Join(whole.patterns, " ") != "./internal/extensions/p/..." {
		t.Errorf("the module's scope: %+v", whole)
	}
}

// A changed .go path whose folder no longer holds a Go file - a package the
// change deleted - is left out of the scope: go vet would fail on the missing
// folder and file a false critical.
func TestAChangeLeavesADeletedPackageOutOfTheScope(t *testing.T) {
	mod := api.Module{Facets: []api.Facet{{Kind: api.FacetCode, Path: "internal/extensions/p"}}}
	root := t.TempDir()
	writeGoFile(t, root, "internal/extensions/p/core/a.go")
	if err := os.MkdirAll(filepath.Join(root, "internal/extensions/p/emptied"), 0o755); err != nil {
		t.Fatal(err)
	}
	sc := changeScope(mod, root, []string{
		"internal/extensions/p/contrib/gone.go", // the folder was deleted
		"internal/extensions/p/emptied/gone.go", // the folder stays, its Go files gone
		"internal/extensions/p/core/a.go",
	})
	if got := strings.Join(sc.patterns, " "); got != "./internal/extensions/p/core" {
		t.Errorf("patterns: %q", got)
	}
	if only := changeScope(mod, root, []string{"internal/extensions/p/contrib/gone.go"}); len(only.patterns) != 0 {
		t.Errorf("a change that only deleted a package has nothing to check: %+v", only)
	}
}

func writeGoFile(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
