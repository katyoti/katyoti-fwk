// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the events extension's CWM declarations — the
// event types its sources raise (code.changed, timer.elapsed) and the `timed`
// capability an object declares its recurring timers with — from the extension
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

// FS is the pack's folder (event-types/, capabilities/, data-types/).
func FS() fs.FS {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("events: embed sub embed/cwm: " + err.Error())
	}
	return sub
}

// Pack contributes the events pack to the binary that hosts it, published
// under `provider` ("" = the host's name) — `ktw-agents:/event-type/code.changed`.
func Pack(provider string) application.Extension {
	return manifestcontrib.ContributePackPart(provider, "events", "cwm", FS())
}
