// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"regexp"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// ModuleCreate is the convenience verb that authors a `module` node
// + stamps the gocode-specific type_specific fields in one shot. The
// alternative would be `cwm node-create --type module` followed by
// `cwm node-update --type-specific '{…}'`; this collapses that into
// a single call and ensures the gocode-required fields are present.
//
// Filesystem scaffolding (`Scaffold=true` creates the public /
// internal / local directory tree on disk) is deferred — that
// crosses into Phase F territory (`go-ast-discovery` owns the
// disk-writing concerns). v1 sets the field but doesn't act on it.
func (s *GocodeServiceImpl) ModuleCreate(ctx stdctx.Context, in api.ModuleCreateInput) (api.ModuleCreateOutput, error) {
	if in.Name == "" {
		return api.ModuleCreateOutput{}, fmt.Errorf("%w: ModuleCreate: name is required", api.ErrInvalidInput)
	}
	if in.ProjectID == "" {
		return api.ModuleCreateOutput{}, fmt.Errorf("%w: ModuleCreate: project_id is required", api.ErrInvalidInput)
	}
	if in.Kind == "" {
		return api.ModuleCreateOutput{}, fmt.Errorf("%w: ModuleCreate: kind is required (framework | library | sidecar)", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleCreateOutput{}, fmt.Errorf("ModuleCreate: resolve CWMService: %w", err)
	}

	tierTarget := in.TierTarget
	if tierTarget <= 0 {
		tierTarget = 1
	}

	typeSpecific := map[string]any{
		"kind":         string(in.Kind),
		"tier_current": 0,
		"tier_target":  tierTarget,
		"tier_state":   string(api.TierStateBootstrapping),
	}
	// Code facets are written to the declared `code_packages`, the one field
	// every reader takes a module's code from; the other kinds stay pointers
	// under `facets`.
	var codePackages []any
	for _, f := range in.Facets {
		if f.Kind == api.FacetCode {
			codePackages = append(codePackages, f.Path)
		}
	}
	if len(codePackages) > 0 {
		typeSpecific[api.CodePackagesField] = codePackages
	}
	if len(in.Facets) > len(codePackages) {
		facets := make([]map[string]any, 0, len(in.Facets)-len(codePackages))
		for _, f := range in.Facets {
			if f.Kind == api.FacetCode {
				continue
			}
			entry := map[string]any{"kind": string(f.Kind), "path": f.Path}
			if f.Role != "" {
				entry["role"] = f.Role
			}
			facets = append(facets, entry)
		}
		typeSpecific["facets"] = facets
	}

	// A module object's id IS the module name — modules/<name>/objects/<name>.yaml
	// (identity collapse, 2026-09-14). Name is ALREADY the slug by contract
	// ("Module slug (lowercase, hyphens)"), so it is used verbatim: an authored
	// id must be authored, never derived. Deriving it would make identity a
	// function of a display string, which is exactly what the container schema
	// says a slug is NOT.
	slug := in.Name
	if !moduleIDRe.MatchString(slug) {
		return api.ModuleCreateOutput{}, fmt.Errorf(
			"%w: ModuleCreate: name %q must be a module slug (lowercase letters, digits, hyphens)", api.ErrInvalidInput, in.Name)
	}
	// gocode creates software modules: the module level and the
	// code capability its type carries.
	obj := cwmapi.NewContainerObject(slug, slug, "software-module", in.ProjectID, in.Name, in.Description)
	for k, v := range typeSpecific {
		// TypeSpecific dissolved INTO capabilities: the cap:* keys were always
		// field-groups, and the rest stays as a plain group under the type name.
		if group, ok := v.(map[string]any); ok {
			if obj.Capabilities == nil {
				obj.Capabilities = map[string]map[string]any{}
			}
			obj.Capabilities[k] = group
			continue
		}
		if obj.Capabilities == nil {
			obj.Capabilities = map[string]map[string]any{}
		}
		if obj.Capabilities[codeCapability] == nil {
			obj.Capabilities[codeCapability] = map[string]any{}
		}
		obj.Capabilities[codeCapability][k] = v
	}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: slug, Object: obj})
	if err != nil {
		return api.ModuleCreateOutput{}, fmt.Errorf("ModuleCreate: ObjectCreate: %w", err)
	}
	container := created.Object

	mod := api.Module{
		ID:          created.Object.ID,
		Name:        container.Name,
		Slug:        slug,
		Description: container.Description,
		ParentID:    container.Parent,
		Kind:        in.Kind,
		Facets:      in.Facets,
		TierCurrent: 0,
		TierTarget:  tierTarget,
		TierState:   api.TierStateBootstrapping,
	}
	// Scaffolding (in.Scaffold==true creating public/internal/local
	// folders on disk) stays a stub in v1 — that crosses into Phase F
	// territory where `go-ast-discovery` owns disk writes.
	return api.ModuleCreateOutput{Module: mod}, nil
}

// moduleIDRe guards the one place a module id enters the system. The id becomes
// a folder name, so anything outside this alphabet is refused here rather than
// discovered as a path bug later.
var moduleIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
