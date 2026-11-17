// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"embed"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib/connectorkind"
)

// The connectors the sources extension serves itself (local-folder,
// documents): a pack holding the kind's folder, read like any connector's.
//
//go:embed embed/source-connectors
var ownFS embed.FS

// DeclareKinds publishes the `source-connector` kind as provider ("" = the
// host's name): its declaration and its data-type, from the kind's leaf. A
// sidecar shipping connectors in its pack declares the kind through the
// sidecar kit instead; the declarations hash alike.
func DeclareKinds(provider string) application.Extension {
	return builtin.ContributeLeafKinds(provider, connectorkind.Leaf())
}

// OwnConnectors publishes the connectors the sources extension serves, as
// provider ("" = the host's name): the pack's source-connectors folder, read
// by the generic pack reader as the kind declares.
func OwnConnectors(provider string) application.Extension {
	kinds, err := builtin.Kinds()
	if err != nil {
		return func(init *application.ApplicationInit) {
			init.AddContribute("sources", ".own-connectors", func(*application.Application) error {
				return fmt.Errorf("sources connectors: the built-in kinds: %w", err)
			})
		}
	}
	return manifestcontrib.ContributePack(provider, loader.NewEmbedFSLoader(ownFS, "."), "embed", kinds)
}
