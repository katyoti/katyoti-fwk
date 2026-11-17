// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"gopkg.in/yaml.v3"
)

// contributionTypeDiscriminator is the Type value of a contribution-type
// entry. The manifest Builder keys off it to derive the body-schema URI
// (`<provider>:/data-type/<type>`) for every contribution of a kind this
// provider registers.
const contributionTypeResourceType = "contribution-type"

// ContributeContributionTypeResources returns an Extension that
// registers one `contribution-type` Contribution per `<name>.yaml` file
// under <root> in <ldr>. Each file declares a kind the SIDECAR owns
// (`name` + `description` frontmatter); its presence in the manifest is
// what makes the Builder derive `Schema = <provider>:/data-type/<name>`
// for every contribution of that kind.
//
// The system set (skill/task/reference/data-type/...) ships as
// system:/contribution-type/* embedded in every binary; this helper lets
// a sidecar register its OWN kinds (e.g. ktw-docs's `notation`) so the
// type→data-type convention applies to them too.
//
// Body is converted from YAML to canonical JSON at ingest so consumers
// decode a structured descriptor directly.
func ContributeContributionTypeResources(provider string, ldr loader.Loader, root string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".contribution-type-resources."+provider, func(app *application.Application) error {
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			return application.Contribute(app, contribapi.ContributionProvider(NewContributionTypeProvider(resolved, ldr, root)))
		})
	}
}

// NewContributionTypeProvider is the producer ContributeContributionTypeResources
// registers: every YAML file under <root> in <ldr> as one kind declaration of
// <provider>. Exported so a reader with no application — the census
// resolving a binary's foreign kinds against a fixture pack's declarations —
// reads a declaration through the same parser the pack's own host uses,
// rather than through a copy of it.
func NewContributionTypeProvider(provider string, ldr loader.Loader, root string) *EmbedFSResourceProvider {
	return &EmbedFSResourceProvider{
		ProviderName:        provider,
		ResourceType:        contributionTypeResourceType,
		Loader:              ldr,
		Root:                root,
		KeepFolderStructure: false,
		KeepExtension:       false,
		FileExtensions:      []string{"yaml", "yml"},
		BodyDecoder:         contributionTypeYAMLToJSON,
		// A kind's name and description are picker fields like any
		// other, and meta is where picker fields live. They were only
		// in the body, so a listing of kinds showed a blank name —
		// masked by the identity-derived fallback, which is exactly
		// how an empty field survives unnoticed.
		MetadataDecoder: contributionTypeMeta,
	}
}

// contributionTypeYAMLToJSON converts a contribution-type YAML descriptor
// to canonical JSON for storage as the Contribution's data layer.
func contributionTypeYAMLToJSON(_ string, raw []byte) (json.RawMessage, error) {
	return yamlDocToJSON(contributionTypeResourceType, raw)
}

// contributionTypeMeta lifts a kind declaration's picker fields into meta.
//
// The declaration YAML carries `name` and `description`; the name is the kind's
// identity restated (it always equals the entry's ID) and the description is a
// picker field. Reading them here means a listing of kinds shows what the kind
// says about itself, without fetching a body per row.
func contributionTypeMeta(itemPath string, raw []byte) *contribapi.ContributionMeta {
	var decl struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	_ = yaml.Unmarshal(raw, &decl)
	if decl.Name == "" {
		decl.Name = strings.TrimSuffix(strings.TrimSuffix(itemPath, ".yaml"), ".yml")
	}
	return &contribapi.ContributionMeta{
		Name:        decl.Name,
		Description: strings.TrimSpace(decl.Description),
		MediaType:   "application/json",
	}
}
