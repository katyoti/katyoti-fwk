// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

const validSkillMD = `---
name: doc-create
description: Create documents in various formats
requires:
  - workspace
  - skill:template-engine
hashtags:
  - documents
category: writing
provides:
  - task
  - cli
---
# Doc Create

Detailed instructions for creating documents.

Use templates, formats, and styles.
`

// hiddenSkillMD opts out of visibility via `advertised: false`. The skill
// is still callable by name (via `requires:` from a sibling, or via
// direct `skills-get-instruction`); the flag only controls list /
// `<skills_system>` block visibility — so it DOES appear in the
// provider's Enumerate output (the catalog is the URI-resolution
// surface). The list-time filter lives in `api.SkillsFromCatalog`.
const hiddenSkillMD = `---
name: hidden-skill
description: Hidden from list, still callable by URI
advertised: false
---
Some body.
`

const malformedSkillMD = `not a frontmatter at all
this should fail to parse
`

func TestSkillProvider_Enumerate(t *testing.T) {
	fs := fstest.MapFS{
		"embed/skills/doc-create/SKILL.md": &fstest.MapFile{Data: []byte(validSkillMD)},
		"embed/skills/hidden/SKILL.md":     &fstest.MapFile{Data: []byte(hiddenSkillMD)},
		"embed/skills/broken.md":           &fstest.MapFile{Data: []byte(malformedSkillMD)},
		"embed/skills/README.txt":          &fstest.MapFile{Data: []byte("not a skill")},
	}
	p := SkillProvider("ktw-test", loader.NewEmbedFSLoader(fs, ""), "embed/skills")

	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (doc-create + hidden-skill; broken + readme skipped); entries: %+v", len(entries), entries)
	}

	byID := map[string]contribapi.Contribution{}
	for _, e := range entries {
		byID[e.ID] = e
	}

	e, ok := byID["doc-create"]
	if !ok {
		t.Fatalf("missing doc-create entry; got IDs: %v", byID)
	}
	if e.Provider != "ktw-test" {
		t.Errorf("Provider = %q, want ktw-test", e.Provider)
	}
	if e.Type != mbapi.ResourceTypeSkill {
		t.Errorf("Type = %q, want %q", e.Type, mbapi.ResourceTypeSkill)
	}
	var body mbapi.SkillBody
	if err := json.Unmarshal(e.Data(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// The frontmatter is the data layer, exactly; the picker fields are
	// written THERE once. The kind declares `meta.from: data`, so the generic
	// reader derives meta from the data layer (contribapi.ApplyMetaFrom, the
	// same derivation the builder runs): meta carries exactly what the data
	// says, never a field of its own.
	if body.Name != "doc-create" || body.Description != "Create documents in various formats" {
		t.Errorf("data layer lacks the frontmatter identity: %+v", body)
	}
	kinds, err := builtin.Kinds()
	if err != nil {
		t.Fatalf("built-in kinds: %v", err)
	}
	if d := contribapi.MetaDisagreements(e, kinds); len(d) > 0 {
		t.Errorf("meta disagrees with the data layer it derives from: %v", d)
	}
	if e.Metadata == nil || e.Metadata.Name != body.Name || e.Metadata.Description != body.Description ||
		e.Metadata.Category != "writing" || len(e.Metadata.Hashtags) != 1 || e.Metadata.Hashtags[0] != "documents" ||
		e.Metadata.MediaType != "text/markdown" {
		t.Errorf("meta must be the derivation of the data layer (name, description, category, hashtags) plus the media type: %+v", e.Metadata)
	}
	// Three layers: data (frontmatter), instructions (body), source (file).
	if _, ok := e.Layers[mbapi.LayerInstructions]; !ok {
		t.Errorf("no instructions layer: layers = %v", e.Layers)
	} else if instr := string(e.Layers[mbapi.LayerInstructions].Inline); !strings.HasPrefix(instr, "#") || strings.Contains(instr, "---") {
		t.Errorf("instructions layer must be the body after the frontmatter, got %q", instr)
	}
	if ref, ok := e.SourceRef(); !ok || ref.Hash != contribapi.HashContent([]byte(validSkillMD)) {
		t.Errorf("source layer must be the file as written: %+v ok=%v", ref, ok)
	}
	if len(body.Requires) != 2 || body.Requires[0] != "workspace" {
		t.Errorf("Body.Requires = %v, want [workspace skill:template-engine]", body.Requires)
	}
	if len(body.Provides) != 2 {
		t.Errorf("Body.Provides = %v, want [task cli]", body.Provides)
	}

	// Hidden skill MUST appear in the catalog — visibility-filtering
	// happens at list time in `api.SkillsFromCatalog`, not here.
	hidden, ok := byID["hidden-skill"]
	if !ok {
		t.Fatalf("missing hidden-skill entry; advertised:false skills must enter the catalog so URI lookups resolve; got IDs: %v", byID)
	}
	// Visibility is a frontmatter field, so it is in the data layer with the
	// rest of them; the builder derives meta.advertised from it.
	var hiddenBody mbapi.SkillBody
	if err := json.Unmarshal(hidden.Data(), &hiddenBody); err != nil {
		t.Fatalf("decode hidden body: %v", err)
	}
	if hiddenBody.Advertised == nil || *hiddenBody.Advertised {
		t.Error("hidden-skill is advertised in its data layer; advertised:false must survive into the catalog so the list-time filter can hide it")
	}
	if hidden.Metadata == nil || hidden.Metadata.Advertised == nil || *hidden.Metadata.Advertised {
		t.Errorf("hidden-skill's meta must derive advertised:false from its data layer: %+v", hidden.Metadata)
	}
}

func TestSkillProvider_Open(t *testing.T) {
	fs := fstest.MapFS{
		"embed/skills/doc-create/SKILL.md": &fstest.MapFile{Data: []byte(validSkillMD)},
	}
	p := SkillProvider("ktw-test", loader.NewEmbedFSLoader(fs, ""), "embed/skills")

	rc, ref, err := p.Open(context.Background(), "doc-create", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if ref.MediaType != "text/markdown" {
		t.Errorf("MediaType = %q, want text/markdown (source = raw markdown file)", ref.MediaType)
	}
	// The source layer is the RAW SKILL.md — the parsed SkillBody is the data
	// layer, not the file.
	if string(b) != validSkillMD {
		t.Errorf("Open(source) returned %d bytes; want the raw SKILL.md (%d bytes)", len(b), len(validSkillMD))
	}

	var notFound contribapi.ErrContributionNotFound
	if _, _, err := p.Open(context.Background(), "nonexistent", contribapi.LayerSource); !errors.As(err, &notFound) {
		t.Errorf("Open(nonexistent): want ErrContributionNotFound, got %v", err)
	}

	providertest.Run(t, p)
}

func TestSkillProvider_TypeAndProvider(t *testing.T) {
	p := SkillProvider("x", loader.NewEmbedFSLoader(fstest.MapFS{}, ""), "embed/skills")
	if p.Type() != mbapi.ResourceTypeSkill {
		t.Errorf("Type() = %q", p.Type())
	}
	if p.Provider() != "x" {
		t.Errorf("Provider() = %q", p.Provider())
	}
}

func TestSkillProvider_NilLoaderErrors(t *testing.T) {
	p := SkillProvider("x", nil, "embed/skills")
	_, err := p.Enumerate(context.Background())
	if err == nil {
		t.Error("Enumerate with nil Loader should error")
	}
}

func TestSkillProvider_EmptyRootErrors(t *testing.T) {
	p := SkillProvider("x", loader.NewEmbedFSLoader(fstest.MapFS{}, ""), "")
	_, err := p.Enumerate(context.Background())
	if err == nil {
		t.Error("Enumerate with empty Root should error")
	}
}
