// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the comments extension's CWM declaration — the
// `comments` capability, attached to every object, with its thread file —
// from the extension itself.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the comments pack, published under `provider` ("" = the
// host's name).
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("comments: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "comments", "cwm", sub)
}
