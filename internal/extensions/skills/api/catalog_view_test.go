// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/refmatch"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func TestSkillInfoFromCatalog(t *testing.T) {
	// Picker fields from meta, wiring fields from the body — the split the
	// two are for.
	meta := contribapi.ContributionMeta{
		Name:        "doc-create",
		Description: "Create documents",
		Icon:        contribapi.NewIcon("file", "📄"),
		Hashtags:    []string{"writing"},
		Category:    "docs",
	}
	body := mbapi.SkillBody{Requires: []string{"workspace"}, Provides: []string{"task"}}
	info := SkillInfoFromCatalog("ktw-docs", meta, body)
	if info.Name != "doc-create" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Provider != "ktw-docs" {
		t.Errorf("Provider = %q", info.Provider)
	}
	if info.Description != "Create documents" {
		t.Errorf("Description = %q", info.Description)
	}
	if info.URI() != "ktw-docs:/skill/doc-create" {
		t.Errorf("URI = %q, want ktw-docs:/skill/doc-create", info.URI())
	}
	if len(info.Requires) != 1 || info.Requires[0] != "workspace" {
		t.Errorf("Requires = %v", info.Requires)
	}
	if info.Icon != meta.Icon.Emoji || info.IconName != meta.Icon.Tabler {
		t.Errorf("Icon/IconName mismatch")
	}
	if info.Category != "docs" {
		t.Errorf("Category = %q", info.Category)
	}
	if len(info.Provides) != 1 || info.Provides[0] != "task" {
		t.Errorf("Provides = %v", info.Provides)
	}
}

// fakeCatalog: in-test mbapi.Catalog with hand-rolled manifests.
type fakeCatalog struct {
	manifests map[string]*mbapi.Manifest
}

func (c *fakeCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	return c.manifests[name], nil
}

func (c *fakeCatalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	out := make([]*mbapi.Manifest, 0, len(c.manifests))
	for _, m := range c.manifests {
		out = append(out, m)
	}
	return out, nil
}

func (c *fakeCatalog) Names(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(c.manifests))
	for n := range c.manifests {
		out = append(out, n)
	}
	return out, nil
}

func (c *fakeCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, m := range c.manifests {
		for _, e := range m.Contributions {
			if e.Type == t {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func (c *fakeCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	pu, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, err
	}
	for _, m := range c.manifests {
		for _, e := range m.Contributions {
			prov := e.Provider
			if prov == "" {
				prov = m.Provider
			}
			if e.Type == pu.Type && e.ID == pu.ID && prov == pu.Provider {
				return e.Data(), nil
			}
		}
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// skillEntry builds a catalog entry the way the production provider does:
// picker fields in meta, wiring fields in the body. Tests that used to write
// both into one struct now have to say which is which, which is the point.
func skillEntry(provider, id, description string, advertised *bool) contribapi.Contribution {
	c := contribapi.Contribution{
		Provider: provider, Type: mbapi.ResourceTypeSkill, ID: id,
		Metadata: &contribapi.ContributionMeta{
			Name: id, Description: description, Advertised: advertised,
		},
	}
	c.SetData([]byte(`{}`))
	return c
}

func TestSkillsFromCatalog(t *testing.T) {
	cat := &fakeCatalog{
		manifests: map[string]*mbapi.Manifest{
			"ktw-x": {
				Provider: "ktw-x",
				Contributions: []contribapi.Contribution{
					skillEntry("ktw-x", "skill-a", "A", nil),
				},
			},
			"ktw-y": {
				Provider: "ktw-y",
				Contributions: []contribapi.Contribution{
					skillEntry("ktw-y", "skill-b", "B", nil),
					{Provider: "ktw-y", Type: "task", ID: "task-z"},
				},
			},
		},
	}

	out, err := SkillsFromCatalog(context.Background(), cat)
	if err != nil {
		t.Fatalf("SkillsFromCatalog: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d skills, want 2 (task-z is type=task, should be excluded); %+v", len(out), out)
	}

	// Verify both skills present (order is non-deterministic over map iteration).
	var sawA, sawB bool
	for _, s := range out {
		if s.URI() == "ktw-x:/skill/skill-a" {
			sawA = true
		}
		if s.URI() == "ktw-y:/skill/skill-b" {
			sawB = true
		}
	}
	if !sawA {
		t.Error("missing ktw-x:/skill/skill-a")
	}
	if !sawB {
		t.Error("missing ktw-y:/skill/skill-b")
	}
}

func TestSkillsFromCatalog_DedupsAcrossManifests(t *testing.T) {
	// The same skill (same provider+name) carried by two manifests must be
	// listed ONCE — the bug that put render-data-as-ui in the prompt 3x.
	cat := &fakeCatalog{
		manifests: map[string]*mbapi.Manifest{
			"ktw-methods": {Provider: "ktw-methods", Contributions: []contribapi.Contribution{
				skillEntry("ktw-methods", "render-data-as-ui", "render", nil),
			}},
			"ktw-agents": {Provider: "ktw-agents", Contributions: []contribapi.Contribution{
				skillEntry("ktw-methods", "render-data-as-ui", "render", nil),
			}},
		},
	}
	out, err := SkillsFromCatalog(context.Background(), cat)
	if err != nil {
		t.Fatalf("SkillsFromCatalog: %v", err)
	}
	n := 0
	for _, s := range out {
		if s.URI() == "ktw-methods:/skill/render-data-as-ui" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("render-data-as-ui listed %d times, want 1 (dedup by URI); got %+v", n, out)
	}
}

func TestSkillsFromCatalogAll_IncludesUnadvertisedWithFlag(t *testing.T) {
	no := false
	cat := &fakeCatalog{
		manifests: map[string]*mbapi.Manifest{
			"ktw-x": {Provider: "ktw-x", Contributions: []contribapi.Contribution{
				skillEntry("ktw-x", "shown", "d", nil),
				skillEntry("ktw-x", "hidden", "d", &no),
			}},
		},
	}
	// Advertised-only view: hidden excluded.
	adv, _ := SkillsFromCatalog(context.Background(), cat)
	if len(adv) != 1 {
		t.Fatalf("SkillsFromCatalog got %d, want 1 (hidden filtered)", len(adv))
	}
	// Full view: both present, with real flags — so an exact `skills:[hidden]`
	// selector can reach it while `*` (bulk) cannot.
	all, _ := SkillsFromCatalogAll(context.Background(), cat)
	flags := map[string]bool{}
	for _, s := range all {
		flags[s.Name] = s.Advertised
	}
	if len(all) != 2 || flags["shown"] != true || flags["hidden"] != false {
		t.Fatalf("SkillsFromCatalogAll flags wrong: %+v", flags)
	}
	// refmatch over the full set: bulk `*` -> shown only; exact `hidden` -> reaches it.
	bulk := refmatch.Match(SkillItems(all), []refmatch.Selector{{Name: "*"}})
	if len(bulk) != 1 || bulk[0].Name != "shown" {
		t.Errorf("bulk `*` must match advertised only; got %+v", bulk)
	}
	exact := refmatch.Match(SkillItems(all), []refmatch.Selector{ParseSkillSelector("hidden")})
	if len(exact) != 1 || exact[0].Name != "hidden" {
		t.Errorf("exact name must reach the unadvertised skill; got %+v", exact)
	}
}

func TestSkillsFromCatalog_HidesUnadvertised(t *testing.T) {
	no := false
	yes := true
	cat := &fakeCatalog{
		manifests: map[string]*mbapi.Manifest{
			"ktw-x": {
				Provider: "ktw-x",
				Contributions: []contribapi.Contribution{
					skillEntry("ktw-x", "visible", "shown", nil),
					skillEntry("ktw-x", "hidden", "callable but not listed", &no),
					skillEntry("ktw-x", "explicit-yes", "explicit true", &yes),
				},
			},
		},
	}

	out, err := SkillsFromCatalog(context.Background(), cat)
	if err != nil {
		t.Fatalf("SkillsFromCatalog: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d skills, want 2 (visible + explicit-yes; hidden filtered); %+v", len(out), out)
	}
	for _, s := range out {
		if s.Name == "hidden" {
			t.Errorf("unadvertised skill leaked into list output: %+v", s)
		}
	}

	// Critical invariant: the hidden skill MUST still be resolvable by
	// canonical URI — only the LIST surface filters. Resolved through the
	// addressed read rather than through instructions, because visibility is
	// what this test is about and the instructions now live in a layer.
	got, err := mbapi.GetOne[mbapi.SkillBody](context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, "ktw-x:/skill/hidden")
	if err != nil {
		t.Errorf("hidden skill must resolve by URI even though filtered from list: %v", err)
	}
	// The typed view carries the entry's meta, so a consumer that resolved one
	// contribution can say which one without a second lookup.
	if got.Meta.Name != "hidden" {
		t.Errorf("resolved %q, want hidden", got.Meta.Name)
	}
	if got.URI != "ktw-x:/skill/hidden" {
		t.Errorf("resolved URI = %q", got.URI)
	}
}

func TestSkillsFromCatalog_NilCatalog(t *testing.T) {
	out, err := SkillsFromCatalog(context.Background(), nil)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil for nil catalog", out)
	}
}

func TestSkillsFromCatalog_EmptyCatalog(t *testing.T) {
	cat := &fakeCatalog{manifests: map[string]*mbapi.Manifest{}}
	out, err := SkillsFromCatalog(context.Background(), cat)
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(out) != 0 {
		t.Errorf("out = %v, want empty for empty catalog", out)
	}
}

func TestInstructionsFromCatalog(t *testing.T) {
	// The instructions are the skill's SOURCE layer now, not a body field —
	// so the catalog must serve that layer, exactly as a real one does.
	const skillMD = `---
name: doc-create
description: Create documents
---

# Doc Create

Full L1 body text here.`
	const wantInstructions = `# Doc Create

Full L1 body text here.`
	cat := &layerCatalog{
		fakeCatalog: fakeCatalog{
			manifests: map[string]*mbapi.Manifest{
				"ktw-docs": {
					Provider: "ktw-docs",
					Contributions: []contribapi.Contribution{
						{Provider: "ktw-docs", Type: mbapi.ResourceTypeSkill, ID: "doc-create",
							Layers: map[string]contribapi.LayerRef{
								contribapi.LayerData: contribapi.InlineData([]byte(`{}`))[contribapi.LayerData],
								contribapi.LayerInstructions: {
									Hash: contribapi.HashContent([]byte(wantInstructions)),
									Size: int64(len(wantInstructions)), MediaType: "text/markdown",
								},
								contribapi.LayerSource: {
									Hash: contribapi.HashContent([]byte(skillMD)),
									Size: int64(len(skillMD)), MediaType: "text/markdown",
								},
							}},
					},
				},
			},
		},
		// The fake serves the INSTRUCTIONS layer: the body the producer split
		// from the file. The consumer never sees a fence.
		source: map[string]string{"ktw-docs:/skill/doc-create": wantInstructions},
	}

	// Happy path: the instructions layer IS the L1 body; nothing is stripped here.
	got, err := InstructionsFromCatalog(context.Background(), cat, "ktw-docs:/skill/doc-create")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != wantInstructions {
		t.Errorf("instructions = %q", got)
	}

	// Unknown provider → ErrSkillNotInCatalog
	_, err = InstructionsFromCatalog(context.Background(), cat, "ktw-nope:/skill/foo")
	if !errors.Is(err, ErrSkillNotInCatalog) {
		t.Errorf("unknown provider: err = %v, want ErrSkillNotInCatalog", err)
	}
}

// layerCatalog serves a source layer, the way a real catalog does. Without it
// Open falls back to the body, which no longer carries the markdown.
type layerCatalog struct {
	fakeCatalog
	source map[string]string
}

func (c *layerCatalog) OpenLayer(_ context.Context, uri, _ string) (io.ReadCloser, contribapi.LayerRef, error) {
	raw, ok := c.source[uri]
	if !ok {
		return nil, contribapi.LayerRef{}, mbapi.ErrSidecarNotInCatalog
	}
	return io.NopCloser(strings.NewReader(raw)), contribapi.LayerRef{
		Hash: contribapi.HashContent([]byte(raw)), Size: int64(len(raw)),
	}, nil
}

// Two-phase bootstrap protocol stubs (hub-boot-order ADR).
func (*fakeCatalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }
func (*fakeCatalog) Reindex(_ context.Context, _ []string) error  { return nil }
func (*fakeCatalog) MarkReady(_ context.Context) error            { return nil }
func (*fakeCatalog) Ready() bool                                  { return true }
