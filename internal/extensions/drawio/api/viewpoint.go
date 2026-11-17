// SPDX-License-Identifier: Apache-2.0

package api

// ContributionTypeViewpoint is the Type discriminator of a viewpoint.
const ContributionTypeViewpoint = "viewpoint"

// ViewpointBody is a `viewpoint` contribution: what the user picks as
// "the type of drawing". It binds a notation (vocabulary and rules), the
// kinds this picture shows, a template library (looks) with its page
// template, and a layout profile.
type ViewpointBody struct {
	ID            string       `json:"id"                      jsonschema:"description=Kebab-case identifier; matches the contribution ID"`
	DisplayName   string       `json:"displayName"             jsonschema:"description=Human-readable name such as ArchiMate layered view"`
	Description   string       `json:"description,omitempty"   jsonschema:"description=What this kind of picture is for"`
	Notation      string       `json:"notation"                jsonschema:"description=URI of the notation contribution (provider:/notation/id)"`
	Templates     string       `json:"templates"               jsonschema:"description=URI of the drawio-template library that draws it (provider:/drawio-template/id)"`
	Page          string       `json:"page,omitempty"          jsonschema:"description=Name of the page template in the library; empty draws no frame"`
	Elements      []string     `json:"elements,omitempty"      jsonschema:"description=Element kinds this viewpoint shows; empty shows every kind of the notation"`
	Relationships []string     `json:"relationships,omitempty" jsonschema:"description=Relationship kinds this viewpoint shows; empty shows every kind of the notation"`
	Layout        *LayoutHints `json:"layout,omitempty"        jsonschema:"description=Layout profile: preset or algorithm and direction and ELK options"`
}

// ShowsElement reports whether the viewpoint admits an element kind.
func (v *ViewpointBody) ShowsElement(kind string) bool { return admits(v.Elements, kind) }

// ShowsRelationship reports whether the viewpoint admits a relationship kind.
func (v *ViewpointBody) ShowsRelationship(kind string) bool { return admits(v.Relationships, kind) }

func admits(list []string, kind string) bool {
	if len(list) == 0 {
		return true
	}
	for _, k := range list {
		if k == kind || k == "*" {
			return true
		}
	}
	return false
}
