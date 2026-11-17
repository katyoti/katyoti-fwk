// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"gopkg.in/yaml.v3"
)

// ContributeKindResources returns an Extension that publishes every file under
// <root> in <ldr> as one contribution of <kind> — ANY kind, including one no Go
// code has ever heard of.
//
// This is the generic producer behind "a new kind needs a declaration and a
// provider, nothing else": the declaration is a `contribution-type` entry the
// pack ships (see ContributeContributionTypeResources), and this is the
// provider. Nothing here knows what the kind means.
//
// Each entry carries up to two layers, plus any sibling layer asked for:
//
//   - `source` — the file as written, addressed by hash (always);
//   - `data` — the file's structure as canonical JSON, when the file is YAML or
//     JSON. That is the layer the kind's schema validates and the one a
//     consumer decodes. A file of any other format has no structure to offer
//     and carries its source only;
//   - a sibling layer (WithSiblingLayer) — the file of the same name with
//     another extension, e.g. a display engine's widget page `landscape.html`
//     beside `landscape.yaml`. That file is part of its entry, not an entry;
//   - `translations` — the sibling `<name>.i18n.yaml`, always: the entry's
//     texts in other languages (contribapi translations.go). Every kind gets
//     it without asking; the content lint refuses it on a kind that declares
//     nothing translatable.
//
// The ID is the path under <root> without its extension (`breakfast/pancakes`
// for `breakfast/pancakes.yaml`). Picker fields are not written here: a kind
// that wants them declares `meta.from: data` and the manifest builder derives
// meta from this data layer — one writer.
func ContributeKindResources(provider, kind string, ldr loader.Loader, root string, opts ...KindResourceOption) application.Extension {
	o := kindResourceOptions{siblings: map[string]string{contribapi.LayerTranslations: TranslationsSuffix}}
	for _, opt := range opts {
		opt(&o)
	}
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".kind-resources."+provider+"."+kind, func(app *application.Application) error {
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			return application.Contribute(app, contribapi.ContributionProvider(&EmbedFSResourceProvider{
				ProviderName:        resolved,
				ResourceType:        kind,
				Loader:              ldr,
				Root:                root,
				KeepFolderStructure: true,
				KeepExtension:       false,
				BodyDecoder:         structuredDataLayer,
				MetadataDecoder:     o.metadata,
				SiblingLayers:       o.siblings,
			}))
		})
	}
}

// TranslationsSuffix is the sibling file carrying an entry's translations
// layer: `<name>.i18n.yaml` beside `<name>.yaml`.
const TranslationsSuffix = ".i18n.yaml"

// TranslationsLayer converts an entry's `<name>.i18n.yaml` to the
// translations layer it publishes: canonical JSON. One conversion for every
// producer that attaches translations (kind resources, CWM packs).
func TranslationsLayer(label string, raw []byte) (json.RawMessage, error) {
	return yamlDocToJSON(label, raw)
}

// KindResourceOption tunes ContributeKindResources.
type KindResourceOption func(*kindResourceOptions)

type kindResourceOptions struct {
	siblings map[string]string
	metadata func(itemPath string, raw []byte) *contribapi.ContributionMeta
}

// WithMetadata writes each entry's meta from its file. For a kind that other
// members publish as a FOREIGN kind: derived meta (`meta.from: data`) is
// filled by the builder of the member that declares the kind, so an entry
// published elsewhere would carry none. Such a kind declares no `meta.from`,
// and its producers write the picker fields here instead — from the same
// file, so there is still one source.
func WithMetadata(fn func(itemPath string, raw []byte) *contribapi.ContributionMeta) KindResourceOption {
	return func(o *kindResourceOptions) { o.metadata = fn }
}

// WithSiblingLayer carries the file `<name><ext>` beside each entry
// `<name>.yaml` as the entry's `layer` (e.g. "widget", ".html"). The kind
// declares that layer; the sibling file is not an entry of its own.
func WithSiblingLayer(layer, ext string) KindResourceOption {
	return func(o *kindResourceOptions) {
		if o.siblings == nil {
			o.siblings = map[string]string{}
		}
		o.siblings[layer] = ext
	}
}

// structuredDataLayer derives the `data` layer of a file-backed entry: the
// file's structure as canonical JSON for YAML and JSON files, nothing for any
// other format (a nil body adds no layer).
func structuredDataLayer(itemPath string, raw []byte) (json.RawMessage, error) {
	switch strings.ToLower(path.Ext(itemPath)) {
	case ".yaml", ".yml":
		return yamlDocToJSON(itemPath, raw)
	case ".json":
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return nil, fmt.Errorf("%s: json: %w", itemPath, err)
		}
		return buf.Bytes(), nil
	default:
		return nil, nil
	}
}

// yamlDocToJSON converts one YAML document to canonical JSON. Content, not
// configuration: a contribution's structure is published as written, with no
// variable expansion — which is why this is not config.DecodeBytes.
func yamlDocToJSON(label string, raw []byte) (json.RawMessage, error) {
	var tree any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("%s yaml: %w", label, err)
	}
	b, err := json.Marshal(stringKeys(tree))
	if err != nil {
		return nil, fmt.Errorf("%s json marshal: %w", label, err)
	}
	return b, nil
}

// stringKeys converts every nested map[any]any to map[string]any: yaml.v3
// decodes a mapping whose keys include a non-string scalar (`cases: {true:
// 3}`) as map[any]any, which JSON cannot marshal.
func stringKeys(v any) any {
	switch m := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = stringKeys(val)
		}
		return out
	case map[string]any:
		for k, val := range m {
			m[k] = stringKeys(val)
		}
		return m
	case []any:
		for i, val := range m {
			m[i] = stringKeys(val)
		}
		return m
	default:
		return v
	}
}
