// SPDX-License-Identifier: Apache-2.0

package api

// ContributionTypeDrawioTemplate is the Type discriminator of a template
// library.
const ContributionTypeDrawioTemplate = "drawio-template"

// TemplateLibraryBody is a `drawio-template` contribution: the looks of a
// notation or of a house style. The templates themselves are draw.io XML,
// authored in draw.io: in Drawio, each top-level cell (or group) marked
// ktw-template="<kind>" draws that element kind, "edge:<kind>" that
// relationship kind, and a page named "page:<name>" is a page
// template. Labels and attribute values use %name% placeholders;
// ktw-style="<styleKey>=<binding>;…" overrides style keys with bindings;
// ktw-when="<expression>" keeps a child cell only when it holds.
//
// Bindings compute the values a template cannot hold literally (a colour
// from a lookup, a list of links): each is one of the value primitives
// (scalar, const, lookup, match, coalesce, expr) over the element's scope
// (name, id, kind, attributes, constants).
type TemplateLibraryBody struct {
	ID          string              `json:"id"                    jsonschema:"description=Kebab-case identifier; matches the contribution ID"`
	DisplayName string              `json:"displayName"           jsonschema:"description=Human-readable name of the library"`
	Description string              `json:"description,omitempty" jsonschema:"description=What this library draws and for whom"`
	Notation    string              `json:"notation,omitempty"    jsonschema:"description=URI of the notation whose kinds the library draws"`
	Constants   map[string]any      `json:"constants,omitempty"   jsonschema:"description=Shared lookup tables (palettes and sizes) read by bindings"`
	Bindings    map[string]Resolver `json:"bindings,omitempty"    jsonschema:"description=Named derived values a template reads as %name% or through ktw-style"`
	Drawio      string              `json:"drawio"                jsonschema:"description=The draw.io file holding the templates (filled by the provider from the library's .drawio file)"`
}
