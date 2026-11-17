// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the publications extension's declarations: the
// `publication-connector` kind (from its leaf, publisherkind) and the data
// types of the publisher API (the publication capability and the document
// tree). A publisher ships its publication connectors as a folder of its pack
// (publication-connectors), read by the generic pack reader.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/contrib/publisherkind"
)

//go:embed embed/cwm
var packFS embed.FS

// DeclareKinds publishes the `publication-connector` kind as provider ("" =
// the host's name): its declaration and its data-type, from the kind's leaf.
// A sidecar shipping publishers in its pack declares the kind through the
// sidecar kit instead; the declarations hash alike.
func DeclareKinds(provider string) application.Extension {
	return builtin.ContributeLeafKinds(provider, publisherkind.Leaf())
}

// Pack publishes the publisher API's data types (cap-publication,
// document-tree) as provider ("" = the host's name).
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("publications: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "publications", "cwm", sub)
}
