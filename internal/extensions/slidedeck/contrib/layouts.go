// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the slide-deck engine's contributions from the
// host that wires it (ktw-docs): the built-in layouts as `deck-layout`
// contributions, and the CWM pack (the slide-deck object type).
package contrib

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"gopkg.in/yaml.v3"
)

// The built-in layouts: one declaration per file, `<name>.yaml`. Their
// styling ships with the deck component in ktw-display-lib.
//
//go:embed layouts/*.yaml
var layoutsFS embed.FS

// The CWM pack: the slide-deck object type, its field-group and its panel.
//
//go:embed embed/cwm
var packFS embed.FS

// ContributeLayouts publishes every built-in layout as
// `<provider>:/deck-layout/<name>`. The kind itself is declared by the host's
// pack (ktw-docs: embed/contribution-types/deck-layout.yaml).
func ContributeLayouts(provider string) application.Extension {
	return manifestcontrib.ContributeKindResources(provider, deckapi.ContributionTypeDeckLayout,
		loader.NewEmbedFSLoader(layoutsFS, "."), "layouts")
}

// BuiltinLayouts decodes the built-in layouts, as the deck-layout data layer
// is read (the YAML's structure, by the JSON names the data-type publishes):
// what a theme's sample deck is checked against without a catalog.
//
// It reads the embedded files on purpose, never the catalog: its callers
// (CheckSlideGroups, slidedeck/lint) are build-time checks a sidecar runs in its
// tests and census on the files it ships, where no universe exists and no
// project's override entry can apply. A runtime reader reads the catalog
// lanes instead (deckapi.Layouts, GrammarAt, LayoutsInReach).
func BuiltinLayouts() ([]deckapi.LayoutBody, error) {
	entries, err := layoutsFS.ReadDir("layouts")
	if err != nil {
		return nil, err
	}
	var out []deckapi.LayoutBody
	for _, e := range entries {
		raw, err := layoutsFS.ReadFile("layouts/" + e.Name())
		if err != nil {
			return nil, err
		}
		var tree any
		if err := yaml.Unmarshal(raw, &tree); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		js, _ := json.Marshal(tree)
		var l deckapi.LayoutBody
		if err := json.Unmarshal(js, &l); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, l)
	}
	return out, nil
}
