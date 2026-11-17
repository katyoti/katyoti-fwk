// SPDX-License-Identifier: Apache-2.0

package render

import (
	"context"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/quality"
)

// RelayoutResult is a .drawio laid out again, with the quality of the
// result.
type RelayoutResult struct {
	XML     string                   `json:"xml"`
	Moved   int                      `json:"moved"`
	Pinned  []string                 `json:"pinned,omitempty"`
	Report  []string                 `json:"report,omitempty"`
	Quality *drawioapi.QualityReport `json:"quality,omitempty"`
}

// Relayout lays out an existing .drawio — generated, edited in draw.io or
// drawn by hand — and returns it with only its geometry changed. Groups
// are laid out as nested containers, pinned cells keep their box, and the
// drawing's current order is kept. The layout options are, field by
// field: hints, then the file's own layout hints, then its viewpoint's
// profile, then the defaults. Needs bun (ELK).
func Relayout(ctx context.Context, data []byte, hints *drawioapi.LayoutHints) (*RelayoutResult, error) {
	data, err := drawioapi.ExtractDrawioXML(data)
	if err != nil {
		return nil, err
	}
	read, err := archdiagram.Read(data)
	if err != nil {
		return nil, err
	}
	layout := mergeLayout(projectLayoutHints(hints), projectLayoutHints(read.Diagram.LayoutHints))
	if vp := read.Diagram.Viewpoint; vp != "" {
		if dr, err := drawioapi.ResolveDrawing(ctx, vp); err == nil && dr.Viewpoint != nil {
			layout = mergeLayout(layout, projectLayoutHints(dr.Viewpoint.Layout))
		}
	}
	if layout == nil {
		layout = archdiagram.DefaultLayout()
	} else {
		layout = mergeLayout(layout, archdiagram.DefaultLayout())
	}
	out, err := archdiagram.Relayout(ctx, data, layout)
	if err != nil {
		return nil, err
	}
	res := &RelayoutResult{XML: string(out.XML), Moved: out.Moved, Pinned: out.Pinned, Report: out.Report}
	if after, err := archdiagram.Read(out.XML); err == nil {
		res.Quality = quality.Score(after)
	}
	return res, nil
}
