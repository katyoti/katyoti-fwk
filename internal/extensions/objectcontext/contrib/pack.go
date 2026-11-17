// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes object-context's CWM declarations from the
// extension itself: the `context` capability (a type's `context:` section and
// its type_schema). Its own kind, `agent-context`, is a CWM kind every pack
// reader reads from a pack's agent-contexts/.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the object-context pack, published under `provider` ("" =
// the host's name).
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("objectcontext: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "object-context", "cwm", sub)
}
