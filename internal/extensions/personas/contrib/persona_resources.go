// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// PersonaProvider is the provider of the `persona` kind over <ldr>/<root>: the
// generic producer (manifestcontrib.EmbedFSResourceProvider) reading the
// kind's declared `source:` block (the built-in persona declaration) — a
// TASK.md whose frontmatter says `category: persona`, its frontmatter the data
// layer (with the JSON names the persona library decodes), its body the
// instructions folded into data as `body`, addressed by its name. A task file
// of another category is not a persona; a file that cannot be read is dropped
// and recorded. The persona library (personas/internal/core) folds an
// augmentation into its baseline.
func PersonaProvider(provider string, ldr loader.Loader, root string) *manifestcontrib.EmbedFSResourceProvider {
	return manifestcontrib.KindProvider(provider, ldr, root, personaKind())
}

// personaKind is the persona kind as this binary knows it: the built-in
// reader's declaration (the personas pack's own, personakind.Leaf, once the
// reader carries it; the system set's until then). Compiled into the binary:
// a declaration that does not decode is a broken build, reported loudly by
// every enumeration rather than here.
func personaKind() contribapi.KindDeclaration {
	kinds, err := builtin.Kinds()
	if err != nil {
		return contribapi.KindDeclaration{Name: mbapi.ResourceTypePersona}
	}
	d, ok := kinds.Get(mbapi.ResourceTypePersona)
	if !ok || d.Source == nil {
		panic(fmt.Sprintf("personas: the built-in kinds declare no source for %q", mbapi.ResourceTypePersona))
	}
	return d
}

// ContributePersonaLoader returns an Extension that contributes the persona
// provider of <loader>/<root> to the manifest's provider extension point, so
// every persona there appears as a `type: persona` entry in the host's
// manifest.
//
//	personascontrib.ContributePersonaLoader("ktw-methods", loader.NewEmbedFSLoader(efs, "."), "embed/personas")
func ContributePersonaLoader(provider string, ldr loader.Loader, root string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("personas", ".Contributions."+provider, func(app *application.Application) error {
			return application.Contribute(app, contribapi.ContributionProvider(PersonaProvider(provider, ldr, root)))
		})
	}
}
