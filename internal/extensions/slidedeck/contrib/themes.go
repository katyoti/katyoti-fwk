// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// The built-in theme: katyoti, the deck component's own defaults, laid out
// as a pack: `themes/deck-themes/katyoti.yaml` and its files under
// `themes/deck-assets/katyoti/`.
//
//go:embed themes
var themesFS embed.FS

// ContributeBuiltinThemes publishes the built-in themes (katyoti) from the
// host that wires the slide-deck engine, through the generic pack reader:
// each folder read as the built-in deck kind that declares it (deckkind). A
// sidecar that ships its own theme lays out the same folders at its pack
// root and publishes them with no slide-deck code. The kinds are read when
// the application is built, never when the extension list is assembled.
func ContributeBuiltinThemes(provider string) application.Extension {
	return func(init *application.ApplicationInit) {
		kinds, err := builtin.Kinds()
		if err != nil {
			init.AddContribute("slidedeck", ".builtin-themes", func(*application.Application) error {
				return fmt.Errorf("slide-deck built-in themes: the built-in kinds: %w", err)
			})
			return
		}
		manifestcontrib.ContributePack(provider, loader.NewEmbedFSLoader(themesFS, "."), "themes", kinds)(init)
	}
}

// BuiltinThemesFS holds the built-in themes as a pack under `themes/`
// (deck-themes/, deck-assets/), for a test that checks them.
func BuiltinThemesFS() fs.FS { return themesFS }
