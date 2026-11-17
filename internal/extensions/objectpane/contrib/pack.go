// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the object pane's CWM declarations from the
// extension itself: the `pane` capability (a type's `pane:` section and its
// type_schema). Its own kind, `object-panel`, is a CWM kind every pack
// reader reads from a pack's object-panels/.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the object-pane pack, published under `provider` ("" = the
// host's name).
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("objectpane: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "object-pane", "cwm", sub)
}
