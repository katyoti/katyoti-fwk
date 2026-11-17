// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"io/fs"

	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// OverlayFS returns an fs.FS rooted at a sidecar's workspace CWM override dir
// (`.katyoti/overwrites/<provider>/embed/cwm`) when the universe opts in
// (`allow_overwrites`) AND that dir exists; else nil.
//
// It is passed as the LAST source to `storage.LoadTypesLayered`, which is
// last-source-wins: an overlay type shadows the embedded one, a new overlay type
// is added, and a malformed overlay YAML is skipped (handled in
// loadNodeTypesFS) so it falls back to the embedded asset. nil sources are
// ignored by LoadTypesLayered, so callers pass the result through directly.
//
// This mirrors the skill/task overlay (per-sidecar, at-source, flows through the
// catalog) — distinct from the native `<HomeDir>/cwm/types` global store-host
// override, which is left untouched. It is the generic pack override tree of the `cwm` pack, exported so
// the CWM runtime layers the same tree the pack reader does — one
// definition, not two.
func OverlayFS(homeDir, provider string) fs.FS {
	return manifestcontrib.PackOverlayFS(homeDir, provider, packPath)
}

// FloorOverlays are the override trees of a provider's own CWM types, layered
// after its embedded types (last wins): the universe's alone, gated by
// allow_overwrites - the operator's last word (decisions
// ). A
// project's overrides of CWM types are override objects published through
// the catalog; its per-file `overwrites/<provider>/` tree is no longer read
// (it retires through the data migration). The store's floor and the
// provider's contribution both layer exactly these.
func FloorOverlays(homeDir, provider string) []fs.FS {
	if ov := OverlayFS(homeDir, provider); ov != nil {
		return []fs.FS{ov}
	}
	return nil
}
