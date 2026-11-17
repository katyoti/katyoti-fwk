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

// TaskProviders are the providers of the `task` kind over <ldr>/<root>: the
// generic producer (manifestcontrib.EmbedFSResourceProvider) reading the
// kind's declared `source:` block (the built-in task declaration) — a
// TASK.md, the entry of its folder, or a flat <name>.md; its frontmatter as
// written the data layer with its body folded in as `body`, the file the
// source, `<stem>.i18n.yaml` its translations — and, as its companions, the
// providers of its input and output contracts (`<stem>.input.dsl.yaml`,
// `<stem>.output.dsl.yaml`) published as the data-types
// `<provider>:/data-type/task/<name>/input|output`.
//
// No task code here: how a task file becomes its layers is the kind's
// declaration. The task engine runs what the catalog serves from them
// (CatalogTask: the data layer, an override's included, and the contracts'
// source layers); the readers of the catalog apply the defaults the parser
// applied (task/api).
func TaskProviders(provider string, ldr loader.Loader, root string) []*manifestcontrib.EmbedFSResourceProvider {
	return manifestcontrib.KindProviders(provider, ldr, root, taskKind())
}

// taskKind is the task kind as this binary knows it: the built-in reader's
// declaration (the task pack's own, taskkind.Leaf, once the reader carries
// it; the system set's until then). Compiled into the binary: a declaration
// that does not decode is a broken build, reported loudly by every
// enumeration rather than here.
func taskKind() contribapi.KindDeclaration {
	kinds, err := builtin.Kinds()
	if err != nil {
		return contribapi.KindDeclaration{Name: mbapi.ResourceTypeTask}
	}
	d, ok := kinds.Get(mbapi.ResourceTypeTask)
	if !ok || d.Source == nil {
		panic(fmt.Sprintf("task: the built-in kinds declare no source for %q", mbapi.ResourceTypeTask))
	}
	return d
}

// ContributeTaskResources returns an Extension that contributes the task
// providers of <loader>/<root> — the tasks and their contracts — to the
// manifest's provider extension point.
func ContributeTaskResources(provider string, ldr loader.Loader, root string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("task", ".Contributions."+provider, func(app *application.Application) error {
			for _, p := range TaskProviders(provider, ldr, root) {
				if err := application.Contribute(app, contribapi.ContributionProvider(p)); err != nil {
					return err
				}
			}
			return nil
		})
	}
}
