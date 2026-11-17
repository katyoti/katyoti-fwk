// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// Pack contributes the slide-deck CWM pack — the `slide-deck` object type, the
// `deck` field-group and its data-type, and the `deck` panel — to the binary
// that hosts it (ktw-docs), published under `provider`.
func Pack(provider string) application.Extension {
	return manifestcontrib.ContributePackPart(provider, "slidedeck", "cwm", PackFS())
}

// PackFS is the pack as its files, for a test that loads it.
func PackFS() fs.FS {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("slidedeck: embed sub embed/cwm: " + err.Error())
	}
	return sub
}
