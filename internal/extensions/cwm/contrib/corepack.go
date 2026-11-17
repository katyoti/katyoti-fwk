// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var corePackFS embed.FS

// CorePack contributes what CWM itself declares — the `object-core` data-type
// its Identity tab edits the object core with, the `about` edge — so any
// binary that hosts
// CWM has them, whatever packs are loaded beside it. Published under
// `provider` ("" = the host's name): `ktw-agents:/data-type/object-core`.
func CorePack(provider string) application.Extension {
	sub, err := fs.Sub(corePackFS, "embed/cwm")
	if err != nil {
		panic("cwm: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "cwm-core", packPath, sub)
}
