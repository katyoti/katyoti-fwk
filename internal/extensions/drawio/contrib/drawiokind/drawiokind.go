// SPDX-License-Identifier: Apache-2.0

// Package drawiokind is the drawio kinds as the drawio pack declares them:
// `notation`, `viewpoint` and `drawio-template`, each a contribution-type (its
// YAML, the schema provider-relative, and a `source:` block saying how a
// pack's folder becomes entries) and a data-type, the shape of its drawioapi
// body. A leaf: it imports the leaf shape, the Go-type helper and drawio/api
// only, so the built-in kind reader (internal/core/contribution/builtin) can
// import it without a cycle - and a sidecar that ships notations, viewpoints
// or template libraries as folders of its pack never links drawio/contrib.
package drawiokind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

var (
	//go:embed notation.yaml
	notationYAML []byte
	//go:embed viewpoint.yaml
	viewpointYAML []byte
	//go:embed drawio-template.yaml
	templateYAML []byte
)

// Notation is the notation kind's declaration: its YAML and its schema.
func Notation() leaf.Declaration {
	return leaf.Declaration{
		Kind: drawioapi.ContributionTypeNotation,
		YAML: notationYAML,
		Schema: manifestcontrib.GoTypeOf[drawioapi.NotationBody](drawioapi.ContributionTypeNotation, "1.0.0", "Notation",
			"The language a diagram speaks: its element kinds, its relationship kinds\n"+
				"and the rules that make a model correct for it.\n"),
	}
}

// Viewpoint is the viewpoint kind's declaration: its YAML and its schema.
func Viewpoint() leaf.Declaration {
	return leaf.Declaration{
		Kind: drawioapi.ContributionTypeViewpoint,
		YAML: viewpointYAML,
		Schema: manifestcontrib.GoTypeOf[drawioapi.ViewpointBody](drawioapi.ContributionTypeViewpoint, "1.0.0", "Viewpoint",
			"A type of drawing: a notation, the kinds this picture shows, the template\n"+
				"library and page template that draw it, and its layout profile.\n"),
	}
}

// Template is the drawio-template kind's declaration: its YAML and its schema.
// The .drawio file beside a library is folded into its data as `drawio`.
func Template() leaf.Declaration {
	return leaf.Declaration{
		Kind: drawioapi.ContributionTypeDrawioTemplate,
		YAML: templateYAML,
		Schema: manifestcontrib.GoTypeOf[drawioapi.TemplateLibraryBody](drawioapi.ContributionTypeDrawioTemplate, "1.0.0", "TemplateLibrary",
			"A template library: draw.io templates for the kinds of a notation or a\n"+
				"house style, with the constants and bindings their placeholders read.\n"),
	}
}

// Leaves are the three drawio kinds, in the order a binary publishes them.
func Leaves() []leaf.Declaration {
	return []leaf.Declaration{Notation(), Viewpoint(), Template()}
}
