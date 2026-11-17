// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes this binary's CWM type pack as contributions.
//
// It is a LEAF: it depends on the contribution contract, the generic producer
// and the extension mechanism — never on the CWM runtime that consumes these
// types. Publishing a folder of type definitions must not link the engine
// that interprets them, and the linker's unit is the package, so the separation
// has to be physical.
package contrib

import (
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// ContributeCWMTypes publishes a sidecar's OWN CWM pack (its `embed/cwm`)
// as catalog Contributions, read directly from the FS — independent of any
// live CWMService, so a pack-only sidecar contributes without hosting one.
//
// Wire shapes:
//
//	URI:    <provider>:/<kind>/<name>
//	Type:   the kind whose `source:` block names the folder
//	Schema: system:/data-type/<kind>
//	Body:   the declaration file's tree as JSON
//
// It is one part of the provider's pack (manifestcontrib.ContributePackPart):
// the extensions the sidecar hosts contribute theirs as plain pack parts
// (path "cwm"), and ONE provider per kind publishes them all, the universe override tree
// last. Pass `ownFS == nil` when the
// binary ships no pack of its own; `provider == ""` means the binary's name.
//
// Unlike an extension's plain part it leaves the pack's data-types out: its
// caller publishes them itself (ContributeDataTypeResources), and
// doing it twice would publish every schema twice.
func ContributeCWMTypes(provider string, ownFS fs.FS) application.Extension {
	return func(init *application.ApplicationInit) {
		owner := provider
		if owner == "" {
			owner = "self"
		}
		if ownFS == nil {
			ownFS = emptyFS{}
		}
		manifestcontrib.ContributePackPart(provider, owner, packPath, withoutDataTypes{ownFS})(init)
	}
}

// emptyFS is the pack of a binary that ships none of its own.
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
