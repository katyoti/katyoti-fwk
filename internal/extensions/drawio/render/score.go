// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/quality"
)

// Score measures a .drawio document: overlaps, crossings, edges through
// nodes, bends, zone escapes, extent and density. It reads the document
// first, so it scores any diagram, generated or drawn by hand. It measures
// the first page; ScorePages measures each.
func Score(data []byte) (*drawioapi.QualityReport, error) {
	r, err := Read(data)
	if err != nil {
		return nil, err
	}
	return quality.Score(r), nil
}

// CheckLayout lists what reads badly on a .drawio document as draw.io
// paints it, on every page: labels over labels (text-overlap, gated), the
// same text twice in one place, labels out of their shape, shapes covering
// each other. In a document of several pages each finding names its page. A
// document that cannot be read is an error, never a pass.
func CheckLayout(data []byte) ([]drawioapi.Finding, error) {
	pages, err := Pages(data)
	if err != nil {
		return nil, err
	}
	var out []drawioapi.Finding
	for _, p := range pages {
		r, err := archdiagram.Read(p.XML)
		if err != nil {
			return nil, fmt.Errorf("page %q: %w", pageLabel(p), err)
		}
		for _, f := range quality.CheckLayout(r) {
			if len(pages) > 1 {
				f.Page = pageLabel(p)
			}
			out = append(out, f)
		}
	}
	return out, nil
}
