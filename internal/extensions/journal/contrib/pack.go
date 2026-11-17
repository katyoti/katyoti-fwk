// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the journal extension's CWM declarations — the
// `journal-day` object type — from the extension itself, not from the sidecar
// that hosts it.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the journal pack to the binary that hosts it, published
// under `provider` ("" = the host's name) — `ktw-agents:/object-type/journal-day`.
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("journal: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "journal", "cwm", sub)
}
