// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// contributeConnectors publishes a made-up sidecar's connectors the way the
// sidecar kit does: the folder dir of ldr read by the generic pack reader as
// the source-connector kind declares (its kinds from the built-in reader),
// under the sidecar's namespace. Only that folder is read.
func contributeConnectors(provider string, ldr loader.Loader, dir string) application.Extension {
	kinds, err := builtin.Kinds()
	if err != nil {
		panic(err)
	}
	return manifestcontrib.ContributePack(provider, onlyUnder{ldr, dir + "/"}, path.Dir(dir), kinds,
		manifestcontrib.WithRemap(map[string]string{"source-connector": path.Base(dir)}))
}

// onlyUnder narrows a loader's Walk to the files under one folder.
type onlyUnder struct {
	loader.Loader
	prefix string
}

func (o onlyUnder) Walk() []string {
	var out []string
	for _, f := range o.Loader.Walk() {
		if strings.HasPrefix(f, o.prefix) {
			out = append(out, f)
		}
	}
	return out
}
