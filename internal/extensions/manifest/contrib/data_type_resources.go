// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"path"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// DataTypeResourcesOption tunes ContributeDataTypeResources. Use the
// `With*` constructors below — the struct is intentionally not exported.
type DataTypeResourcesOption func(*dataTypeResourcesConfig)

type dataTypeResourcesConfig struct {
	flatSubfolders bool
}

// WithFlatSubfolders collapses sub-folder structure into the URI:
// files under nested directories get an ID that's just the basename
// (without the `.dsl.*` suffix), so a file at
// `data-types/capabilities/cap-decision.dsl.yaml` becomes
// `<provider>:/data-type/cap-decision` rather than
// `<provider>:/data-type/capabilities/cap-decision`. Use this when sub-folders
// are an on-disk organizing aid (e.g. CWM's `capabilities/` for field-groups)
// and the URI surface should stay flat. Default (no option) preserves
// the sub-folder prefix in the URI — appropriate when the sub-folder
// is semantically part of the type's identity.
func WithFlatSubfolders() DataTypeResourcesOption {
	return func(c *dataTypeResourcesConfig) { c.flatSubfolders = true }
}

// ContributeDataTypeResources returns an Extension that contributes a
// data-type ContributionProvider rooted at <root> in <ldr>. Every
// `*.dsl.yaml` or `*.dsl.json` file under <root> becomes a
// `type: data-type` entry whose ID is the file's basename without the
// `.dsl.yaml` / `.dsl.json` extension (e.g. `skill-body`).
//
// Sub-folders under <root> are walked recursively. By default the
// sub-folder path stays in the URI — `<root>/groupA/foo.dsl.yaml`
// becomes `<provider>:/data-type/groupA/foo`. Pass
// `WithFlatSubfolders()` to collapse the sub-folder so the URI is
// `<provider>:/data-type/foo` regardless of nesting. CWM uses this to
// keep capability field-groups under `capabilities/` while keeping flat
// URIs like `ktw-methods:/data-type/cap-decision`.
//
// The hub's catalog Phase-2 wire-up walks these Contributions and
// calls `SchemaService.RegisterDataType` on each Body. After Phase 2,
// every other Contribution whose `Schema` field references one of
// these URIs can be edge-extracted and validated server-side.
//
// Provider is the sidecar name used as the Contribution's provider
// token and the EM contribute key. Pass an empty string to use the
// CURRENT BINARY's name (resolved from `app.Provider()` at Contribute
// time) — that's the right choice for framework-shared data-types
// (e.g. the `skill-body` schema shipped by `skillslocal.Extension()`
// is identical on every binary, so its URI should be
// `<binary>:/data-type/skill-body`, not `skills:/data-type/skill-body`).
// Pass an explicit provider name when contributing data-types that
// SEMANTICALLY belong to a different namespace (rare).
//
// Mirrors `ContributeSkillResources` / `ContributeReferenceResources`:
// thin wrapper over a parameterized `EmbedFSResourceProvider`.
func ContributeDataTypeResources(provider string, ldr loader.Loader, root string, opts ...DataTypeResourcesOption) application.Extension {
	return func(init *application.ApplicationInit) {
		// Use a stable hook key — falls back to "default" when the
		// caller passes an empty provider (resolved-at-runtime case).
		hookKey := provider
		if hookKey == "" {
			hookKey = "default"
		}
		init.AddContribute("manifest", ".data-type-resources."+hookKey, func(app *application.Application) error {
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			return application.Contribute(app, contribapi.ContributionProvider(NewDataTypeProvider(resolved, ldr, root, opts...)))
		})
	}
}

// NewDataTypeProvider is the producer ContributeDataTypeResources registers:
// every DSL file under <root> in <ldr> as one data-type of <provider>.
// Exported for the same reader NewContributionTypeProvider serves — a
// fixture pack's schemas, read without an application through the real
// producer.
func NewDataTypeProvider(provider string, ldr loader.Loader, root string, opts ...DataTypeResourcesOption) *EmbedFSResourceProvider {
	cfg := dataTypeResourcesConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	transform := stripDSLExtension
	if cfg.flatSubfolders {
		transform = stripDSLExtensionFlat
	}
	return &EmbedFSResourceProvider{
		ProviderName:        provider,
		ResourceType:        contribapi.ResourceTypeDataType,
		Loader:              ldr,
		Root:                root,
		KeepFolderStructure: false,
		KeepExtension:       false,
		IDTransform:         transform,
		// Its field labels in other languages: `<name>.dsl.i18n.yaml` beside
		// `<name>.dsl.yaml` (contribution-type data-type, `translatable`).
		SiblingLayers: map[string]string{contribapi.LayerTranslations: TranslationsSuffix},
		// No BodyDecoder: a data-type's content IS its DSL, and the
		// source layer carries it. Residency is the kind's declaration
		// (`system:/contribution-type/data-type`: source resident), applied
		// by the manifest builder — not a field here.
		MetadataDecoder: func(itemPath string, _ []byte) *contribapi.ContributionMeta {
			return &contribapi.ContributionMeta{MediaType: dataTypeMediaType(itemPath)}
		},
	}
}

// stripDSLExtension drops the trailing `.dsl.yaml` / `.dsl.json` /
// `.dsl.yml` suffix from a data-type file name so the Contribution ID
// is just the type name (e.g. `skill-body.dsl.yaml` → `skill-body`).
// Falls back to the standard extension strip for files that don't
// match the `.dsl.*` convention. Sub-folder path (if any) is preserved
// in the returned ID; pair with `WithFlatSubfolders()` /
// `stripDSLExtensionFlat` to collapse it instead.
func stripDSLExtension(name string) string {
	lower := strings.ToLower(name)
	for _, suf := range []string{".dsl.yaml", ".dsl.yml", ".dsl.json"} {
		if strings.HasSuffix(lower, suf) {
			return name[:len(name)-len(suf)]
		}
	}
	return strings.TrimSuffix(name, path.Ext(name))
}

// stripDSLExtensionFlat strips both the sub-folder prefix AND the
// `.dsl.*` suffix so a file at `capabilities/cap-decision.dsl.yaml`
// becomes the flat ID `cap-decision`. Selected by `WithFlatSubfolders()` in
// `ContributeDataTypeResources`. Subfolder collision (two files with
// the same basename in different folders) is the caller's
// responsibility — first writer wins, the second is dropped silently
// by the contribution map's overwrite semantics.
func stripDSLExtensionFlat(name string) string {
	return stripDSLExtension(path.Base(name))
}

// dataTypeFormat returns "yaml" or "json" from the file path. Empty
// when the extension is unrecognized — SchemaService.RegisterDataType
// then falls back to auto-detection.
func dataTypeFormat(itemPath string) string {
	lower := strings.ToLower(itemPath)
	switch {
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return "yaml"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	}
	return ""
}

// dataTypeMediaType names the DSL's serialisation from the file's extension.
//
// The ONE statement of the format. It used to be said twice — here and in the
// body envelope's `format` field — which is two answers to one question and a
// place for them to disagree.
func dataTypeMediaType(itemPath string) string {
	if dataTypeFormat(itemPath) == "json" {
		return "application/json"
	}
	return "application/yaml"
}
