// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"errors"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

func sampleDrawing(t *testing.T, provider string) []contribapi.Contribution {
	t.Helper()
	return []contribapi.Contribution{
		drawiotesting.Contribution(t, provider, drawioapi.ContributionTypeNotation, "mini", drawioapi.NotationBody{
			ID: "mini", DisplayName: "Mini",
			Elements:      map[string]drawioapi.ElementKind{"Box": {Label: "Box"}},
			Relationships: map[string]drawioapi.RelationshipKind{"link": {Label: "Link"}},
		}),
		drawiotesting.Contribution(t, provider, drawioapi.ContributionTypeDrawioTemplate, "mini", drawioapi.TemplateLibraryBody{
			ID: "mini", DisplayName: "Mini", Drawio: "<mxfile/>",
		}),
		drawiotesting.Contribution(t, provider, drawioapi.ContributionTypeViewpoint, "mini-all", drawioapi.ViewpointBody{
			ID: "mini-all", DisplayName: "Mini", Notation: provider + ":/notation/mini", Templates: provider + ":/drawio-template/mini",
		}),
	}
}

// TestResolveDrawing_HappyPath resolves a viewpoint and what it names.
func TestResolveDrawing_HappyPath(t *testing.T) {
	ctx := drawiotesting.WithCatalog(t, drawiotesting.NewCatalog(map[string][]contribapi.Contribution{"ktw-docs": sampleDrawing(t, "ktw-docs")}))
	d, err := drawioapi.ResolveDrawing(ctx, "ktw-docs:/viewpoint/mini-all")
	if err != nil {
		t.Fatal(err)
	}
	if d.Notation.Elements["Box"].Label != "Box" || d.Library.Drawio != "<mxfile/>" || d.Viewpoint.ID != "mini-all" {
		t.Errorf("unexpected drawing: %+v", d)
	}
}

// TestResolveDrawing_CrossSidecar resolves a viewpoint another sidecar
// publishes, whose notation lives in ktw-docs.
func TestResolveDrawing_CrossSidecar(t *testing.T) {
	cs := sampleDrawing(t, "ktw-docs")
	cs = append(cs, drawiotesting.Contribution(t, "ktw-acme", drawioapi.ContributionTypeViewpoint, "acme", drawioapi.ViewpointBody{
		ID: "acme", DisplayName: "Acme", Notation: "ktw-docs:/notation/mini", Templates: "ktw-docs:/drawio-template/mini",
	}))
	cat := drawiotesting.NewCatalog(nil)
	for _, c := range cs {
		cat.Add(c)
	}
	d, err := drawioapi.ResolveDrawing(drawiotesting.WithCatalog(t, cat), "ktw-acme:/viewpoint/acme")
	if err != nil {
		t.Fatal(err)
	}
	if d.Notation.ID != "mini" {
		t.Errorf("notation: got %q", d.Notation.ID)
	}
}

func TestResolveViewpoint_NotFound(t *testing.T) {
	ctx := drawiotesting.WithCatalog(t, drawiotesting.NewCatalog(map[string][]contribapi.Contribution{"ktw-docs": sampleDrawing(t, "ktw-docs")}))
	_, err := drawioapi.ResolveViewpoint(ctx, "ktw-docs:/viewpoint/nope")
	if !errors.Is(err, drawioapi.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestResolveViewpoint_WrongKind(t *testing.T) {
	ctx := drawiotesting.WithCatalog(t, drawiotesting.NewCatalog(nil))
	_, err := drawioapi.ResolveViewpoint(ctx, "ktw-docs:/notation/mini")
	if err == nil || !strings.Contains(err.Error(), "is not a viewpoint") {
		t.Fatalf("want a wrong-kind error, got %v", err)
	}
}

func TestResolveViewpoint_MalformedURI(t *testing.T) {
	ctx := drawiotesting.WithCatalog(t, drawiotesting.NewCatalog(nil))
	if _, err := drawioapi.ResolveViewpoint(ctx, "not a uri"); err == nil {
		t.Fatal("want a parse error")
	}
}

func TestResolveDrawing_MissingNotation(t *testing.T) {
	cat := drawiotesting.NewCatalog(nil)
	cat.Add(drawiotesting.Contribution(t, "ktw-docs", drawioapi.ContributionTypeViewpoint, "orphan", drawioapi.ViewpointBody{
		ID: "orphan", Notation: "ktw-docs:/notation/none", Templates: "ktw-docs:/drawio-template/none",
	}))
	if _, err := drawioapi.ResolveDrawing(drawiotesting.WithCatalog(t, cat), "ktw-docs:/viewpoint/orphan"); !errors.Is(err, drawioapi.ErrNotFound) {
		t.Fatalf("want ErrNotFound for the notation, got %v", err)
	}
}

// TestNotation_Allows covers kinds, categories, wildcards and the
// absence of rules.
func TestNotation_Allows(t *testing.T) {
	n := &drawioapi.NotationBody{
		Elements: map[string]drawioapi.ElementKind{
			"Actor": {Category: "business"}, "Process": {Category: "business"}, "App": {Category: "application"},
		},
		Relationships: map[string]drawioapi.RelationshipKind{"triggering": {}, "serving": {}},
	}
	if !n.Allows("Actor", "triggering", "App") {
		t.Error("no rules allows everything")
	}
	n.Rules = &drawioapi.NotationRules{Allowed: []drawioapi.RelationshipRule{
		{Relationship: "triggering", Sources: []string{"category:business"}, Targets: []string{"Process"}},
		{Relationship: "serving", Sources: []string{"App"}, Targets: []string{"*"}},
	}}
	for _, c := range []struct {
		src, rel, tgt string
		want          bool
	}{
		{"Actor", "triggering", "Process", true},
		{"App", "triggering", "Process", false},
		{"Actor", "triggering", "App", false},
		{"App", "serving", "Actor", true},
		{"Actor", "serving", "App", false},
		{"Ghost", "serving", "App", true}, // unknown kinds are validation's concern
	} {
		if got := n.Allows(c.src, c.rel, c.tgt); got != c.want {
			t.Errorf("Allows(%s, %s, %s) = %v, want %v", c.src, c.rel, c.tgt, got, c.want)
		}
	}
}
