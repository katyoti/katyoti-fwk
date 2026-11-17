// SPDX-License-Identifier: Apache-2.0

package local

import (
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// SidecarFeature returns the sidecar-kit feature that wires the ScriptService
// runtime together with the loader feeding it from `embed/scripts`.
//
// It lives here rather than in the sidecar kit so that only a binary which
// actually ships scripts imports this package. A boolean kit option would not
// achieve that: Go links by import, so a kit importing this package to honour a
// disabled flag would link the script engine into every sidecar.
func SidecarFeature() func(appName string, efs fs.FS) []application.Extension {
	return SidecarFeatureFrom("", "embed/scripts")
}

// SidecarFeatureFrom is SidecarFeature with an explicit provider name and
// embedded root. An empty provider means "use the binary's own name".
func SidecarFeatureFrom(provider, root string) func(appName string, efs fs.FS) []application.Extension {
	return func(appName string, efs fs.FS) []application.Extension {
		p := provider
		if p == "" {
			p = appName
		}
		return []application.Extension{
			Extension(),
			ContributeScriptLoader(loader.NewEmbedFSLoader(efs, "."), root, p),
		}
	}
}
