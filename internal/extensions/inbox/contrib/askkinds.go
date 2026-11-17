// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"embed"
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// The `ask-kind` kind's declaration, owned by the inbox extension and
// published by the binary hosting it (ktw-agents), and the nine built-in
// kinds of questions it serves itself.
//
//go:embed embed/contribution-types embed/ask-kinds
var kindFS embed.FS

// DeclareKinds publishes the `ask-kind` kind as provider ("" = the host's name).
func DeclareKinds(provider string) application.Extension {
	return manifestcontrib.ContributeContributionTypeResources(provider, loader.NewEmbedFSLoader(kindFS, "."), "embed/contribution-types")
}

// OwnAskKinds publishes the built-in kinds of questions - approve, decide,
// test-by-hand, form, provide, choose, permission, unblock, accept - as
// provider ("" = the host's name).
func OwnAskKinds(provider string) application.Extension {
	return ContributeAskKinds(provider, loader.NewEmbedFSLoader(kindFS, "."), "embed/ask-kinds")
}

// ContributeAskKinds publishes each `<root>/<name>.yaml` in ldr as an
// `ask-kind` entry `<provider>:/ask-kind/<name>`: how a pack adds a kind of
// question. The kind is declared by the inbox extension; a pack publishes it
// as a foreign kind, so the picker fields are written here, from the YAML.
func ContributeAskKinds(provider string, ldr loader.Loader, root string) application.Extension {
	return manifestcontrib.ContributeKindResources(provider, api.ContributionTypeAskKind,
		yamlUnder{ldr, root}, root, manifestcontrib.WithMetadata(askKindMeta))
}

func askKindMeta(_ string, raw []byte) *contribapi.ContributionMeta {
	var k api.AskKind
	if config.DecodeBytes(raw, marshal.YAML, &k) != nil {
		return nil
	}
	return &contribapi.ContributionMeta{Name: k.Name, Description: k.Description}
}

// yamlUnder narrows a loader's Walk to the YAML files directly under root.
type yamlUnder struct {
	loader.Loader
	root string
}

func (w yamlUnder) Walk() []string {
	var out []string
	for _, f := range w.Loader.Walk() {
		rel, ok := strings.CutPrefix(f, w.root+"/")
		ext := strings.ToLower(path.Ext(rel))
		if ok && !strings.Contains(rel, "/") && (ext == ".yaml" || ext == ".yml") {
			out = append(out, f)
		}
	}
	return out
}
