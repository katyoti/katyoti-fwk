// SPDX-License-Identifier: Apache-2.0

// Package publisherkind is the `publication-connector` kind as the
// publications pack declares it: its contribution-type
// (publication-connector.yaml, the schema provider-relative, the files read
// from a pack's publication-connectors folder) and its data-type, the shape
// of api.ConnectorDecl. A leaf: it imports the leaf shape, the Go-type helper
// and the publisher API only (no CWM), so the built-in kind reader
// (internal/core/contribution/builtin) can import it without a cycle - and a
// publisher ships its publishers as a folder of its pack, read by the generic
// pack reader.
package publisherkind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

//go:embed publication-connector.yaml
var declaration []byte

// Leaf is the publication-connector kind's declaration: its YAML and its
// schema, the shape of api.ConnectorDecl.
func Leaf() leaf.Declaration {
	return leaf.Declaration{
		Kind: api.ContributionTypePublicationConnector,
		YAML: declaration,
		Schema: manifestcontrib.GoTypeOf[api.ConnectorDecl]("publication-connector", "1.0.0", "ConnectorDecl",
			"A publisher: the data type of its instances' settings and the method\n"+
				"that publishes a document tree.\n"),
	}
}
