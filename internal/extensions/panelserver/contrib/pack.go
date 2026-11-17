// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the generic panel types - steps
// log, question and chat, internal/core/panel's GenericTypes - as
// `panel-type` contributions of the binary that runs the tasks opening them:
// `ktw-agents:/panel-type/progress` and its siblings, read by the panel
// server (their retention, what they keep) and by any screen like a pack's
// own type. Their declaration is Go's, the one source the panel handle fills
// an open from, laid out as a pack's panel-types/<name>.yaml files.
package contrib

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// Root is the folder the pack's files are under.
const Root = "generic-panel-types"

// Files are the generic types as a pack lays them out:
// <Root>/panel-types/<name>.yaml (JSON, a YAML document), the name the URI's
// last segment.
func Files() (map[string]string, error) {
	out := map[string]string{}
	for uri, decl := range panel.GenericTypes() {
		name := uri[strings.LastIndex(uri, "/")+1:]
		body, err := json.MarshalIndent(decl, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("panel type %s: %w", uri, err)
		}
		out[Root+"/panel-types/"+name+".yaml"] = string(body) + "\n"
	}
	return out, nil
}

// Pack publishes the generic panel types under provider ("" = the host's
// name, ktw-agents).
func Pack(provider string) application.Extension {
	files, err := Files()
	kinds, kerr := contribution.SystemKinds()
	if err == nil {
		err = kerr
	}
	if err != nil {
		return func(init *application.ApplicationInit) {
			init.AddContribute("manifest", ".pack.panel-types", func(*application.Application) error {
				return fmt.Errorf("the generic panel types: %w", err)
			})
		}
	}
	return manifestcontrib.ContributePack(provider, loader.NewMapLoader(files), Root, kinds)
}
