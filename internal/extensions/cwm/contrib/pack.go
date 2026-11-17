// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"io/fs"
	"strings"
)

// A CWM pack — an `embed/cwm` tree holding object types, capabilities and
// their data-types, aspects, edge types, workflows, policies, hooks, and the
// folders of the kinds extensions own (a gate, an object panel, an agent
// context) — is one part of its provider's pack, read by the generic pack
// reader (manifestcontrib.ContributePackPart): one provider per kind over
// every part of the provider, each folder read as the kind whose `source:`
// names it, the universe override tree (`overwrites/<provider>/embed/cwm`)
// last, a clash between two parts or a folder no kind reads refused naming
// the owners. Nothing here lists a kind.
//
// A pack belongs to whoever owns its concepts: a sidecar (its own
// `embed/cwm`) or an EXTENSION the sidecar hosts
// (`internal/extensions/<ext>/contrib/embed/cwm`). URIs stay
// `<provider>:/<kind>/<name>` wherever a declaration lives, so moving a type
// from a sidecar pack into its extension changes nothing a consumer can see.
// An extension ships its pack as a plain part of its host's pack
// (manifestcontrib.ContributePackPart(provider, owner, "cwm", fsys)): the host
// needs no wiring beyond importing the extension, and the extension links no
// CWM package. A pack names what its host serves with the empty provider —
// the URI grammar's "this binary" (`:/data-type/type-verification`,
// `:verification/verify-function`) — and the published declaration carries
// the host's name in its place (manifestcontrib.ResolveHostURIs), so the pack
// works under whichever provider hosts it.

// packPath is a CWM pack's place in a binary's embed tree — its override
// subtree under `overwrites/<provider>/embed/`.
const packPath = "cwm"

// dataTypesFolder holds a pack's data-types. A sidecar's own pack
// (ContributeCWMTypes) leaves it out of its part, because the sidecar's
// bootstrap publishes it flat (`data-types/capabilities/x` → `data-type/x`).
// An extension's pack is a plain part (manifestcontrib.ContributePackPart,
// path "cwm"): its flat `data-types/` folder is read by the data-type kind.
const dataTypesFolder = "data-types"

// withoutDataTypes is a pack without its `data-types/` folder, which the
// sidecar's bootstrap publishes flat beside the generic reader.
type withoutDataTypes struct{ fs.FS }

func isDataTypes(name string) bool {
	return name == dataTypesFolder || strings.HasPrefix(name, dataTypesFolder+"/")
}

func (f withoutDataTypes) Open(name string) (fs.File, error) {
	if isDataTypes(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	file, err := f.FS.Open(name)
	if err != nil || name != "." {
		return file, err
	}
	if dir, ok := file.(fs.ReadDirFile); ok {
		return rootDir{dir}, nil
	}
	return file, nil
}

func (f withoutDataTypes) ReadDir(name string) ([]fs.DirEntry, error) {
	if isDataTypes(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries, err := fs.ReadDir(f.FS, name)
	if name != "." {
		return entries, err
	}
	return dropDataTypes(entries), err
}

// rootDir is the pack's root without its `data-types/` entry.
type rootDir struct{ fs.ReadDirFile }

func (d rootDir) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := d.ReadDirFile.ReadDir(n)
	return dropDataTypes(entries), err
}

func dropDataTypes(entries []fs.DirEntry) []fs.DirEntry {
	out := entries[:0:0]
	for _, e := range entries {
		if e.Name() != dataTypesFolder {
			out = append(out, e)
		}
	}
	return out
}
