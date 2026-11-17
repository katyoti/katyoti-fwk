// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the inbox extension's CWM declarations — the
// `inbox` and `approval-request` object types — from the extension itself, not
// from the sidecar that hosts it.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the inbox pack to the binary that hosts it, published under
// `provider` ("" = the host's name) — `ktw-agents:/object-type/inbox`.
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("inbox: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "inbox", "cwm", sub)
}
