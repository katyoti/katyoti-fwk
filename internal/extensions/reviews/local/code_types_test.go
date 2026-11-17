// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// ktw-code's code review types, as shipped (app/ktw-code/embed/cwm/
// review-types): every one applies to a module with code packages and is run
// by the code-review skill with its own guide; none applies to a module
// without code.
func TestTheCodeReviewTypesApplyToAModuleWithCode(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "ktw-code", "embed", "cwm", "review-types")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"capabilities/module.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/capabilities/module.yaml")},
		"data-types/cap-module.dsl.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/data-types/cap-module.dsl.yaml")},
		"capabilities/code.yaml":         {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"data-types/cap-code.dsl.yaml":   {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/data-types/cap-code.dsl.yaml")},
	}
	var shipped []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".i18n.yaml") {
			continue
		}
		files["review-types/"+e.Name()] = &fstest.MapFile{Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/review-types/"+e.Name())}
		shipped = append(shipped, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	if len(shipped) != 13 {
		t.Fatalf("ktw-code ships %d code review types, want 13: %v", len(shipped), shipped)
	}
	w := setupWith(t, files)
	w.create(t, cwmapi.Object{ID: "m.code", Type: "module", Parent: "m", Name: "with code",
		Capabilities: map[string]map[string]any{"code": {"code_packages": []any{"internal/extensions/x"}}}})

	got := func(id string) map[string]api.ReviewTypeDef {
		out, err := w.svc.TypesFor(w.ctx, api.TypesInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]api.ReviewTypeDef{}
		for _, d := range out.Dimensions {
			for _, ty := range d.Types {
				m[ty.Name] = ty
			}
		}
		return m
	}
	withCode := got("m.code")
	var names []string
	for _, n := range shipped {
		ty, ok := withCode[n]
		if !ok {
			t.Errorf("%s does not apply to a module with code", n)
			continue
		}
		names = append(names, n)
		if ty.Runner.Skill != "ktw-code:/skill/code-review" || ty.Runner.Input["review"] != n ||
			!strings.HasPrefix(ty.Runner.Input["guide"].(string), "ktw-code:/reference/review/") || ty.Dimension == "" {
			t.Errorf("%s as shipped: %+v", n, ty)
		}
	}
	sort.Strings(names)
	for n := range got("m") {
		for _, s := range shipped {
			if n == s {
				t.Errorf("%s applies to a module without code", n)
			}
		}
	}
}
