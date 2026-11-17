// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// skillBodyType programmatically builds the canonical SkillBody Type AST
// matching internal/extensions/skills/embed/data-types/skill-body.dsl.yaml.
// Built in Go rather than parsed from the DSL so the test doesn't reach
// into the internal schema parser.
func skillBodyType() *schemaapi.Type {
	return &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Fields: map[string]*schemaapi.Type{
			"name": {Kind: schemaapi.KindString},
			"requires": {
				Kind: schemaapi.KindArray,
				Items: &schemaapi.TypeRef{Inline: &schemaapi.Type{
					Kind:     schemaapi.KindLink,
					Target:   "Contribution",
					LinkMode: schemaapi.LinkMany2One,
				}},
			},
		},
	}
}

// TestExtractEdges_arrayOfLinks asserts the canonical skill-body shape
// extracts one edge per `requires:` entry.
func TestExtractEdges_arrayOfLinks(t *testing.T) {
	body := json.RawMessage(`{
		"name": "doc-create",
		"requires": ["ktw-docs:/skill/template-engine", "agent:/plugin/workspace"]
	}`)
	edges, err := core.ExtractEdges(body, skillBodyType())
	if err != nil {
		t.Fatalf("ExtractEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2: %+v", len(edges), edges)
	}
	// Order across map fields isn't guaranteed, but within the array it is.
	wantTargets := map[string]string{
		"ktw-docs:/skill/template-engine": "requires[0]",
		"agent:/plugin/workspace":         "requires[1]",
	}
	for _, e := range edges {
		if e.Relation != "requires" {
			t.Errorf("Relation = %q, want %q", e.Relation, "requires")
		}
		wantSource, ok := wantTargets[e.Target]
		if !ok {
			t.Errorf("unexpected Target %q", e.Target)
			continue
		}
		if e.Source != wantSource {
			t.Errorf("Target %q: Source = %q, want %q", e.Target, e.Source, wantSource)
		}
	}
}

// TestExtractEdges_emptyRequires asserts that omitting the optional
// edge field produces zero edges (not an error).
func TestExtractEdges_emptyRequires(t *testing.T) {
	body := json.RawMessage(`{"name": "lonely-skill"}`)
	edges, err := core.ExtractEdges(body, skillBodyType())
	if err != nil {
		t.Fatalf("ExtractEdges: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("got %d edges, want 0: %+v", len(edges), edges)
	}
}

// programBodyType is the canonical ConfigProgramBody Type AST shape
// (one2one link via `installSkill`).
func programBodyType() *schemaapi.Type {
	return &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Fields: map[string]*schemaapi.Type{
			"name": {Kind: schemaapi.KindString},
			"installSkill": {
				Kind:     schemaapi.KindLink,
				Target:   "Contribution",
				LinkMode: schemaapi.LinkOne2One,
			},
		},
	}
}

func TestExtractEdges_singleLink(t *testing.T) {
	body := json.RawMessage(`{
		"name": "pandoc",
		"installSkill": "ktw-docs:/skill/install-pandoc"
	}`)
	edges, err := core.ExtractEdges(body, programBodyType())
	if err != nil {
		t.Fatalf("ExtractEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1: %+v", len(edges), edges)
	}
	if edges[0].Relation != "installSkill" {
		t.Errorf("Relation = %q, want %q", edges[0].Relation, "installSkill")
	}
	if edges[0].Target != "ktw-docs:/skill/install-pandoc" {
		t.Errorf("Target = %q", edges[0].Target)
	}
	if edges[0].Source != "installSkill" {
		t.Errorf("Source = %q, want %q", edges[0].Source, "installSkill")
	}
}

// TestExtractEdges_nilDataType asserts a Contribution without a Schema
// gets zero edges (not an error).
func TestExtractEdges_nilDataType(t *testing.T) {
	body := json.RawMessage(`{"requires": ["x:/skill/y"]}`)
	edges, err := core.ExtractEdges(body, nil)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

// TestExtractEdges_invalidBody asserts a malformed JSON body returns
// an error (catalog handles it as a lint finding, not an ingest fail).
func TestExtractEdges_invalidBody(t *testing.T) {
	_, err := core.ExtractEdges([]byte("not json"), skillBodyType())
	if err == nil {
		t.Errorf("expected error on invalid JSON body, got nil")
	}
}

// TestExtractEdges_skipNonLinkFields asserts that non-edge fields don't
// produce edges (only LinkMode-marked fields do).
func TestExtractEdges_skipNonLinkFields(t *testing.T) {
	mixedType := &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Fields: map[string]*schemaapi.Type{
			"name": {Kind: schemaapi.KindString},
			"hashtags": {
				Kind:  schemaapi.KindArray,
				Items: &schemaapi.TypeRef{Inline: &schemaapi.Type{Kind: schemaapi.KindString}},
			},
			"requires": {
				Kind: schemaapi.KindArray,
				Items: &schemaapi.TypeRef{Inline: &schemaapi.Type{
					Kind:     schemaapi.KindLink,
					Target:   "Contribution",
					LinkMode: schemaapi.LinkMany2One,
				}},
			},
		},
	}
	body := json.RawMessage(`{
		"name": "x",
		"hashtags": ["a", "b"],
		"requires": ["p:/skill/r"]
	}`)
	edges, err := core.ExtractEdges(body, mixedType)
	if err != nil {
		t.Fatalf("ExtractEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1 (only requires): %+v", len(edges), edges)
	}
	if edges[0].Relation != "requires" {
		t.Errorf("Relation = %q", edges[0].Relation)
	}
}

// TestCatalog_reindexExtractsEdges asserts the full bootstrap flow:
// (1) load a Contribution with a Schema URI into the catalog,
// (2) attach a SchemaService stub that returns the right data-type,
// (3) call Reindex, and observe Edges populated on the Contribution.
func TestCatalog_reindexExtractsEdges(t *testing.T) {
	dir := t.TempDir()
	cache := core.NewCache(dir)

	// A Contribution that points at a Schema (the data-type URI) and
	// has a `requires` array of contribution-URIs.
	body := json.RawMessage(`{
		"name": "doc-create",
		"requires": ["ktw-docs:/skill/template-engine"]
	}`)
	m := &mbapi.Manifest{
		Provider: "ktw-docs",
		Version:  "0.1.0",
		Contributions: []contribapi.Contribution{
			// The kind declares the schema; edge extraction resolves it from
			// here, the same as validation does.
			skillKind(t, "ktw-docs:/data-type/skill-body"),
			{
				Provider: "ktw-docs", Type: "skill", ID: "doc-create",
				URI:    "ktw-docs:/skill/doc-create",
				Layers: contribapi.InlineData(body),
			},
		},
	}
	core.ReHash(m)
	if err := core.SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	cat, err := core.NewCatalog(cache, core.MembersOf(cache, "ktw-docs"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	// Stub schema service: return the SkillBody-shaped data-type
	// encoded as a Schema document JSON.
	schemaDoc := &schemaapi.Schema{
		Name: "skill-body",
		Root: "SkillBody",
		Types: map[string]*schemaapi.Type{
			"SkillBody": skillBodyType(),
		},
	}
	schemaRaw, _ := json.Marshal(schemaDoc)
	stub := &reindexSchemaStub{dataTypes: map[string]json.RawMessage{
		"ktw-docs:/data-type/skill-body": schemaRaw,
	}}
	if err := cat.SetSchema(stub); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	// Fetch back via the catalog's GetContributionsByType.
	got, err := cat.GetContributionsByType(context.Background(), "skill")
	if err != nil {
		t.Fatalf("GetContributionsByType: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d Contributions, want 1", len(got))
	}
	// Edges are DERIVED, so they are read from the aggregator's index rather
	// than from the contribution — the contribution no longer carries them.
	edges := cat.EdgesOf(got[0].ResolvedURI())
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge after Reindex, got %d: %+v", len(edges), edges)
	}
	if edges[0].Target != "ktw-docs:/skill/template-engine" {
		t.Errorf("edge Target = %q", edges[0].Target)
	}
	if edges[0].Relation != "requires" {
		t.Errorf("edge Relation = %q", edges[0].Relation)
	}
	// And the reverse question is answerable from the same index.
	if back := cat.EdgesTo("ktw-docs:/skill/template-engine", "requires"); len(back) != 1 {
		t.Errorf("EdgesTo = %+v, want 1 edge", back)
	}
}

// reindexSchemaStub is the minimal mbapi.CatalogSchemaService impl used
// by the reindex-flow test. Returns hardcoded data-type JSON for known
// URIs; everything else returns empty + nil (the "not owned by this
// sidecar" sentinel).
type reindexSchemaStub struct {
	dataTypes map[string]json.RawMessage
}

func (s *reindexSchemaStub) GetDataTypeByURI(_ context.Context, uri string) (json.RawMessage, error) {
	if raw, ok := s.dataTypes[uri]; ok {
		return raw, nil
	}
	return nil, nil
}

func (s *reindexSchemaStub) ValidateBody(_ context.Context, _ string, _ json.RawMessage) error {
	return nil
}

// TestContributionEdge_jsonRoundTrip asserts the edge value-type
// preserves all three fields across JSON round-trip (matters for the
// canonical-JSON hash invariant on Manifest).
func TestContributionEdge_jsonRoundTrip(t *testing.T) {
	e := contribapi.ContributionEdge{
		Relation: "requires",
		Target:   "p:/skill/r",
		Source:   "requires[0]",
	}
	bs, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round contribapi.ContributionEdge
	if err := json.Unmarshal(bs, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round != e {
		t.Errorf("round-trip diff: %+v != %+v", round, e)
	}
}

// skillKind declares `skill` with the given schema, so an entry of that kind
// resolves to it.
func skillKind(t *testing.T, schema string) contribapi.Contribution {
	t.Helper()
	c := contribapi.Contribution{Provider: "ktw-docs", Type: "contribution-type", ID: "skill"}
	c.SetData([]byte(`{"name":"skill","schema":"` + schema + `"}`))
	return c
}
