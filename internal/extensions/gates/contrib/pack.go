// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the gates extension's CWM declarations — the
// `gates` capability (the confirmations group and the gate() function) — from
// the extension itself. Gates and gate requirements are contributed by the
// packs that define them (`gates/`, `gate-requirements/`).
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the gates pack, published under `provider` ("" = the
// host's name). Its gates/ and gate-requirements/ are CWM kinds
// (system:/contribution-type/gate, gate-requirement), read by every pack
// reader with no registration.
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("gates: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "gates", "cwm", sub)
}
