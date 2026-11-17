// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// Sections replaced the page region: a part of a page fed by
// its own source is a section - an empty box with its load and placeholder
// (render.LoadedSection) - and deleted the region path - its
// `region` kind, its renderer, the loadRegion host service and the GUI's
// loader. It does not come back: no display-library or GUI code, no page
// grammar, no sidecar's page (Go code or declared object panel) names it.
var regionPath = regexp.MustCompile(`loadRegion|RegionRenderer|UIRegionNode|RegionPage\b|nodes/region\.tsx|case 'region'|["']?kind["']?\s*:\s*["']region["']|\bkind:\s*region\b`)

func TestTheRegionPathStaysDeleted(t *testing.T) {
	if slices.Contains(render.PageKinds["layout"], "region") {
		t.Error("render.PageKinds holds the region kind - it was deleted: a part fed by its own source is a section (render.LoadedSection)")
	}
	root := repoRoot(t)
	checked := 0
	walk := func(dir string, keep func(rel string) bool) {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", "_compiled", "testdata":
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") || !keep(rel) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			if m := regionPath.FindString(string(raw)); m != "" {
				t.Errorf("%s names %q - the region path was deleted: a part fed by its own source is a section (render.LoadedSection, a box with load and placeholder)", rel, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	ui := func(rel string) bool { return strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".tsx") }
	walk("app/ktw-display-lib/src", ui)
	walk("app/ktw-gui/frontend/src", ui)
	goCode := func(rel string) bool { return strings.HasSuffix(rel, ".go") }
	walk("internal/core/render", goCode)
	walk("internal/extensions", goCode)
	panels := func(rel string) bool {
		return (strings.Contains(rel, "/object-panels/") || strings.Contains(rel, "/embed/")) && strings.HasSuffix(rel, ".yaml")
	}
	walk("app", panels)
	walk("internal/extensions", panels)
	if checked < 1000 {
		t.Fatalf("checked %d files: the roots are wrong", checked)
	}
}
