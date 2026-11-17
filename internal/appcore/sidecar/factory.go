// SPDX-License-Identifier: Apache-2.0

// Package sidecar provides the sidecar contract services.
// This file exposes factory functions that create the default implementations.
// The impls live in internal/ (hidden); these factories are the public surface.
package sidecar

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	scinternal "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	skillsapi "github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// NewSidecar creates the default Sidecar service (identity + introspection).
func NewSidecar(appName, version, description string) api.Sidecar {
	return scinternal.NewDefaultSidecar(scinternal.SidecarIdentity{
		AppName:     appName,
		Version:     version,
		Description: description,
	})
}

// NewConfigService creates the default ConfigService from aggregated infrastructure needs.
// Tool checking uses publish.CheckTool directly (no injection needed — CheckTool is now
// in core/publish/, importable by sidecar/internal/core without cycles).
func NewConfigService(app *application.Application, appName string, efs fs.FS) api.SidecarConfigService {
	deps := scinternal.AggregateConfigDeps(app, appName, efs)
	return scinternal.NewDefaultConfigService(deps)
}

// NewLLMService creates the default LLMService.
// registry can be nil (empty — populated later by AutoKTSPContainer).
// loader can be nil (no skills — llm info returns a fallback message).
func NewLLMService(providerID, mode string) api.LLMService {
	registry := scinternal.NewToolRegistry()
	return scinternal.NewDefaultLLMService(registry, nil, providerID, mode)
}

// NewLLMServiceFromContext creates an LLMService that wraps existing services.
// The ToolRegistry and SkillLoader are resolved lazily at call time from the
// context — this avoids ordering issues during bootstrap (the KTSP container
// may not exist yet when the LLMService factory runs).
func NewLLMServiceFromContext(ctx context.Context, providerID, mode string) api.LLMService {
	return &lazyLLMService{
		ctx:        ctx,
		providerID: providerID,
		mode:       mode,
	}
}

// lazyLLMService resolves dependencies on first call, not at creation time.
type lazyLLMService struct {
	ctx        context.Context
	providerID string
	mode       string
	inner      *scinternal.DefaultLLMService
}

func (l *lazyLLMService) ensureInit(ctx context.Context) {
	if l.inner != nil {
		return
	}
	registry := scinternal.NewToolRegistry()

	// Populate tool registry from KTSP container
	if endpoint, err := application.UseNamedService[*ktsp.Endpoint](ctx, "default"); err == nil {
		populateRegistryFromEndpoint(registry, endpoint, l.providerID)
	}

	// Resolve Catalog for `llm info`'s skill enumeration (post step-13
	// — SkillManager no longer carries List / GetInstructions; the
	// catalog ships both layers in each binary's descriptor).
	var loader scinternal.SkillLoader
	if cat, err := application.UseService[mbapi.Catalog](ctx); err == nil && cat != nil {
		loader = &catalogSkillAdapter{cat: cat, provider: l.providerID}
	}

	l.inner = scinternal.NewDefaultLLMService(registry, loader, l.providerID, l.mode)
}

func (l *lazyLLMService) Info(ctx context.Context) (api.LLMInfoOutput, error) {
	l.ensureInit(ctx)
	return l.inner.Info(ctx)
}
func (l *lazyLLMService) CallTool(ctx context.Context, in api.LLMCallToolInput) (api.LLMCallToolOutput, error) {
	l.ensureInit(ctx)
	return l.inner.CallTool(ctx, in)
}
func (l *lazyLLMService) ListTools(ctx context.Context, in api.LLMListToolsInput) (api.LLMListToolsOutput, error) {
	l.ensureInit(ctx)
	return l.inner.ListTools(ctx, in)
}
func (l *lazyLLMService) DescribeTool(ctx context.Context, in api.LLMDescribeToolInput) (api.LLMDescribeToolOutput, error) {
	l.ensureInit(ctx)
	return l.inner.DescribeTool(ctx, in)
}
func (l *lazyLLMService) ListResources(ctx context.Context, in api.LLMListResourcesInput) (api.LLMListResourcesOutput, error) {
	l.ensureInit(ctx)
	return l.inner.ListResources(ctx, in)
}
func (l *lazyLLMService) GetResource(ctx context.Context, in api.LLMGetResourceInput) (api.LLMGetResourceOutput, error) {
	l.ensureInit(ctx)
	return l.inner.GetResource(ctx, in)
}
func (l *lazyLLMService) ReadResource(ctx context.Context, in api.LLMReadResourceInput) (api.LLMReadResourceOutput, error) {
	l.ensureInit(ctx)
	return l.inner.ReadResource(ctx, in)
}
func (l *lazyLLMService) ListResourceTypes(ctx context.Context) (api.LLMListResourceTypesOutput, error) {
	l.ensureInit(ctx)
	return l.inner.ListResourceTypes(ctx)
}

// skipSlots lists slot names that should NOT be registered as tools
// (they are the LLMService itself + infrastructure services).
var skipSlots = map[string]bool{
	"llm":     true,
	"sidecar": true,
	"config":  true,
}

// populateRegistryFromEndpoint adds all KTSP methods as tool entries.
// Tool names follow the canonical Tool URI form locked in the
// the command-surface decision: `<provider>:<slot>/<method-kebab>` (default
// instance — parens omitted). Examples:
//
//	ktw-weather:weather-service/get-current
//	ktw-example:example-service/search
//
// Using URIs as the registry key makes `llm tools call <uri>` round-trip
// cleanly with `llm tools list` / `llm tools describe` and matches every
// other surface in the ecosystem (KTSP subjects, MCP resource IDs,
// `<sidecar_system>` skill IDs).
func populateRegistryFromEndpoint(registry *scinternal.ToolRegistry, endpoint *ktsp.Endpoint, providerID string) {
	info := endpoint.Info()
	for _, slot := range info.Slots {
		// Skip internal/hidden/infrastructure slots
		if slot.Tags["internal"] == "true" || skipSlots[slot.Name] {
			continue
		}
		for _, method := range slot.Methods {
			// A method kept off the LLM surface (meta.MethodDef.NoLLM → mcp=false)
			// is a person's act: not listed, not callable through `llm tools`.
			if method.Tags[ktsp.TagMethodCLI] == "false" || method.Tags[ktsp.TagMethodMCP] == "false" {
				continue
			}

			// Canonical Tool URI: `<provider>:<slot>/<method-kebab>`.
			toolName := providerID + ":" + slot.Name + "/" + corereflect.ToKebab(method.Name)

			var schema json.RawMessage
			if method.InputSchema != nil {
				if data, err := json.Marshal(method.InputSchema); err == nil {
					schema = data
				}
			}

			slotName := slot.Name
			methodName := method.Name
			registry.Register(scinternal.ToolEntry{
				Name:        toolName,
				Description: method.Description,
				Schema:      schema,
				Source:      slotName + "/" + methodName,
				Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
					// Sidecar tool registry doesn't carry the instance
					// dimension — every tool dispatches to the
					// singleton DefaultInstance. CRUD-style multi-
					// instance services are reached over the bus
					// (where the subject encodes the instance), not
					// through this registry.
					return endpoint.Handle(ctx, slotName, ktsp.DefaultInstance, methodName, params)
				},
			})
		}
	}
}

// catalogSkillAdapter adapts `mbapi.Catalog` to the `SkillLoader`
// interface (`LoadSkillBody`, `ListSkills`) used by `DefaultLLMService`
// for `llm info`. Replaces the legacy `skillManagerAdapter` (deleted
// in step 13 of bus-split-aggregator-to-catalog) — SkillManager no
// longer ships List/GetInstructions, but the catalog has the same data
// (`SkillBody.Instructions` for L1, `SkillBody.Name/Description` for L0).
type catalogSkillAdapter struct {
	cat      mbapi.Catalog
	provider string
}

func (a *catalogSkillAdapter) LoadSkillBody(ctx context.Context, name string) (string, error) {
	// Accept two input forms: (a) a bare skill name (e.g. "weather-usage")
	// which we compose into the canonical full URI under the adapter's
	// provider, or (b) an already-formed URI (e.g.
	// "ktw-other:/skill/foo") which we pass through verbatim. Per the
	// 2026-05-24 skill normalization the catalog parser only accepts the
	// full `<provider>:/skill/<name>` form.
	if instr, err := skillsapi.InstructionsFromCatalog(ctx, a.cat, a.provider+":/skill/"+name); err == nil {
		return instr, nil
	}
	if instr, err := skillsapi.InstructionsFromCatalog(ctx, a.cat, name); err == nil {
		return instr, nil
	}
	return "", skillsapi.ErrSkillNotInCatalog
}

func (a *catalogSkillAdapter) ListSkills(ctx context.Context) ([]scinternal.SkillSummary, error) {
	skills, err := skillsapi.SkillsFromCatalog(ctx, a.cat)
	if err != nil {
		return nil, err
	}
	result := make([]scinternal.SkillSummary, len(skills))
	for i, s := range skills {
		result[i] = scinternal.SkillSummary{
			Name:        s.URI(),
			Description: s.Description,
		}
	}
	return result, nil
}

// AggregatePublishedServices collects all PublishedService entries from the EM.
// Convenience wrapper around application.UseAppExtensions.
func AggregatePublishedServices(app *application.Application) []publish.PublishedService {
	return application.UseAppExtensions[publish.PublishedService](app)
}
