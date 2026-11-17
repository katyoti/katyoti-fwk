// SPDX-License-Identifier: Apache-2.0

// Package core implements the file-overview service over CWM's named files.
package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/fileoverview/api"
)

// Service implements api.FileOverviewService.
type Service struct{}

// New returns the service.
func New() *Service { return &Service{} }

var _ api.FileOverviewService = (*Service)(nil)

// Read returns the first of the capability's named files that exists.
func (s *Service) Read(ctx context.Context, in api.ReadInput) (api.ReadOutput, error) {
	if in.ID == "" {
		return api.ReadOutput{}, fmt.Errorf("%w: id is required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ReadOutput{}, err
	}
	for _, name := range api.Names {
		f, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: in.ID, Capability: api.Capability, Name: name})
		if err != nil {
			return api.ReadOutput{}, err
		}
		if f.File.Exists {
			return api.ReadOutput{Name: name, Path: f.File.Path, Media: f.File.Media, Exists: true, Content: string(f.Content)}, nil
		}
	}
	return api.ReadOutput{}, nil
}

// noOverviewYet is what the panel says of an object whose overview is not
// written: what the overview is, where it goes, and who writes it — no generic
// action writes one; a type that does offers it among the object's actions.
func noOverviewYet(path string) string {
	where := "the object's overview file"
	if path != "" {
		where = "the object's overview file, " + path
	}
	return "No overview yet. It is " + where + ", written by an agent or a person; " +
		"when this object's type writes one, the action is among the object's actions."
}

// overviewPath is where the markdown overview goes, from the capability's
// declaration ("" when it cannot be resolved).
func overviewPath(ctx context.Context, id string) string {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return ""
	}
	f, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: id, Capability: api.Capability, Name: "overview"})
	if err != nil {
		return ""
	}
	return f.File.Path
}

// Page answers the overview as the display that shows it.
func (s *Service) Page(ctx context.Context, in api.ReadInput) (render.Def, error) {
	got, err := s.Read(ctx, in)
	if err != nil {
		return render.Def{}, err
	}
	switch {
	case !got.Exists:
		return emptyPage(noOverviewYet(overviewPath(ctx, in.ID)), ""), nil
	case got.Name == "page":
		return pageOf(got), nil
	}
	mt := got.Media
	if mt == "" {
		mt = render.MediaTypeForPath(got.Path)
	}
	d := render.Def{MediaType: mt, Data: got.Content, Title: got.Path}
	for kind, kmt := range render.DocumentKinds() {
		if kmt == mt {
			d.Kind = kind
		}
	}
	return d, nil
}

// pageOf reads the page file: a component naming its kind is shown as
// written, a bare tree is a page; a file that is no JSON object, or a page
// off the grammar, says so instead of failing the panel.
func pageOf(got api.ReadOutput) render.Def {
	var raw map[string]any
	if err := json.Unmarshal([]byte(got.Content), &raw); err != nil || raw == nil {
		return emptyPage(fmt.Sprintf("The overview page %s is not a JSON object: %v", got.Path, err), "red")
	}
	d := render.Def{Kind: "page", DSLTree: raw, Title: got.Path}
	if kind, _ := raw["kind"].(string); kind != "" {
		if _, isNode := nodeKinds()[kind]; !isNode {
			d = render.Def{}
			if err := json.Unmarshal([]byte(got.Content), &d); err != nil {
				return emptyPage(fmt.Sprintf("The overview page %s is not a display definition: %v", got.Path, err), "red")
			}
		}
	}
	if err := d.Validate(); err != nil {
		return emptyPage(fmt.Sprintf("The overview page %s cannot be shown: %v", got.Path, err), "red")
	}
	return d
}

// nodeKinds are the kinds of a page node: a file whose root is one of them is
// a bare tree, not a component (a `table` is both — as a root it is a node).
func nodeKinds() map[string]bool {
	out := map[string]bool{}
	for _, ks := range render.PageKinds {
		for _, k := range ks {
			out[k] = true
		}
	}
	return out
}

func emptyPage(text, color string) render.Def {
	return render.Def{Kind: "page", DSLTree: render.Section("Overview", "file-text").Empty(text, color)}
}
