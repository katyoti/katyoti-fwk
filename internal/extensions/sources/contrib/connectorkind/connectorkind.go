// SPDX-License-Identifier: Apache-2.0

// Package connectorkind is the `source-connector` kind as the sources pack
// declares it: its contribution-type (source-connector.yaml, the schema
// provider-relative, the files read from a pack's source-connectors folder)
// and its data-type, the shape of connectordecl.ConnectorDecl. A leaf: it
// imports the leaf shape, the Go-type helper and the CWM-free declaration
// type only, so the built-in kind reader (internal/core/contribution/builtin)
// can import it without a cycle - and a connector ships its connectors as a
// folder of its pack, read by the generic pack reader.
package connectorkind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api/connectordecl"
)

//go:embed source-connector.yaml
var declaration []byte

// Leaf is the source-connector kind's declaration: its YAML and its schema,
// the shape of connectordecl.ConnectorDecl.
func Leaf() leaf.Declaration {
	return leaf.Declaration{
		Kind: connectordecl.Kind,
		YAML: declaration,
		Schema: manifestcontrib.GoTypeOf[connectordecl.ConnectorDecl]("source-connector", "1.5.0", "ConnectorDecl",
			"A kind of place a module can read from: the form of its location and\n"+
				"the method that syncs it.\n"),
	}
}
