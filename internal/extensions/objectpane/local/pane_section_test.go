// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	panecontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/contrib"
)

// A type's `pane:` section is the pane's to read: its view fills the type's
// Information tab. The `pane` capability it puts in force is type-level — in
// force (what extensions read) but not listed among the object's capabilities
// in the header.
func TestThePaneSectionIsReadAndItsCapabilityNotShown(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/container.yaml": "name: container\n",
		"object-types/note.yaml":      "name: note\ntree: node\ncapabilities: [container]\npane:\n  view: {kind: extension, extension: note-view}\n",
	}, panecontrib.Pack(""))
	create(t, ctx, cwm, cwmapi.Object{ID: "m.n", Type: "note"})

	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: "m.n"})
	if err != nil {
		t.Fatal(err)
	}
	inForce := false
	for _, c := range eff.Capabilities {
		inForce = inForce || c.ID == "pane"
	}
	if !inForce {
		t.Fatalf("the section's capability is not in force: %+v", eff.Capabilities)
	}

	d := detail(t, ctx, svc, "m.n", true)
	if slices.Contains(d.Capabilities, "pane") {
		t.Errorf("a type-level capability is listed in the header: %v", d.Capabilities)
	}
	for _, c := range d.Effective {
		if c.ID == "pane" {
			t.Errorf("a type-level capability is in the shown effective list: %+v", d.Effective)
		}
	}
	if !slices.Contains(d.Capabilities, "container") {
		t.Errorf("the type's own capability is not shown: %v", d.Capabilities)
	}
	fields, ok := panelByID(d.Panels)["section:fields"]
	if !ok || fields.Content == nil || fields.Content.Extension != "note-view" {
		t.Fatalf("the type's view did not fill its tab: %+v (strip %v)", fields, ids(d.Panels))
	}
}
