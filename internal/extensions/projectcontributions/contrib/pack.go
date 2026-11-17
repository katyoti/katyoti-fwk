// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the project's `contributions` capability: where a
// project keeps the skills, tasks, kinds and packs it contributes to its
// universe (its provider token and its folder) - and the `contributes`
// capability, which publishes any object as an entry of a declared kind,
// served by the object-backed provider (ObjectProvider). A leaf the binary hosting CWM
// imports (ktw-agents) — never a sidecar, which links no CWM pack of its own
// through it. The capability's field-group is generated from
// projectfile.Contributions, the one reader of the settings outside CWM.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the project-contributions pack, published under `provider`
// ("" = the host's name): the `contributions` capability, attached to every
// project object, and its field-group's data-type `cap-contributions`.
func Pack(provider string) application.Extension {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("projectcontributions: embed sub embed/cwm: " + err.Error())
	}
	return manifestcontrib.ContributePackPart(provider, "project-contributions", "cwm", sub)
}
