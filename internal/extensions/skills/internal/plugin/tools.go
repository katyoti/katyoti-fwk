// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// --- Typed tool input/output structs ---

// GetInstructionsInput is the input for the skills-get-instruction tool.
// Per the 2026-05-24 skill normalization, `skill_name` is the FULL
// canonical URI form `<provider>:/skill/<name>` — the short
// `provider:name` shape was retired alongside the SkillManager RPC
// drop. The catalog accepts both forms during the transition; new
// callers should always emit URIs.
type GetInstructionsInput struct {
	SkillName string `json:"skill_name" jsonschema:"Skill URI (full form: <provider>:/skill/<name>, e.g., ktw-docs:/skill/doc-create)" required:"true"`
}

// GetInstructionsOutput is the output of the skills-get-instruction tool.
type GetInstructionsOutput struct {
	Instructions string `json:"instructions" jsonschema:"The skill's full instructions"`
}

// --- Tool constructors ---

// NewGetInstructionTool creates the typed skills-get-instruction tool.
//
// Data source: `mbapi.Catalog` published by `sidecarDiscoveryExt`.
// `SkillBody.Instructions` (the L1 body text from each skill's
// SKILL.md) is shipped in the descriptor — no per-call RPC. Per
// `bus-split-aggregator-to-catalog` step 8.
func NewGetInstructionTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"skills-get-instruction",
		"Load detailed instructions for a skill. Use the full URI form `<provider>:/skill/<name>` (e.g., `ktw-docs:/skill/doc-create`).",
		func(ctx context.Context, in GetInstructionsInput) (GetInstructionsOutput, error) {
			cat, err := application.UseService[mbapi.Catalog](ctx)
			if err != nil || cat == nil {
				return GetInstructionsOutput{}, fmt.Errorf("resolve Catalog: %w", err)
			}
			instructions, err := api.InstructionsFromCatalog(ctx, cat, in.SkillName)
			if err != nil {
				return GetInstructionsOutput{}, err
			}
			return GetInstructionsOutput{Instructions: instructions}, nil
		},
		toolsapi.WithReadOnly(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📘", Label: "Skill", Color: "#C792EA", Template: "{{.skill_name}}"}),
	)
}

// ReadResourceInput is the input for the read-resource tool.
type ReadResourceInput struct {
	URI   string `json:"uri" jsonschema:"Resource URI <provider>:/<type>/<id> (e.g., ktw-docs:/reference/rich-markdown.md). For a skill's followable procedure use skills-get-instruction instead." required:"true"`
	Layer string `json:"layer,omitempty" jsonschema:"Optional layer to read instead of the default content (e.g. translations for a guide recipe's texts in other languages)"`
}

// ReadResourceOutput carries the resource's raw content.
type ReadResourceOutput struct {
	Content string `json:"content" jsonschema:"The resource's raw content as authored"`
}

// NewReadResourceTool creates the typed read-resource tool — the agent-surface
// `read` verb for any NON-skill resource (reference, config, data-type, …),
// addressed by URI. It resolves the content via the in-process Catalog's content
// plane (`GetBody`), which fetches cross-sidecar bodies over the bus on a local
// miss — so an agent reads a reference by URI instead of the filesystem.
func NewReadResourceTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"read-resource",
		"Read a resource's content by URI — a reference, config, data-type, or any non-skill resource (URI form `<provider>:/<type>/<id>`). For a skill's followable instructions, use skills-get-instruction instead.",
		func(ctx context.Context, in ReadResourceInput) (ReadResourceOutput, error) {
			cat, err := application.UseService[mbapi.Catalog](ctx)
			if err != nil || cat == nil {
				return ReadResourceOutput{}, fmt.Errorf("resolve Catalog: %w", err)
			}
			if in.Layer != "" {
				l, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, in.URI, in.Layer)
				if err != nil {
					return ReadResourceOutput{}, fmt.Errorf("read %q layer %q: %w", in.URI, in.Layer, err)
				}
				b, err := l.Bytes()
				if err != nil {
					return ReadResourceOutput{}, fmt.Errorf("read %q layer %q: %w", in.URI, in.Layer, err)
				}
				return ReadResourceOutput{Content: string(b)}, nil
			}
			body, err := cat.GetBody(ctx, in.URI)
			if err != nil {
				return ReadResourceOutput{}, fmt.Errorf("read %q: %w", in.URI, err)
			}
			return ReadResourceOutput{Content: string(body)}, nil
		},
		toolsapi.WithReadOnly(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📄", Label: "Read", Color: "#80CBC4", Template: "{{.uri}}"}),
	)
}

// ListResourcesInput is the input for the list-resources tool.
type ListResourcesInput struct {
	Kind  string `json:"kind" jsonschema:"Resource kind to list (e.g. guide-recipe, skill, task, reference)" required:"true"`
	Words string `json:"words,omitempty" jsonschema:"Search words; a resource matches when its name, description or hashtags contain them. Empty lists every resource of the kind"`
}

// ResourceHit is one resource list-resources found.
type ResourceHit struct {
	URI         string   `json:"uri"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Hashtags    []string `json:"hashtags,omitempty"`
}

// ListResourcesOutput is the list-resources result, best match first.
type ListResourcesOutput struct {
	Resources []ResourceHit `json:"resources"`
}

// maxResourceHits bounds a listing handed to the model.
const maxResourceHits = 20

// NewListResourcesTool creates the typed list-resources tool — the agent's
// search over the catalog by kind and words, reading only each entry's
// metadata (its derived name, description and hashtags; no body is fetched).
// It is how the agent finds the guide recipe for "how do I …?"
// (modules/guide, decision OD-013), and works for any kind.
func NewListResourcesTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"list-resources",
		"List the resources of one kind whose name, description or hashtags match some words, best match first (e.g. kind guide-recipe, words \"add capability\"). Returns URIs; read one with read-resource.",
		func(ctx context.Context, in ListResourcesInput) (ListResourcesOutput, error) {
			cat, err := application.UseService[mbapi.Catalog](ctx)
			if err != nil || cat == nil {
				return ListResourcesOutput{}, fmt.Errorf("resolve Catalog: %w", err)
			}
			entries, err := cat.GetContributionsByType(ctx, in.Kind)
			if err != nil {
				return ListResourcesOutput{}, fmt.Errorf("list %q: %w", in.Kind, err)
			}
			return ListResourcesOutput{Resources: SearchResources(entries, in.Words, maxResourceHits)}, nil
		},
		toolsapi.WithReadOnly(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🔎", Label: "Find", Color: "#80CBC4", Template: "{{.kind}} {{.words}}"}),
	)
}

// SearchResources ranks entries by how many of the words their metadata
// contains (case-insensitive; a word must appear in the name, the description
// or a hashtag), keeping only entries matching every word. No words keeps
// every entry. Ties keep URI order.
func SearchResources(entries []contribapi.Contribution, words string, limit int) []ResourceHit {
	terms := strings.Fields(strings.ToLower(words))
	type scored struct {
		hit   ResourceHit
		score int
	}
	var found []scored
	for _, c := range entries {
		m := c.Metadata
		if m == nil {
			m = &contribapi.ContributionMeta{}
		}
		name := strings.ToLower(m.Name)
		desc := strings.ToLower(m.Description)
		tags := strings.ToLower(strings.Join(m.Hashtags, " "))
		score, all := 0, true
		for _, w := range terms {
			switch {
			case strings.Contains(tags, w):
				score += 3
			case strings.Contains(name, w):
				score += 2
			case strings.Contains(desc, w):
				score++
			default:
				all = false
			}
		}
		if !all {
			continue
		}
		found = append(found, scored{ResourceHit{URI: c.ResolvedURI(), Name: m.Name, Description: m.Description, Hashtags: m.Hashtags}, score})
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}
		return found[i].hit.URI < found[j].hit.URI
	})
	out := make([]ResourceHit, 0, len(found))
	for i, f := range found {
		if limit > 0 && i >= limit {
			break
		}
		out = append(out, f.hit)
	}
	return out
}

// SkillTools returns all typed skill tools the agent plugin exposes:
// `skills-get-instruction` (a skill's followable form) + `read-resource` (any
// other resource's raw content by URI) — the agent-surface bindings of the
// canonical `get-instruction` and `read` verbs.
func SkillTools() []toolsapi.Tool {
	return []toolsapi.Tool{
		NewGetInstructionTool(),
		NewReadResourceTool(),
		NewListResourcesTool(),
	}
}
