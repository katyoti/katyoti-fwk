// SPDX-License-Identifier: Apache-2.0

package render

import (
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// helper to find an issue by Code; returns "" if not found.
func findCode(issues []ValidationIssue, code string) *ValidationIssue {
	for i := range issues {
		if issues[i].Code == code {
			return &issues[i]
		}
	}
	return nil
}

// minimalValid is the smallest LogicalDiagram that should pass all
// structural + referential checks (assuming the app-flux body is in
// the test catalog via ctxWithAppFlux).
func minimalValid() *drawioapi.LogicalDiagram {
	return &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Nodes:     []drawioapi.LogicalNode{{ID: "n1", Label: "A", Kind: "Application"}},
	}
}

func TestValidate_HappyPath(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	r, err := Validate(ctx, minimalValid())
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if r.HasErrors() {
		t.Errorf("HappyPath should have no errors, got: %v", r.Errors)
	}
}

func TestValidate_NilDiagram(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	r, err := Validate(ctx, nil)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !r.HasErrors() || findCode(r.Errors, "nil-diagram") == nil {
		t.Errorf("want nil-diagram error, got: %v", r.Errors)
	}
}

func TestValidate_SchemaError_MissingStencilSet(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Viewpoint = ""
	r, _ := Validate(ctx, ld)
	if !r.HasErrors() {
		t.Fatal("want schema error for missing stencilSet")
	}
	// Either "schema" code (Schema check caught it) or "viewpoint-not-found"
	// — both legitimate, depending on whether the schema's `required` check
	// or the URI pattern fires first.
	if findCode(r.Errors, "schema") == nil && findCode(r.Errors, "viewpoint-not-found") == nil {
		t.Errorf("want schema or viewpoint-not-found, got: %v", r.Errors)
	}
}

func TestValidate_StencilSetNotFound(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Viewpoint = "ktw-docs:/viewpoint/nonexistent"
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "viewpoint-not-found") == nil {
		t.Errorf("want viewpoint-not-found, got: %v", r.Errors)
	}
}

func TestValidate_UnknownNodeKind(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Nodes[0].Kind = "Mystery"
	r, _ := Validate(ctx, ld)
	issue := findCode(r.Errors, "unknown-kind")
	if issue == nil {
		t.Fatalf("want unknown-kind, got: %v", r.Errors)
	}
	if !strings.Contains(issue.Message, "Mystery") {
		t.Errorf("error should name the bad kind, got: %s", issue.Message)
	}
	if !strings.Contains(issue.Message, "Application") {
		t.Errorf("error should list available kinds, got: %s", issue.Message)
	}
}

func TestValidate_UnknownEdgeKind(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Nodes = append(ld.Nodes, drawioapi.LogicalNode{ID: "n2", Label: "B", Kind: "Application"})
	ld.Edges = []drawioapi.LogicalEdge{{ID: "e1", Source: "n1", Target: "n2", Kind: "BogusFlow"}}
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "unknown-edge-kind") == nil {
		t.Errorf("want unknown-edge-kind, got: %v", r.Errors)
	}
}

func TestValidate_DuplicateNodeID(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Nodes = append(ld.Nodes, drawioapi.LogicalNode{ID: "n1", Label: "Duplicate", Kind: "Application"})
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "duplicate-id") == nil {
		t.Errorf("want duplicate-id, got: %v", r.Errors)
	}
}

func TestValidate_DanglingEdgeSource(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Edges = []drawioapi.LogicalEdge{{ID: "e1", Source: "ghost", Target: "n1", Kind: "Flow"}}
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "dangling-source") == nil {
		t.Errorf("want dangling-source, got: %v", r.Errors)
	}
}

func TestValidate_DanglingEdgeTarget(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Edges = []drawioapi.LogicalEdge{{ID: "e1", Source: "n1", Target: "ghost", Kind: "Flow"}}
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "dangling-target") == nil {
		t.Errorf("want dangling-target, got: %v", r.Errors)
	}
}

func TestValidate_DanglingNodeZone(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Nodes[0].Zone = "z-ghost"
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "dangling-zone") == nil {
		t.Errorf("want dangling-zone, got: %v", r.Errors)
	}
}

func TestValidate_DanglingZoneMember(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Zones = []drawioapi.LogicalZone{{ID: "z1", Label: "Z", Order: 0, Members: []string{"ghost"}}}
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "dangling-member") == nil {
		t.Errorf("want dangling-member, got: %v", r.Errors)
	}
}

func TestValidate_DuplicateEdgeID(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Nodes = append(ld.Nodes, drawioapi.LogicalNode{ID: "n2", Label: "B", Kind: "Application"})
	ld.Edges = []drawioapi.LogicalEdge{
		{ID: "e1", Source: "n1", Target: "n2", Kind: "Flow"},
		{ID: "e1", Source: "n2", Target: "n1", Kind: "Flow"},
	}
	r, _ := Validate(ctx, ld)
	dups := 0
	for _, i := range r.Errors {
		if i.Code == "duplicate-id" {
			dups++
		}
	}
	if dups != 1 {
		t.Errorf("want 1 duplicate-id, got %d: %v", dups, r.Errors)
	}
}

func TestValidate_DuplicateZoneID(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Zones = []drawioapi.LogicalZone{
		{ID: "z1", Label: "A", Order: 0},
		{ID: "z1", Label: "B", Order: 1},
	}
	r, _ := Validate(ctx, ld)
	dups := 0
	for _, i := range r.Errors {
		if i.Code == "duplicate-id" {
			dups++
		}
	}
	if dups != 1 {
		t.Errorf("want 1 duplicate-id for zone, got %d: %v", dups, r.Errors)
	}
}

// TestValidate_ForbiddenRelationship: a relationship the notation's rules
// forbid is an error naming the relationships that are allowed.
func TestValidate_ForbiddenRelationship(t *testing.T) {
	cat := drawiotesting.NewCatalog(nil)
	cat.Add(drawiotesting.Contribution(t, "ktw-test", drawioapi.ContributionTypeNotation, "ruled", drawioapi.NotationBody{
		ID: "ruled", DisplayName: "Ruled",
		Elements:      map[string]drawioapi.ElementKind{"Actor": {Label: "Actor"}, "Process": {Label: "Process"}},
		Relationships: map[string]drawioapi.RelationshipKind{"triggering": {Label: "Triggering"}, "assignment": {Label: "Assignment"}},
		Rules: &drawioapi.NotationRules{Allowed: []drawioapi.RelationshipRule{
			{Relationship: "assignment", Sources: []string{"Actor"}, Targets: []string{"Process"}},
		}},
	}))
	cat.Add(drawiotesting.Contribution(t, "ktw-test", drawioapi.ContributionTypeDrawioTemplate, "t", drawioapi.TemplateLibraryBody{ID: "t", Drawio: "<mxfile/>"}))
	cat.Add(drawiotesting.Contribution(t, "ktw-test", drawioapi.ContributionTypeViewpoint, "v", drawioapi.ViewpointBody{
		ID: "v", Notation: "ktw-test:/notation/ruled", Templates: "ktw-test:/drawio-template/t",
	}))
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-test:/viewpoint/v",
		Nodes:     []drawioapi.LogicalNode{{ID: "a", Label: "A", Kind: "Actor"}, {ID: "p", Label: "P", Kind: "Process"}},
		Edges: []drawioapi.LogicalEdge{
			{ID: "ok", Source: "a", Target: "p", Kind: "assignment"},
			{ID: "bad", Source: "a", Target: "p", Kind: "triggering"},
		},
	}
	r, err := Validate(drawiotesting.WithCatalog(t, cat), ld)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Errors) != 1 {
		t.Fatalf("want exactly one error, got %v", r.Errors)
	}
	e := r.Errors[0]
	if e.Code != "forbidden-relationship" || e.Path != "edges[1].kind" || !strings.Contains(e.Message, "allowed: assignment") {
		t.Errorf("unexpected error: %+v", e)
	}
}

// TestValidate_ZoneParents: a zone's parent must exist and a zone cannot
// be inside itself.
func TestValidate_ZoneParents(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := minimalValid()
	ld.Zones = []drawioapi.LogicalZone{{ID: "a", Label: "A", Parent: "ghost"}}
	r, _ := Validate(ctx, ld)
	if findCode(r.Errors, "dangling-zone") == nil {
		t.Errorf("want dangling-zone for an unknown parent, got: %v", r.Errors)
	}
	ld.Zones = []drawioapi.LogicalZone{{ID: "a", Label: "A", Parent: "b"}, {ID: "b", Label: "B", Order: 1, Parent: "a"}}
	r, _ = Validate(ctx, ld)
	if findCode(r.Errors, "zone-cycle") == nil {
		t.Errorf("want zone-cycle, got: %v", r.Errors)
	}
	ld.Zones = []drawioapi.LogicalZone{{ID: "pool", Label: "Pool"}, {ID: "lane", Label: "Lane", Parent: "pool"}}
	ld.Nodes[0].Zone = "lane"
	r, _ = Validate(ctx, ld)
	if len(r.Errors) > 0 {
		t.Errorf("a lane in a pool is valid, got: %v", r.Errors)
	}
}
