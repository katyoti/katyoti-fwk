// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WORK TEMPLATES — what a request's work is made of: the recipes, as data. The
// shipped ones come with planification; a module overrides one by name, or adds
// its own, in its planning space (`<module>/.ktw/templates/*.yaml`). The template
// of a request is chosen by its kind, or by the type and lifecycle state of the
// object it is on, at its depth (full unless the request says quick).

// templatesFor is the templates in force in a module: the shipped ones, each
// replaced by the module's of the same name, and the module's own after them.
func templatesFor(ctx context.Context, module string) ([]api.Recipe, error) {
	base, err := loadRecipes()
	if err != nil {
		return nil, err
	}
	own, err := moduleTemplates(ctx, module)
	if err != nil || len(own) == 0 {
		return base, err
	}
	return mergeTemplates(base, own), nil
}

// mergeTemplates is base with each template replaced by own's of the same
// name, and own's other templates added.
func mergeTemplates(base, own []api.Recipe) []api.Recipe {
	out := make([]api.Recipe, 0, len(base)+len(own))
	for _, r := range base {
		if i := slices.IndexFunc(own, func(o api.Recipe) bool { return o.Name == r.Name }); i >= 0 {
			out = append(out, own[i])
			continue
		}
		out = append(out, r)
	}
	for _, o := range own {
		if !slices.ContainsFunc(base, func(r api.Recipe) bool { return r.Name == o.Name }) {
			out = append(out, o)
		}
	}
	sortRecipes(out)
	return out
}

// moduleTemplates reads a module's own templates; none when it has no folder.
func moduleTemplates(ctx context.Context, module string) ([]api.Recipe, error) {
	if module == "" {
		return nil, nil
	}
	dir := filepath.Join(application.ProjectRoot(), modulesDir(ctx), module, ".ktw", "templates")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, nil
	}
	return readRecipes(os.DirFS(dir), ".")
}

// modulesDir is the project's modules folder (its project object's), else the default.
func modulesDir(ctx context.Context) string {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return cwmapi.DefaultModulesDir
	}
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapProject}) // the project level capability, never a type name
	if err != nil || len(got.Objects) == 0 {
		return cwmapi.DefaultModulesDir
	}
	return got.Objects[0].ModulesDir()
}

// templateFor is the work template of the request on o, in force in o's module.
func templateFor(ctx context.Context, o cwmapi.Object) (api.Recipe, error) {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	templates, err := templatesFor(ctx, module)
	if err != nil {
		return api.Recipe{}, err
	}
	resolved := ""
	if to := edgeTo(o, api.EdgeResolves); to != "" {
		if cwm, err := cwmOf(ctx); err == nil {
			if r, err := loadObject(ctx, cwm, to); err == nil {
				resolved = r.Type
			}
		}
	}
	return selectRecipeOf(templates, o, resolved)
}

// selectRecipe is selectRecipeOf for a request meeting nothing.
func selectRecipe(recipes []api.Recipe, o cwmapi.Object) (api.Recipe, error) {
	return selectRecipeOf(recipes, o, "")
}

// selectRecipeOf is the most specific template for the request on o — its
// types naming o's type (and its states o's state), else its kind — at the
// request's depth when one exists, else the full one.
func selectRecipeOf(recipes []api.Recipe, o cwmapi.Object, resolved string) (api.Recipe, error) {
	kind, depth := "", api.DepthFull
	if hasRequest(o) {
		f := fieldsOf(o)
		if kind = f.Kind; kind == "" {
			kind = api.KindBuild
		}
		if f.Depth != "" {
			depth = f.Depth
		}
	}
	state := lifeState(o)
	if state == "" {
		state = nodeState(o)
	}
	match := func(r api.Recipe, at string) bool {
		if recipeDepth(r) != at {
			return false
		}
		if len(r.States) > 0 && !slices.Contains(r.States, state) {
			return false
		}
		if len(r.Resolves) > 0 && !slices.Contains(r.Resolves, resolved) {
			return false
		}
		if len(r.Types) > 0 {
			return slices.Contains(r.Types, o.Type) && (r.Kind == "" || r.Kind == kind)
		}
		return kind != "" && r.Kind == kind
	}
	for _, at := range []string{depth, api.DepthFull} {
		for _, r := range recipes {
			if match(r, at) {
				return r, nil
			}
		}
	}
	return api.Recipe{}, invalid("no template for a %s in state %s (kind %q, depth %s)", o.Type, state, kind, depth)
}

// recipeDepth is a template's depth: full unless it says quick.
func recipeDepth(r api.Recipe) string {
	if r.Depth == "" {
		return api.DepthFull
	}
	return r.Depth
}
