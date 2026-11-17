// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ToolEntry maps a tool name to its handler and metadata.
type ToolEntry struct {
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema for input params
	Source      string          // e.g., "docs/Render" or "scripts/process-data"
	Handler     func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)
}

// ToolRegistry is the per-sidecar tool namespace.
// Maps tool name â†’ entry. Built at bootstrap from PublishedService reflection
// + ScriptService entries.
type ToolRegistry struct {
	tools map[string]ToolEntry
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]ToolEntry)}
}

// Register adds a tool to the registry. Panics on name collision.
func (r *ToolRegistry) Register(entry ToolEntry) {
	if _, exists := r.tools[entry.Name]; exists {
		panic(fmt.Sprintf("tool name collision: %q already registered", entry.Name))
	}
	r.tools[entry.Name] = entry
}

// Get returns a tool entry by name.
func (r *ToolRegistry) Get(name string) (ToolEntry, bool) {
	e, ok := r.tools[name]
	return e, ok
}

// All returns all tool entries sorted by name.
func (r *ToolRegistry) All() []ToolEntry {
	entries := make([]ToolEntry, 0, len(r.tools))
	for _, e := range r.tools {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// SkillLoader is the minimal interface for loading skills.
// Implemented by SkillManager adapter — avoids importing the full skills package.
type SkillLoader interface {
	LoadSkillBody(ctx context.Context, name string) (string, error)
	ListSkills(ctx context.Context) ([]SkillSummary, error)
}

// SkillSummary is a minimal skill entry for the sidecar system message.
type SkillSummary struct {
	Name        string
	Description string
}

// DefaultLLMService implements the LLM-facing interface.
type DefaultLLMService struct {
	Registry    *ToolRegistry
	SkillLoader SkillLoader
	ProviderID  string // sidecar name for URI construction
	Mode        string // "agent" or "cli" — determines header
}

// NewDefaultLLMService creates a DefaultLLMService.
func NewDefaultLLMService(registry *ToolRegistry, loader SkillLoader, providerID, mode string) *DefaultLLMService {
	return &DefaultLLMService{
		Registry:    registry,
		SkillLoader: loader,
		ProviderID:  providerID,
		Mode:        mode,
	}
}

// Info builds the <sidecar_system> block with identity, calling conventions,
// tool names, skill names, and the usage skill body.
func (s *DefaultLLMService) Info(ctx context.Context) (api.LLMInfoOutput, error) {
	bin := s.ProviderID
	var sb strings.Builder

	sb.WriteString("<sidecar_system>\n\n")

	// Identity
	sb.WriteString("## Identity\n")
	sb.WriteString(fmt.Sprintf("%s\n\n", bin))

	// Calling conventions — LLM-only surface. ONE way to do each thing
	// (no CLI alternatives — the LLM drives this binary purely via the
	// `llm` verbs).
	sb.WriteString("## Addressing\n")
	sb.WriteString("Everything is addressed by URI; the byte after `:` tells the kind: ")
	sb.WriteString("**resource** (read) `<provider>:/<type>/<id>`, **tool** (call) `<provider>:<slot>/<method>`, **event** (subscribe) `<provider>:!<scope>/<path>`. ")
	sb.WriteString("A task is a tool (`<provider>:tasks/<name>`) — call a short one directly; dispatch long-running work in the background and poll its status.\n\n")

	sb.WriteString("## Calling Conventions\n")
	sb.WriteString("Every `llm` verb emits JSON. One verb per outcome:\n\n")
	sb.WriteString(fmt.Sprintf("    %s llm tools list                    -> tool inventory (name + description)\n", bin))
	sb.WriteString(fmt.Sprintf("    %s llm tools describe <tool-uri>     -> one tool's full input schema\n", bin))
	sb.WriteString(fmt.Sprintf("    %s llm tools call <tool-uri> '<json>' -> invoke the tool\n", bin))
	sb.WriteString(fmt.Sprintf("    %s llm resources list --type <t>     -> resources of one type (omit --type for all; `resources types` lists the types)\n", bin))
	sb.WriteString(fmt.Sprintf("    %s llm resources read <uri>          -> one resource's content — works for EVERY type: skills, references, configs, … (`--into <file>` saves to disk instead of printing)\n\n", bin))

	sb.WriteString("MANDATORY: run `llm tools describe <tool-uri>` before the first call to a tool — never guess a tool's input shape.\n\n")

	sb.WriteString("Skills are packages of domain expertise. Load a skill: `llm resources read <provider>:/skill/<name>`. ")
	sb.WriteString("When a skill names a reference, load it: `llm resources read <provider>:/reference/<path>`. Always pass full URIs.\n\n")

	// Available tools (name + description)
	if tools := s.Registry.All(); len(tools) > 0 {
		sb.WriteString("## Available Tools\n")
		for _, t := range tools {
			desc := t.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **%s** — %s\n", t.Name, desc))
		}
		sb.WriteString("\n")
	}

	// Available skills (name + description)
	if s.SkillLoader != nil {
		if skills, err := s.SkillLoader.ListSkills(ctx); err == nil && len(skills) > 0 {
			sb.WriteString("## Available Skills\n")
			sb.WriteString("Load any skill below by its full `<provider>:/skill/<name>` URI with `llm resources read`.\n")
			for _, sk := range skills {
				desc := sk.Description
				if len(desc) > 80 {
					desc = desc[:77] + "..."
				}
				sb.WriteString(fmt.Sprintf("- **%s** — %s\n", sk.Name, desc))
			}
			sb.WriteString("\n")
		}
	}

	// Usage guide (skill body)
	if s.SkillLoader != nil {
		body, err := s.SkillLoader.LoadSkillBody(ctx, "usage")
		if err == nil && body != "" {
			sb.WriteString("## Usage Guide\n\n")
			sb.WriteString(body)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("</sidecar_system>")

	return api.LLMInfoOutput{
		Content: sb.String(),
		Mode:    s.Mode,
	}, nil
}

// CallTool dispatches a tool call by name with JSON params.
func (s *DefaultLLMService) CallTool(ctx context.Context, in api.LLMCallToolInput) (api.LLMCallToolOutput, error) {
	entry, ok := s.Registry.Get(in.ToolName)
	if !ok {
		return api.LLMCallToolOutput{
			ToolName: in.ToolName,
			Error:    fmt.Sprintf("unknown tool: %q", in.ToolName),
		}, nil
	}

	if entry.Handler == nil {
		return api.LLMCallToolOutput{
			ToolName: in.ToolName,
			Error:    fmt.Sprintf("tool %q has no handler", in.ToolName),
		}, nil
	}

	result, err := entry.Handler(ctx, in.Params)
	if err != nil {
		return api.LLMCallToolOutput{
			ToolName: in.ToolName,
			Error:    err.Error(),
		}, nil
	}

	return api.LLMCallToolOutput{
		ToolName: in.ToolName,
		Result:   result,
	}, nil
}

// ListTools returns all callable tools. Schema is included only when
// in.WithSchema=true; otherwise the field is nil on every entry to
// keep the default response small (parallels ResourceService.List
// + WithBody).
func (s *DefaultLLMService) ListTools(ctx context.Context, in api.LLMListToolsInput) (api.LLMListToolsOutput, error) {
	all := s.Registry.All()
	tools := make([]api.LLMToolEntry, len(all))
	for i, e := range all {
		tools[i] = api.LLMToolEntry{
			Name:        e.Name,
			Description: e.Description,
			Source:      e.Source,
		}
		if in.WithSchema {
			tools[i].Schema = e.Schema
		}
	}
	return api.LLMListToolsOutput{Tools: tools}, nil
}

// DescribeTool returns the full schema for one tool.
func (s *DefaultLLMService) DescribeTool(ctx context.Context, in api.LLMDescribeToolInput) (api.LLMDescribeToolOutput, error) {
	entry, ok := s.Registry.Get(in.ToolName)
	if !ok {
		return api.LLMDescribeToolOutput{}, fmt.Errorf("unknown tool: %q", in.ToolName)
	}
	return api.LLMDescribeToolOutput{
		Name:        entry.Name,
		Description: entry.Description,
		Schema:      entry.Schema,
		Source:      entry.Source,
	}, nil
}

// ListResources delegates to the binary's ResourceService. The
// resource surface is the same data that powers `<binary> resources
// list` and the catalog — exposing it through llm/list-resources just
// gives MCP-style consumers the call they expect.
//
// Resolved per-call from context (the same re-entrant pattern used
// elsewhere in this package); a missing ResourceService means this
// binary doesn't ship the manifest extension, in which case we
// return an empty list rather than erroring.
func (s *DefaultLLMService) ListResources(ctx context.Context, in api.LLMListResourcesInput) (api.LLMListResourcesOutput, error) {
	rs, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		// No ResourceService published — treat as "no resources".
		return api.LLMListResourcesOutput{Resources: []contribapi.Contribution{}}, nil
	}
	filter := mbapi.ContributionListFilter{}
	if in.Type != "" {
		filter.Types = []string{in.Type}
	}
	out, err := rs.List(ctx, mbapi.ContributionListInput{Filter: filter})
	if err != nil {
		return api.LLMListResourcesOutput{}, fmt.Errorf("list Contributions: %w", err)
	}
	// Ensure URI is populated on every entry (Contribution.URI is
	// declared `omitempty`; ResolvedURI() composes from Provider/Type/ID
	// when missing). Keeps the LLM-facing surface URI-complete.
	entries := out.Contributions
	for i := range entries {
		if entries[i].URI == "" {
			entries[i].URI = entries[i].ResolvedURI()
		}
	}
	return api.LLMListResourcesOutput{Resources: entries}, nil
}

// GetResource delegates to ResourceService.Get. Returns the body in
// either Body (typed JSON) or Bytes (raw) form depending on the
// underlying provider.
func (s *DefaultLLMService) GetResource(ctx context.Context, in api.LLMGetResourceInput) (api.LLMGetResourceOutput, error) {
	rs, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		return api.LLMGetResourceOutput{}, fmt.Errorf("ResourceService not published: %w", err)
	}
	out, err := rs.Get(ctx, mbapi.ContributionGetInput{URI: in.URI})
	if err != nil {
		return api.LLMGetResourceOutput{}, err
	}
	c := out.Contribution
	if c.URI == "" {
		c.URI = c.ResolvedURI()
	}
	return api.LLMGetResourceOutput{Contribution: c}, nil
}

// ReadResource returns a contribution's raw readable content (reference
// files, any file-backed resource) through the content lane,
// ResourceService.OpenLayer — not the compatibility Read.
func (s *DefaultLLMService) ReadResource(ctx context.Context, in api.LLMReadResourceInput) (api.LLMReadResourceOutput, error) {
	rs, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		return api.LLMReadResourceOutput{}, fmt.Errorf("ResourceService not published: %w", err)
	}
	return readResourceLayer(ctx, rs, in.URI)
}

// readResourceLayer opens the layer mbapi.ReadLayer names for the entry — the
// one statement of the read-layer rule (source when declared, else data) —
// through the content lane. The entry comes from the resource service's index
// (Get), so the rule is asked of the entry, never re-derived here; a refusal
// (a bulk-band layer, bytes the producer does not carry) comes back typed.
func readResourceLayer(ctx context.Context, rs mbapi.ResourceService, uri string) (api.LLMReadResourceOutput, error) {
	got, err := rs.Get(ctx, mbapi.ContributionGetInput{URI: uri})
	if err != nil {
		return api.LLMReadResourceOutput{}, err
	}
	layer := mbapi.ReadLayer(got.Contribution)
	out, err := rs.OpenLayer(ctx, mbapi.OpenLayerInput{URI: uri, Layer: layer})
	if err != nil {
		return api.LLMReadResourceOutput{}, err
	}
	if rerr := mbapi.RefusedErr(uri, layer, out); rerr != nil {
		return api.LLMReadResourceOutput{}, fmt.Errorf("read %s: %w", uri, rerr)
	}
	return api.LLMReadResourceOutput{
		URI:       uri,
		MediaType: out.Ref.MediaType,
		Content:   out.Content,
	}, nil
}

// ListResourceTypes delegates to ResourceService.Types. Cheap call —
// LLM clients use it for first-pass discovery before drilling into
// each type with ListResources(type).
func (s *DefaultLLMService) ListResourceTypes(ctx context.Context) (api.LLMListResourceTypesOutput, error) {
	rs, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		return api.LLMListResourceTypesOutput{Types: []string{}}, nil
	}
	types, err := rs.Types(ctx)
	if err != nil {
		return api.LLMListResourceTypesOutput{}, fmt.Errorf("list resource types: %w", err)
	}
	if types == nil {
		types = []string{}
	}
	return api.LLMListResourceTypesOutput{Types: types}, nil
}
