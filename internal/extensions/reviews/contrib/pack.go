// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the reviews extension's CWM declarations — the
// `review` and `finding` object types, their fields' data-types and
// workflows, the `reviewable` capability and the Reviews and Review panels —
// from the extension itself, not from the sidecar that hosts it.
package contrib

import (
	"embed"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the reviews pack to the binary that hosts it, published
// under `provider` ("" = the host's name) — `ktw-agents:/object-type/review`.
func Pack(provider string) application.Extension {
	return manifestcontrib.ContributePackPart(provider, "reviews", "cwm", FS())
}

// FS is the pack as its files, for a test that loads it into a CWM app.
func FS() fs.FS {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("reviews: embed sub embed/cwm: " + err.Error())
	}
	return sub
}
