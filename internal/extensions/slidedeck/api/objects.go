// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A deck theme stored in the project (OD-008): a `deck-theme` object whose
// `deck-theme` capability is its ThemeBody, its files in its module's
// themes/<name>/ folder. The resolver finds these before the sidecars'
// deck-theme contributions; one of the same name hides the contribution.
const (
	ObjectTypeDeckTheme = "deck-theme"
	CapabilityDeckTheme = "deck-theme"
	// ProviderProject is the provider a project theme is attributed to.
	ProviderProject = "project"
	// ThemesFolder is where a project theme's files sit in its module.
	ThemesFolder = "themes"
)

// ThemeObjects is what the resolver reads project themes through: the
// objects of a type, and a file of their module. CWMService has both; a host
// without it in process (the GUI) passes its own with WithThemeObjects.
type ThemeObjects interface {
	ObjectList(ctx context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error)
	ObjectFileRead(ctx context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error)
}

type themeObjectsKey struct{}

// WithThemeObjects makes the resolver read project themes through src.
func WithThemeObjects(ctx context.Context, src ThemeObjects) context.Context {
	return context.WithValue(ctx, themeObjectsKey{}, src)
}

// themeObjects is the source of project themes: the one the context names,
// else the CWMService in reach; nil when there is none (no project themes).
func themeObjects(ctx context.Context) ThemeObjects {
	if src, ok := ctx.Value(themeObjectsKey{}).(ThemeObjects); ok && src != nil {
		return src
	}
	if application.AppFromContext(ctx) == nil {
		return nil
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || cwm == nil {
		return nil
	}
	return cwm
}

// projectTheme is a theme stored as an object.
type projectTheme struct {
	object string
	body   ThemeBody
}

// projectThemes reads the project's deck-theme objects; a CWM that cannot be
// read leaves the contributions alone (no project themes), never an error.
func projectThemes(ctx context.Context) []projectTheme {
	src := themeObjects(ctx)
	if src == nil {
		return nil
	}
	out, err := src.ObjectList(ctx, cwmapi.ObjectListInput{Type: ObjectTypeDeckTheme})
	if err != nil {
		return nil
	}
	var themes []projectTheme
	for _, o := range out.Objects {
		body, err := ThemeOfObject(o)
		if err != nil || !themeName.MatchString(body.Name) {
			continue // not a theme a deck can name
		}
		themes = append(themes, projectTheme{object: o.ID, body: body})
	}
	return themes
}

// ThemeOfObject reads a deck-theme object's declaration (its capability).
func ThemeOfObject(o cwmapi.Object) (ThemeBody, error) {
	raw, err := json.Marshal(o.Capabilities[CapabilityDeckTheme])
	if err != nil {
		return ThemeBody{}, err
	}
	var t ThemeBody
	if err := json.Unmarshal(raw, &t); err != nil {
		return ThemeBody{}, fmt.Errorf("object %s: not a deck theme: %w", o.ID, err)
	}
	return t, nil
}

// themeName is a name a deck writes in `theme:` (kebab-case ASCII).
var themeName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// safeAssetPath: a theme's file is named inside its folder — relative, with
// no `..` segment and no drive or backslash — so a declaration (editable on
// the object) cannot read the module's other files.
func safeAssetPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, `\:`) {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || seg == "" {
			return false
		}
	}
	return true
}

// ThemeFilePath is where a project theme's file sits in its module.
func ThemeFilePath(theme, file string) string {
	return path.Join(ThemesFolder, theme, file)
}

// withProjectThemes puts the project's themes first: a contribution of the
// same name is hidden.
func withProjectThemes(project []projectTheme, contributed []mbapi.Attributed[ThemeBody]) []mbapi.Attributed[ThemeBody] {
	if len(project) == 0 {
		return contributed
	}
	seen := map[string]bool{}
	out := make([]mbapi.Attributed[ThemeBody], 0, len(project)+len(contributed))
	for _, p := range project {
		if seen[p.body.Name] {
			continue
		}
		seen[p.body.Name] = true
		out = append(out, mbapi.Attributed[ThemeBody]{Provider: ProviderProject, Body: p.body})
	}
	for _, c := range contributed {
		if !seen[c.Body.Name] {
			out = append(out, c)
		}
	}
	return out
}

// resolveProjectTheme reads a project theme's files from its module.
func resolveProjectTheme(ctx context.Context, p projectTheme) (ResolvedTheme, error) {
	src := themeObjects(ctx)
	out := ResolvedTheme{Provider: ProviderProject, Body: p.body, Assets: map[string][]byte{}}
	for _, a := range p.body.Assets() {
		if !safeAssetPath(a) {
			return ResolvedTheme{}, fmt.Errorf("theme %s: file %q leaves the theme's folder", p.body.Name, a)
		}
		f, err := src.ObjectFileRead(ctx, cwmapi.ObjectFileReadInput{ID: p.object, Path: ThemeFilePath(p.body.Name, a)})
		if err != nil {
			return ResolvedTheme{}, fmt.Errorf("theme %s: file %s: %w", p.body.Name, a, err)
		}
		out.Assets[a] = f.Content
	}
	return out, nil
}

// OptionSourceDeckThemes is the `options_from` source of ThemeOptions.
const OptionSourceDeckThemes = "deck-themes"

// ThemeOptions is the `deck-themes` option source: every theme in reach —
// the project's first, then the contributions q reaches — as records a form
// field chooses from: `name`, `description`, `provider` and `data` (the
// theme's declaration). A form whose host serves it offers a theme imported
// into the project the moment it is stored.
func ThemeOptions(q mbapi.Q) schemaapi.OptionSource {
	return schemaapi.OptionSourceFunc(func(ctx context.Context, _ map[string]any) ([]map[string]any, error) {
		themes, err := Themes(ctx, q)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(themes))
		for _, t := range themes {
			var data map[string]any
			if raw, err := json.Marshal(t.Body); err == nil {
				_ = json.Unmarshal(raw, &data)
			}
			out = append(out, map[string]any{"name": t.Body.Name, "description": t.Body.Description, "provider": t.Provider, "data": data})
		}
		return out, nil
	})
}

// IsProjectTheme reports whether the project holds a theme named name (which
// then hides a sidecar's theme of that name).
func IsProjectTheme(ctx context.Context, name string) bool {
	for _, p := range projectThemes(ctx) {
		if p.body.Name == name {
			return true
		}
	}
	return false
}
