// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"bytes"
	"strings"
)

// OverlayLoader serves files from an OVERLAY (e.g. a workspace
// `.katyoti/overwrites/<sidecar>/<type>` dir) on top of a BASE (e.g. a sidecar's
// embedded FS). The overlay WINS per path — but only when the overlay file passes
// the `accept` guard. A malformed overlay entry (rejected by accept) falls back to
// the base, so a broken override never kills the resource.
//
// This differs from CompoundLoader (unconditional last-wins): the accept guard is
// what makes a corrupt/empty/frontmatter-less override transparently fall through
// to the released embed copy instead of shadowing it with garbage.
//
// List/Walk return the overlay-first union (a path is listed once), so a new
// overlay-only resource shows up and an overridden one is not duplicated.
type OverlayLoader struct {
	base    Loader
	overlay Loader
	accept  func(path string, data []byte) bool
	// name is what Origin answers for a path the overlay serves; refuse,
	// when set, says why a well-formed overlay file must not be served (a
	// policy, not a format: Refusal reports it).
	name   string
	refuse func(path string) string
}

// OriginSource is a loader that says which layer serves a path: "" for its
// base, else the name its overlay was given (NamedOverlay).
type OriginSource interface {
	Origin(path string) string
}

// RefusalSource is a loader that says why a layer's copy of a path was not
// served although it is there and well-formed ("" when it was not refused).
type RefusalSource interface {
	Refusal(path string) string
}

// NamedOverlay is NewOverlayLoader with a name — Origin answers it for the
// paths the overlay serves — and a refuse policy: a path it gives a reason
// for is served from the base instead, and Refusal says why.
func NamedOverlay(base, overlay Loader, name string, refuse func(path string) string) *OverlayLoader {
	o := NewOverlayLoader(base, overlay, nil)
	o.name, o.refuse = name, refuse
	return o
}

// Origin implements OriginSource: the overlay's name when it serves path,
// else what the base answers.
func (o *OverlayLoader) Origin(path string) string {
	if _, ok := o.overlayHit(path); ok {
		return o.name
	}
	if b, ok := o.base.(OriginSource); ok {
		return b.Origin(path)
	}
	return ""
}

// Refusal implements RefusalSource: the refuse policy's reason when the
// overlay holds an accepted copy of path it refuses, else the base's answer.
func (o *OverlayLoader) Refusal(path string) string {
	if o.refuse != nil {
		if data, err := o.overlay.ReadFile(path); err == nil && data != nil && o.accept(path, data) {
			if why := o.refuse(path); why != "" {
				return why
			}
		}
	}
	if b, ok := o.base.(RefusalSource); ok {
		return b.Refusal(path)
	}
	return ""
}

// NewOverlayLoader wraps base with overlay. `accept` decides whether an overlay
// file is well-formed enough to shadow the base; nil installs DefaultAccept.
func NewOverlayLoader(base, overlay Loader, accept func(path string, data []byte) bool) *OverlayLoader {
	if accept == nil {
		accept = DefaultAccept
	}
	return &OverlayLoader{base: base, overlay: overlay, accept: accept}
}

// DefaultAccept is the generic, format-agnostic guard: a non-empty file, and —
// for a `.md` contribution — one that opens with a `---` frontmatter delimiter.
// This rejects truncated/garbage overrides without parsing the body, so a broken
// override falls back to embed.
func DefaultAccept(path string, data []byte) bool {
	if len(bytes.TrimSpace(data)) == 0 {
		return false
	}
	if strings.HasSuffix(path, ".md") {
		return bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), []byte("---"))
	}
	return true
}

// overlayHit returns the overlay's bytes for path when present AND accepted.
func (o *OverlayLoader) overlayHit(path string) ([]byte, bool) {
	data, err := o.overlay.ReadFile(path)
	if err != nil || data == nil || !o.accept(path, data) {
		return nil, false
	}
	if o.refuse != nil && o.refuse(path) != "" {
		return nil, false
	}
	return data, true
}

// ReadFile returns the accepted overlay bytes, else the base bytes.
func (o *OverlayLoader) ReadFile(path string) ([]byte, error) {
	if data, ok := o.overlayHit(path); ok {
		return data, nil
	}
	return o.base.ReadFile(path)
}

// Exists is true when the base has the path, or the overlay has an ACCEPTED copy.
func (o *OverlayLoader) Exists(path string) bool {
	if _, ok := o.overlayHit(path); ok {
		return true
	}
	return o.base.Exists(path)
}

// List/Walk return the overlay-first union (deduplicated). A malformed overlay
// entry that only the overlay has still lists, but ReadFile falls back to base
// (empty for an overlay-only path) — the contributor then skips the unparseable file.
func (o *OverlayLoader) List(pattern string) []string {
	return mergeUnique(o.base.List(pattern), o.overlay.List(pattern))
}

func (o *OverlayLoader) Walk() []string {
	return mergeUnique(o.base.Walk(), o.overlay.Walk())
}

func (o *OverlayLoader) Reset(path string) {
	o.base.Reset(path)
	o.overlay.Reset(path)
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, group := range [][]string{a, b} {
		for _, p := range group {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}
