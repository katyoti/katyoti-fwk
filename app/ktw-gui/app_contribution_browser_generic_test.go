// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// TestResourcesPane_OneGenericViewForEveryKind proves the Resources pane's
// detail view is generic (ms-contribution-ext e22): its backend
// (app_contribution_browser.go) and its panel (ContributionBrowserPanel.tsx)
// name no kind the framework ships, so every kind — including one a pack
// declares with no code (the e19 fixture, walked by
// internal/test/catalog.TestGUIWalk_EveryEntryOfEveryKindOpens) — gets the same
// meta card, kind card, layers table and per-layer tabs through
// OpenContributionLayer. The one exception is the meta-model itself: a kind
// DECLARATION links to its entries, whichever kind it declares.
func TestResourcesPane_OneGenericViewForEveryKind(t *testing.T) {
	kinds := frameworkKinds(t)
	var hits []string

	// Backend: no string literal is a kind name.
	src := "app_contribution_browser.go"
	f, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", src, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil && kinds[s] {
				hits = append(hits, src+": "+strconv.Quote(s))
			}
		}
		return true
	})

	// Panel: no quoted kind name outside the meta-model.
	panel := filepath.Join("frontend", "src", "sections", "dev", "ContributionBrowserPanel.tsx")
	b, err := os.ReadFile(panel)
	if err != nil {
		t.Fatalf("read %s: %v", panel, err)
	}
	quoted := regexp.MustCompile(`'([a-z][a-z0-9-]*)'|"([a-z][a-z0-9-]*)"`)
	for _, m := range quoted.FindAllStringSubmatch(string(b), -1) {
		s := m[1] + m[2]
		if kinds[s] && !contribapi.IsMetaKind(s) {
			hits = append(hits, filepath.ToSlash(panel)+": "+strconv.Quote(s))
		}
	}

	if len(hits) > 0 {
		sort.Strings(hits)
		t.Errorf("the Resources pane names a kind — a kind-specific view has crept in; "+
			"render it through the generic detail (meta, kind, layers, OpenContributionLayer):\n  %s",
			strings.Join(hits, "\n  "))
	}
}

// frameworkKinds reads the name of every kind the framework ships.
func frameworkKinds(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join("..", "..", "internal", "core", "contribution", "embed", "contribution-type")
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil || len(files) < 10 {
		t.Fatalf("found %d kind declarations under %s (%v)", len(files), dir, err)
	}
	out := map[string]bool{}
	for _, p := range files {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var decl struct {
			Name string `yaml:"name"`
		}
		if err := marshal.Unmarshal(raw, marshal.YAML, &decl); err != nil || decl.Name == "" {
			t.Fatalf("%s: no kind name (%v)", p, err)
		}
		out[decl.Name] = true
	}
	return out
}
