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

// SkillProvider is the provider of the `skill` kind over <ldr>/<root>: the
// generic producer (manifestcontrib.EmbedFSResourceProvider) reading the
// kind's declared `source:` block (the built-in skill declaration) — a
// markdown file split once into its frontmatter (data), its body
// (instructions) and itself (source), addressed by its name whatever its
// layout, a skill folder's references, scripts and assets left out, a file
// that cannot be read as a skill dropped and recorded.
//
// No skill code here: how a skill is laid out is the kind's declaration, so
// this package links no skill parser (rule producers-link-no-kind-package
// holds the generic producer; this is wiring). Unadvertised skills are
// published — visibility is filtered at list time (`api.SkillsFromCatalog`).
func SkillProvider(provider string, ldr loader.Loader, root string) *manifestcontrib.EmbedFSResourceProvider {
	return manifestcontrib.KindProvider(provider, ldr, root, skillKind())
}

// skillKind is the skill kind as this binary knows it: the built-in reader's
// declaration (the skills pack's own, skillkind.Leaf, once the reader carries
// it; the system set's until then). Compiled into the binary: a declaration
// that does not decode is a broken build, reported loudly by every
// enumeration rather than here.
func skillKind() contribapi.KindDeclaration {
	kinds, err := builtin.Kinds()
	if err != nil {
		return contribapi.KindDeclaration{Name: mbapi.ResourceTypeSkill}
	}
	d, ok := kinds.Get(mbapi.ResourceTypeSkill)
	if !ok || d.Source == nil {
		panic(fmt.Sprintf("skills: the built-in kinds declare no source for %q", mbapi.ResourceTypeSkill))
	}
	return d
}

// ContributeSkillResources returns an Extension that contributes the skill
// provider of <loader>/<root> to the manifest's provider extension point, so
// every skill there appears as a `type: skill` entry in this binary's
// manifest.
func ContributeSkillResources(provider string, ldr loader.Loader, root string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("skills", ".Contributions."+provider, func(app *application.Application) error {
			return application.Contribute(app, contribapi.ContributionProvider(SkillProvider(provider, ldr, root)))
		})
	}
}
