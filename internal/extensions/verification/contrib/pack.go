// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the verification extension's CWM declarations —
// the `verification` capability and its two functions — from the extension
// itself.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the verification pack, published under `provider` ("" =
// the host's name).
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("verification: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "verification", "cwm", sub)
}
