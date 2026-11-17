// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// ResolvePanel makes what a panel is about to draw self-contained, the same
// for every host (the GUI, a TUI, a document opened in either): a schema named
// by data-type URI is read from the universe's schema authority, one named by
// an action's input (`{action: <uri>}`) from ActionService.Describe, a `ui`
// block is laid over it (render.ResolveComponents), and the fields declaring
// an `options_from` get their choices. A field whose source fails keeps free
// text; a schema that cannot be read fails the whole display, naming it.
func (s *Service) ResolvePanel(ctx context.Context, in api.ResolvePanelInput) (api.ResolvePanelOutput, error) {
	if in.Def == nil {
		return api.ResolvePanelOutput{}, fmt.Errorf("%w: def required", cwmapi.ErrInvalidInput)
	}
	raw, err := json.Marshal(in.Def)
	if err != nil {
		return api.ResolvePanelOutput{}, fmt.Errorf("%w: def: %v", cwmapi.ErrInvalidInput, err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return api.ResolvePanelOutput{}, fmt.Errorf("%w: def: %v", cwmapi.ErrInvalidInput, err)
	}
	out, err := render.ResolveComponents(tree, render.ComponentResolver{
		Fetch: func(ref render.SchemaRef, ext, name string) (map[string]any, error) {
			return fetchSchema(ctx, ref, ext, name)
		},
		Finish: func(schema map[string]any) (map[string]any, error) { return withOptions(ctx, schema) },
	})
	if err != nil {
		return api.ResolvePanelOutput{}, err
	}
	return api.ResolvePanelOutput{Def: out}, nil
}

// ResolvePart answers one specification part (cache-miss
// half): the schema a component of the kind and variant draws, resolved
// exactly as ResolvePanel resolves it inside a page - referenced variants
// merged, options computed - with its hash, render.SpecHash. A host holding
// the current hash gets the hash alone; a stale or absent one gets the part
// and its hash, never a refusal. A part whose fields declare options_from is
// not cacheable: its choices are computed per call.
func (s *Service) ResolvePart(ctx context.Context, in api.ResolvePartInput) (api.ResolvePartOutput, error) {
	if !render.SchemaBoundKinds[in.Kind] {
		return api.ResolvePartOutput{}, fmt.Errorf("%w: kind %q draws no schema", cwmapi.ErrInvalidInput, in.Kind)
	}
	if _, ok := render.SchemaRefOf(in.Schema); !ok {
		return api.ResolvePartOutput{}, fmt.Errorf("%w: schema: a data-type URI or {action: <uri>}", cwmapi.ErrInvalidInput)
	}
	node := map[string]any{"kind": in.Kind, "schema": in.Schema}
	if in.Variant != "" {
		node["variant"] = in.Variant
	}
	options := false
	if _, err := render.ResolveComponents(node, render.ComponentResolver{
		Fetch: func(ref render.SchemaRef, ext, name string) (map[string]any, error) {
			ast, err := fetchSchema(ctx, ref, ext, name)
			if err == nil && declaresOptions(ast) {
				options = true
			}
			return ast, err
		},
		Finish: func(schema map[string]any) (map[string]any, error) { return withOptions(ctx, schema) },
	}); err != nil {
		return api.ResolvePartOutput{}, err
	}
	part, _ := node["schema"].(map[string]any)
	if part == nil {
		return api.ResolvePartOutput{}, fmt.Errorf("%s: no schema resolved", in.Kind)
	}
	hash, err := render.SpecHash(part)
	if err != nil {
		return api.ResolvePartOutput{}, err
	}
	out := api.ResolvePartOutput{Hash: hash, Cacheable: !options}
	if in.Hash != hash || options {
		out.Part = part
	}
	return out, nil
}

// fetchSchema reads a referenced schema as its AST: a data-type from the
// schema authority (its extensions filtered to one ui-<kind> variant), an
// action's input from ActionService.Describe.
func fetchSchema(ctx context.Context, ref render.SchemaRef, ext, name string) (map[string]any, error) {
	if ref.Action != "" {
		acts, err := application.UseService[actionsapi.ActionService](ctx)
		if err != nil || acts == nil {
			return nil, fmt.Errorf("resolve ActionService: %v", err)
		}
		d, err := acts.Describe(ctx, actionsapi.DescribeInput{URI: ref.Action})
		if err != nil {
			return nil, err
		}
		if len(d.InputSchemaAST) == 0 {
			return nil, fmt.Errorf("%s declares no input", ref.Action)
		}
		var ast map[string]any
		if err := json.Unmarshal(d.InputSchemaAST, &ast); err != nil {
			return nil, fmt.Errorf("decode the input of %s: %v", ref.Action, err)
		}
		return ast, nil
	}
	svc, err := application.UseService[schemasvc.SchemaService](ctx)
	if err != nil || svc == nil {
		return nil, fmt.Errorf("resolve SchemaService: %v", err)
	}
	got, err := svc.GetDataTypeWithExtensions(ctx, schemasvc.GetDataTypeWithExtensionsInput{
		URI: ref.URI, Extensions: []schemasvc.ExtensionRef{{Type: ext, Name: name}},
	})
	if err != nil {
		return nil, err
	}
	if got.Schema == nil {
		return nil, fmt.Errorf("no such data-type")
	}
	return toTree(got.Schema)
}

// withOptions computes the choices of every field of schema declaring an
// `options_from`; a schema declaring none comes back untouched.
func withOptions(ctx context.Context, schema map[string]any) (map[string]any, error) {
	if !declaresOptions(schema) {
		return schema, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var s coreschemaapi.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("schema: %v", err)
	}
	for _, e := range coreschemaapi.ResolveOptions(ctx, &s, OptionSources(ctx)) {
		slog.Warn("panel form: a field keeps free text", "schema", s.Name, "error", e)
	}
	return toTree(&s)
}

// OptionSources are the `options_from` sources a panel's form draws its
// choices from: the universe's catalog (this binary's own when it holds no
// universe) and the project's objects (cwmapi.ObjectOptions).
func OptionSources(ctx context.Context) map[string]coreschemaapi.OptionSource {
	cwm, _ := application.UseService[cwmapi.CWMService](ctx)
	return map[string]coreschemaapi.OptionSource{
		cwmapi.OptionSourceObjects: cwmapi.ObjectOptions(cwm),
		mbapi.OptionSourceCatalog: coreschemaapi.OptionSourceFunc(func(ctx context.Context, params map[string]any) ([]map[string]any, error) {
			records, err := mbapi.CatalogOptions(mbapi.Q{Reach: mbapi.ReachUniverse}).Records(ctx, params)
			if err != nil {
				return mbapi.CatalogOptions(mbapi.Q{Reach: mbapi.ReachSelf}).Records(ctx, params)
			}
			return records, nil
		}),
	}
}

// declaresOptions reports an `options_from` anywhere in the tree.
func declaresOptions(v any) bool {
	switch n := v.(type) {
	case map[string]any:
		if _, ok := n[coreschemaapi.OptionsFromKey]; ok {
			return true
		}
		for _, c := range n {
			if declaresOptions(c) {
				return true
			}
		}
	case []any:
		for _, c := range n {
			if declaresOptions(c) {
				return true
			}
		}
	}
	return false
}

func toTree(s *coreschemaapi.Schema) (map[string]any, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
