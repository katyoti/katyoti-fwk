// SPDX-License-Identifier: Apache-2.0

package census

import (
	"encoding/json"
	"fmt"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// validate_panel_components.go — an object panel names a published view.
//
// The views an object panel mounts (`content: {kind: extension, extension:
// <name>}`) are entries of the `panel-component` kind, published by the pack
// that owns each view. A panel naming a view no entry carries is a finding
// naming the panel and the view; a panel with static content (no source)
// whose view declares an input data-type has its content data checked against
// it. A panel with a source is answered at run time: its data is not judged
// here. Panels and views may come from different binaries, so the check reads
// every shipped pack at once (TestEveryShippedObjectPanelNamesAPublishedComponent).

// inputCheck validates a body against a data-type URI: the problems found, or
// an error when the schema cannot be read.
type inputCheck func(schema string, body []byte) ([]string, error)

// panelComponentFindings is one finding per object panel of cs naming an
// unknown view, or whose static data does not match its view's input.
func panelComponentFindings(cs []contribapi.Contribution, check inputCheck) []string {
	components := map[string]objectpaneapi.PanelComponentDecl{}
	for _, c := range cs {
		if c.Type != objectpaneapi.PanelComponentKind {
			continue
		}
		var d objectpaneapi.PanelComponentDecl
		_ = json.Unmarshal(c.Data(), &d)
		components[c.ID] = d
	}
	var out []string
	for _, c := range cs {
		if c.Type != objectpaneapi.ObjectPanelKind || len(c.Data()) == 0 {
			continue
		}
		var p objectpaneapi.PanelDecl
		if err := json.Unmarshal(c.Data(), &p); err != nil {
			out = append(out, fmt.Sprintf("object panel %s: unreadable: %v", c.ResolvedURI(), err))
			continue
		}
		if p.Content == nil || p.Content.Kind != "extension" {
			continue
		}
		name := p.Content.Extension
		comp, ok := components[name]
		if !ok {
			out = append(out, fmt.Sprintf("object panel %s names the view %q, which no panel-component entry publishes", c.ResolvedURI(), name))
			continue
		}
		if comp.Input == "" || p.Source != "" || p.Content.Data == nil || check == nil {
			continue
		}
		body, err := json.Marshal(p.Content.Data)
		if err != nil {
			out = append(out, fmt.Sprintf("object panel %s: its content data: %v", c.ResolvedURI(), err))
			continue
		}
		problems, err := check(comp.Input, body)
		if err != nil {
			out = append(out, fmt.Sprintf("object panel %s: the input %s of the view %q: %v", c.ResolvedURI(), comp.Input, name, err))
			continue
		}
		for _, pr := range problems {
			out = append(out, fmt.Sprintf("object panel %s: its content data does not match the input %s of the view %q: %s", c.ResolvedURI(), comp.Input, name, pr))
		}
	}
	sort.Strings(out)
	return out
}
