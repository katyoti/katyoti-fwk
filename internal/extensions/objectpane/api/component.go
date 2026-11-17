// SPDX-License-Identifier: Apache-2.0

package api

// PanelComponentKind is the contribution kind of a named view an object panel
// mounts (`content: {kind: extension, extension: <name>}`). The pack that owns
// the view publishes it from its CWM pack's `panel-components/` folder, the
// entry's id being the view's name.
const PanelComponentKind = "panel-component"

// PanelComponentDecl is a `panel-component` contribution: one named view of
// the GUI's ui-components registry. A panel naming a view no entry carries is
// a census finding; the GUI registry and the published entries are held equal
// by a drift test.
type PanelComponentDecl struct {
	Description string `json:"description"     yaml:"description"     jsonschema:"description=What the view shows and what it calls"`
	Input       string `json:"input,omitempty" yaml:"input,omitempty" jsonschema:"description=Data-type URI of what the view receives: a static panel's content data is checked against it; empty: the view reads the object itself"`
}
