// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes what the document engine contributes: content
// templates (the `content-template` kind, from any sidecar that ships some)
// and its CWM pack (the `content-template` object type and capability).
package contrib

import (
	"embed"
	"io/fs"
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"gopkg.in/yaml.v3"
)

//go:embed embed/cwm
var packFS embed.FS

// Pack contributes the document engine's CWM pack to the binary that hosts
// it (ktw-docs), published under provider.
func Pack(provider string) application.Extension {
	return manifestcontrib.ContributePackPart(provider, "docs", "cwm", PackFS())
}

// PackFS is the pack as its files, for a test that loads it.
func PackFS() fs.FS {
	sub, err := fs.Sub(packFS, "embed/cwm")
	if err != nil {
		panic("docs: embed sub embed/cwm: " + err.Error())
	}
	return sub
}

// ContributeTemplates publishes each `<root>/<name>.yaml` in ldr as a
// `content-template` entry `<provider>:/content-template/<name>`. The kind is
// declared by ktw-docs; a sidecar that ships templates publishes them as a
// foreign kind, so the picker fields are written here, from the YAML.
func ContributeTemplates(provider string, ldr loader.Loader, root string) application.Extension {
	return manifestcontrib.ContributeKindResources(provider, docsapi.ContributionTypeContentTemplate,
		yamlUnder{ldr, root}, root, manifestcontrib.WithMetadata(templateMeta))
}

func templateMeta(_ string, raw []byte) *contribapi.ContributionMeta {
	var t struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal(raw, &t) != nil {
		return nil
	}
	return &contribapi.ContributionMeta{Name: t.Name, Description: t.Description}
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
