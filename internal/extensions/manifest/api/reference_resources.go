// SPDX-License-Identifier: Apache-2.0

package api

// ResourceTypeReference is the discriminator for arbitrary reference files
// shipped alongside a sidecar — markdown notes, design docs, diagrams,
// images, etc. that the agent can fetch on demand via ResourceService.Get.
//
// References differ from skills in three ways:
//   - No frontmatter: the file is the body, no metadata to extract.
//   - Tree-shaped: identity is the full relative path including extension,
//     so the same loader can serve `diagrams/architecture.md` and
//     `diagrams/architecture.pdf` side-by-side.
//   - Body in the descriptor is metadata only (Path, MediaType). The
//     actual file content is fetched via Get because references can be
//     large and arbitrary-format.
//
// Canonical URI: `<provider>:/reference/<rel-path-with-extension>`
// (e.g. `ktw-docs:/reference/diagrams/architecture.md`).
const ResourceTypeReference = "reference"

// ReferenceBody is the typed body for resources with type "reference".
//
// Intentionally minimal: the descriptor records what references exist
// (so an agent's prompt builder can list them) without inlining every
// file's content. Consumers call ResourceService.Get(uri) when they
// actually need the body.
type ReferenceBody struct {
	// Path is the reference's relative path under the sidecar's
	// references root, e.g. "diagrams/architecture.md". Same as the
	// entry's ID — duplicated here so consumers reading just the body
	// keep the path.
	Path string `json:"path"`
}
