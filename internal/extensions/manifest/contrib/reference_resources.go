// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ContributeReferenceResources returns an Extension that contributes a
// reference ContributionProvider rooted at <root> in <ldr>. Every file under
// the root becomes a `type: reference` entry whose ID is its
// extension-preserving relative path (e.g. `diagrams/architecture.md`).
// The entry's body is a `ReferenceBody{Path, MediaType}` — content is
// fetched separately via ResourceService.Get(uri).
//
// Provider parameter is the sidecar name that hosts these references
// (e.g. "ktw-docs"). Used both as the URI's provider token and as the
// EM contribute key (so contributions don't collide if a binary wires
// multiple reference roots — distinct provider strings differentiate).
//
// Mirrors `ContributeSkillResources`'s shape: thin wrapper over a
// parameterized `EmbedFSResourceProvider`. Replaces the legacy
// `skillslocal.ContributeReferenceLoader` once Phase C of
// `bus-split-resource-provider-ep` deletes the old loader EP.
func ContributeReferenceResources(provider string, ldr loader.Loader, root string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".reference-resources."+provider, func(app *application.Application) error {
			return application.Contribute(app, contribapi.ContributionProvider(&EmbedFSResourceProvider{
				ProviderName:        provider,
				ResourceType:        mbapi.ResourceTypeReference,
				Loader:              ldr,
				Root:                root,
				KeepFolderStructure: true,
				KeepExtension:       true,
				// No mediaType here: it is a picker field and meta has it.
				// Two copies of a media type is two answers to "how do I
				// render this", and a renderer that read the wrong one would
				// be wrong in a way nothing reports.
				BodyDecoder: func(itemPath string, _ []byte) (json.RawMessage, error) {
					return json.Marshal(mbapi.ReferenceBody{Path: itemPath})
				},
				MetadataDecoder: func(itemPath string, _ []byte) *contribapi.ContributionMeta {
					return &contribapi.ContributionMeta{MediaType: detectMediaType(itemPath)}
				},
			}))
		})
	}
}
