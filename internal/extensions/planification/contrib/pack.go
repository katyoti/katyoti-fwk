// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the planification extension's CWM declarations —
// the `request` type, its field-group and data-type, its workflow, the
// `depends-on` edge and its hooks — from the extension
// itself, not from the sidecar that hosts it.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the planification pack to the binary that hosts it,
// published under `provider` ("" = the host's name) —
// `ktw-agents:/object-type/request`.
func Pack(provider string) application.Extension {
	return manifestcontrib.ContributePackPart(provider, "planification", "cwm", FS())
}

// FS is the pack as its files, for a test that loads it into a CWM app.
func FS() fs.FS {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("planification: embed sub embed/cwm: " + err.Error())
	}
	return sub
}
