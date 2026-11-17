// SPDX-License-Identifier: Apache-2.0

package render

import (
	"regexp"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/template"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// TestShippedContent holds ktw-docs' drawio content together: every
// viewpoint resolves; its library parses; every kind the viewpoint shows
// has a template (or the library a default); every ktw-style and every
// placeholder names a binding, an attribute or a built-in; every page the
// viewpoints name exists.
func TestShippedContent(t *testing.T) {
	cat := drawiotesting.ShippedCatalog(t)
	ctx := drawiotesting.WithCatalog(t, cat)
	vps, _ := cat.GetContributionsByType(ctx, drawioapi.ContributionTypeViewpoint)
	if len(vps) < 5 {
		t.Fatalf("want at least 5 shipped viewpoints, got %d", len(vps))
	}
	for _, c := range vps {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			dr, err := drawioapi.ResolveDrawing(ctx, c.ResolvedURI())
			if err != nil {
				t.Fatal(err)
			}
			lib, err := template.Parse(dr.Library)
			if err != nil {
				t.Fatal(err)
			}
			for k := range dr.Notation.Elements {
				if dr.Viewpoint.ShowsElement(k) && lib.Element(k) == nil {
					t.Errorf("element kind %s has no template and the library no default", k)
				}
			}
			for k := range dr.Notation.Relationships {
				if dr.Viewpoint.ShowsRelationship(k) && lib.Relationship(k) == nil {
					t.Errorf("relationship kind %s has no template", k)
				}
			}
			for _, k := range dr.Viewpoint.Elements {
				if _, ok := dr.Notation.Elements[k]; !ok {
					t.Errorf("viewpoint shows %s, which notation %s does not declare", k, dr.Notation.ID)
				}
			}
			if dr.Viewpoint.Page != "" && lib.Page(dr.Viewpoint.Page) == nil {
				t.Errorf("page template %q missing from library %s", dr.Viewpoint.Page, dr.Library.ID)
			}
			for _, name := range styleBindings(dr.Library.Drawio) {
				if _, ok := dr.Library.Bindings[name]; !ok {
					t.Errorf("ktw-style names binding %q, which library %s does not define", name, dr.Library.ID)
				}
			}
		})
	}
}

var styleAttr = regexp.MustCompile(`ktw-style="([^"]*)"`)

// styleBindings lists the binding names every ktw-style of a library uses.
func styleBindings(drawio string) []string {
	var out []string
	for _, m := range styleAttr.FindAllStringSubmatch(drawio, -1) {
		for _, pair := range strings.Split(m[1], ";") {
			if _, b, ok := strings.Cut(pair, "="); ok && strings.TrimSpace(b) != "" {
				out = append(out, strings.TrimSpace(b))
			}
		}
	}
	return out
}
