// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// GlobalPanels answers the declared panels marked global: the Global strip's
// tabs, each by its element id `home:<stem>`, by order then name. There is no
// object: a source panel is called with
// the target, a content panel shows as declared.
func (s *Service) GlobalPanels(ctx context.Context, _ api.GlobalPanelsInput) (api.GlobalPanelsOutput, error) {
	out := api.GlobalPanelsOutput{Panels: []api.ObjectPanel{}}
	for _, d := range s.declarations(ctx).Panels {
		if !d.Global || d.ID == "" {
			continue
		}
		e := api.ObjectPanel{
			ID: "home:" + d.ID, Name: d.Name, Description: d.Description, Icon: d.Icon,
			Order: d.EffectiveOrder(), Origin: "panel", Content: d.Content, Targets: d.Targets, Count: d.Count, Input: d.Input, Empty: d.Empty,
		}
		r := d.EffectiveRefresh()
		e.Refresh = &r
		if e.Name == "" {
			e.Name = d.ID
		}
		e.Kind = api.PanelKindContent
		if d.Source != "" {
			e.Kind, e.Source = api.PanelKindSource, d.Source
		}
		out.Panels = append(out.Panels, e)
	}
	sort.SliceStable(out.Panels, func(i, j int) bool {
		a, b := out.Panels[i], out.Panels[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.Name < b.Name
	})
	return out, nil
}
