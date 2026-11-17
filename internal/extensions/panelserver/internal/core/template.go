// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// ErrUnknownTemplate is a page template the universe does not contribute.
var ErrUnknownTemplate = fmt.Errorf("panel server: unknown page template")

// Template answers a page template's tree by its URI, with
// its hash, render.SpecHash of the tree: a host giving the current hash gets
// the hash alone - a template it holds costs no tree on the wire; a stale or
// absent hash gets the tree. A template off the page grammar is refused,
// naming it.
func (s *Server) Template(ctx context.Context, in panelapi.TemplateInput) (panelapi.TemplateOutput, error) {
	if !render.IsPageTemplateURI(in.URI) {
		return panelapi.TemplateOutput{}, fmt.Errorf("panel server: %q names no page template (<pack>:/%s/<name>)", in.URI, render.PageTemplateKind)
	}
	if s.opts.TemplateBody == nil {
		return panelapi.TemplateOutput{}, fmt.Errorf("%w: %s", ErrUnknownTemplate, in.URI)
	}
	body, err := s.opts.TemplateBody(ctx, in.URI)
	if err != nil {
		return panelapi.TemplateOutput{}, fmt.Errorf("%w: %s: %v", ErrUnknownTemplate, in.URI, err)
	}
	if len(body) == 0 {
		return panelapi.TemplateOutput{}, fmt.Errorf("%w: %s", ErrUnknownTemplate, in.URI)
	}
	var t render.PageTemplate
	if err := config.DecodeBytes(body, marshal.YAML, &t); err != nil {
		return panelapi.TemplateOutput{}, fmt.Errorf("panel server: page template %s: %w", in.URI, err)
	}
	if err := t.Validate(); err != nil {
		return panelapi.TemplateOutput{}, fmt.Errorf("panel server: %s: %w", in.URI, err)
	}
	spec, err := t.Spec(in.URI)
	if err != nil {
		return panelapi.TemplateOutput{}, err
	}
	out := panelapi.TemplateOutput{Hash: spec.Hash}
	if in.Hash != spec.Hash {
		out.Tree = t.Tree
	}
	return out, nil
}
