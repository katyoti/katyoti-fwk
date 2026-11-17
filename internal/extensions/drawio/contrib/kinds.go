// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes a host's drawio content (notations, viewpoints,
// template libraries) through the generic producer, each kind read as its
// declaration's `source:` block says. The kinds themselves are built-in
// leaves (drawiokind), published by a binary through
// builtin.ContributeLeafKinds.
package contrib

import (
	"fmt"
	"path"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// Kinds are the drawio kinds as their leaf declares them (drawiokind, known
// to every binary through builtin.Kinds()), in publishing order.
func Kinds() []contribapi.KindDeclaration {
	kinds, err := builtin.Kinds()
	if err != nil {
		panic(fmt.Sprintf("drawio: the built-in kinds do not read: %v", err))
	}
	var out []contribapi.KindDeclaration
	for _, name := range []string{drawioapi.ContributionTypeNotation, drawioapi.ContributionTypeViewpoint, drawioapi.ContributionTypeDrawioTemplate} {
		d, ok := kinds.Get(name)
		if !ok || d.Source == nil {
			panic(fmt.Sprintf("drawio: kind %q is not declared with a source", name))
		}
		out = append(out, d)
	}
	return out
}

// Providers are the providers of a host's drawio content under root in ldr,
// published as providerName: one per kind, each reading its declared folder
// (notations/, viewpoints/, drawio-templates/ with the .drawio beside each
// library).
func Providers(providerName string, ldr loader.Loader, root string) []*manifestcontrib.EmbedFSResourceProvider {
	var out []*manifestcontrib.EmbedFSResourceProvider
	for _, d := range Kinds() {
		out = append(out, manifestcontrib.KindProviders(providerName, ldr, path.Join(root, d.SourceFolder()), d)...)
	}
	return out
}

// ContributeDrawio returns the Extension that publishes a host's drawio
// content from ldr, under root, as providerName ("" = the host's name):
//
//	<root>/notations/<id>.yaml        → <provider>:/notation/<id>
//	<root>/viewpoints/<id>.yaml       → <provider>:/viewpoint/<id>
//	<root>/drawio-templates/<id>.yaml → <provider>:/drawio-template/<id>
//	                                    (with <id>.drawio beside it)
func ContributeDrawio(providerName string, ldr loader.Loader, root string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".drawio-resources."+providerName+"."+root, func(app *application.Application) error {
			name := providerName
			if name == "" {
				name = app.Provider()
			}
			for _, p := range Providers(name, ldr, root) {
				if err := application.Contribute(app, contribapi.ContributionProvider(p)); err != nil {
					return err
				}
			}
			return nil
		})
	}
}
