// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"strings"
)

// PageTemplateKind is the contribution kind of a page template
// : a page tree contributed once and named by its URI
// <pack>:/page-template/<name>.
const PageTemplateKind = "page-template"

// PageTemplate is the body of a `page-template` contribution: a page tree in
// the page DSL that an answer names by URI and hash instead of carrying it
// (a render.Spec) - a panel type's page, a page source's tree
// a section's. Any pack adds its own as a file,
// embed/page-templates/<name>.yaml, read as the kind declares
// (system:/contribution-type/page-template). The part an answer names is the
// Tree alone: its hash is SpecHash(Tree).
type PageTemplate struct {
	Label       string         `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=The name shown for this template"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=What a page drawn from this template shows"`
	Tree        map[string]any `json:"tree" yaml:"tree" jsonschema:"description=The page tree in the page DSL: a page root or one node for a section"`
}

// Validate refuses a template with no tree or a tree off the page grammar.
func (t PageTemplate) Validate() error {
	if len(t.Tree) == 0 {
		return fmt.Errorf("render: page template: no tree")
	}
	if err := ValidatePage(t.Tree); err != nil {
		return fmt.Errorf("render: page template: %w", err)
	}
	return nil
}

// Spec names the template's tree by its URI and the hash of its tree.
func (t PageTemplate) Spec(uri string) (Spec, error) {
	return NamedSpec(uri, t.Tree)
}

// IsPageTemplateURI reports whether uri names a page template.
func IsPageTemplateURI(uri string) bool {
	return strings.Contains(uri, ":/"+PageTemplateKind+"/")
}
