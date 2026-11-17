// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// The hub holds no bytes for a non-resident layer and can reach the producer
// through ONE route, Read, which answers with one fixed layer. Whether a
// requested layer can take that route is a question about CONTENT ADDRESSES,
// not about which layer the kind calls its default: a capability-panel is one
// JSON file declared as both `data` and `source`, and its `source` tab was
// refused with "not the kind's default layer" while the identical bytes were
// one Read away (2026-09-22 screenshot).
func TestOpenLayer_ReadServesAnyLayerWithTheSameContentAddress(t *testing.T) {
	one := []byte(`{"id":"confluence","label":"Confluence"}`)
	panel := contribapi.Contribution{Provider: "ktw-acme", Type: "capability-panel", ID: "confluence"}
	panel.SetData(one)
	panel.SetSource(one, "application/json")

	if !readServes(panel, contribapi.LayerSource) {
		t.Error("source with the data layer's hash: Read yields these bytes, must be served")
	}
	if !readServes(panel, contribapi.LayerData) {
		t.Error("data on a two-layer entry whose source is the same bytes: served")
	}

	// A skill: markdown source, `{}` data. Read answers with the source; the
	// data layer is different content and stays refused until providers open
	// a layer by name.
	skill := contribapi.Contribution{Provider: "ktw-agents", Type: "skill", ID: "company-research"}
	skill.SetData([]byte(`{}`))
	skill.SetSource([]byte("# Company Research\n"), "text/markdown")
	if !readServes(skill, contribapi.LayerSource) {
		t.Error("the layer Read answers with is always reachable")
	}
	if readServes(skill, contribapi.LayerData) {
		t.Error("data with a different hash than the source: Read cannot yield it — refusing is the honest answer")
	}

	// Data only: Read answers with the data layer.
	capability := contribapi.Contribution{Provider: "ktw-code", Type: "capability", ID: "module-health"}
	capability.SetData(one)
	if !readServes(capability, contribapi.LayerData) {
		t.Error("data-only entry: Read yields the data layer")
	}
	if readServes(capability, contribapi.LayerSource) {
		t.Error("an undeclared layer has no address and is never served")
	}
}
