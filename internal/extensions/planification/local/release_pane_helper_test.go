// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// releasePane is a release's pane as the GUI shows it once its section has
// loaded: its header and actions, then its work - the
// release-work section replaced by what releases/page-section answers.
func releasePane(ctx context.Context, rel api.ReleaseService, id string) (render.Def, error) {
	def, err := rel.Page(ctx, api.GetInput{ID: id})
	if err != nil {
		return def, err
	}
	work, err := rel.PageSection(ctx, api.ReleasePageSectionInput{ID: id, Section: api.ReleasePageWork})
	if err != nil {
		return def, err
	}
	page, _ := def.DSLTree.(map[string]any)
	if page == nil {
		return def, nil
	}
	kids, _ := page["children"].([]map[string]any)
	inner, _ := work.DSLTree.(map[string]any)
	rest, _ := inner["children"].([]map[string]any)
	var out []map[string]any
	for _, k := range kids {
		if k["id"] == "release-work" {
			out = append(out, rest...)
			continue
		}
		out = append(out, k)
	}
	page["children"] = out
	return def, nil
}
