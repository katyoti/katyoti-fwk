// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"log"
	"os"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// contentLoader returns the loader every ContributeXxxResources reads from: the
// binary's embedded FS, transparently overlaid with a workspace override tree
// (`.katyoti/overwrites/<provider>/`) WHEN the universe opts in
// (`allow_overwrites: true` in Univers.yaml). Override files mirror the embed
// layout (`…/embed/<type>/<name>`) so their keys align with the base loader and
// `promote` is a plain copy back into the sidecar's source tree.
//
// Resolution is LAZY: the extension list is built before bootstrap, so Env()
// and the universe manifest aren't ready when this is called. By the time a
// contribution is first enumerated they are, so the wrapper resolves on first
// file access (once) and caches the decision for the process lifetime.
func contentLoader(base loader.Loader, provider string) loader.Loader {
	return &deferredOverlay{base: base, provider: provider}
}

// deferredOverlay defers the allow_overwrites gate + override-dir lookup to
// first use, then delegates every Loader call to the resolved loader (either the
// bare embed base, or an OverlayLoader when overrides are active).
type deferredOverlay struct {
	base     loader.Loader
	provider string
	once     sync.Once
	active   loader.Loader
}

func (d *deferredOverlay) resolve() loader.Loader {
	d.once.Do(func() {
		env := application.Env()
		d.active = buildOverlay(d.base, env.UniverseDir(), d.provider)
	})
	return d.active
}

// buildOverlay layers a provider's embedded content, per file and last wins:
// the binary's embed, then the universe's override tree
// (`<home>/overwrites/<provider>/`, only when Univers.yaml allows overwrites:
// the operator's last word). A project overrides a provider's content through
// override objects the catalog applies, never through a tree read here. The
// layer is named, so a reader can say where a file came from
// (loader.OriginSource). Pure (root passed in) so it is unit-testable without
// the global Env.
func buildOverlay(base loader.Loader, home, provider string) loader.Loader {
	if !cenv.AllowOverwrites(home) {
		return base
	}
	dir := cenv.OverwritesDir(home, provider)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return base // gate on, but this sidecar has no override tree
	}
	log.Printf("[overlay] %s: allow_overwrites=true — workspace overrides ACTIVE from %s (embedded resources may be shadowed)", provider, dir)
	return loader.NamedOverlay(base, loader.NewFSLoader(dir), contribapi.OriginUniverse, nil)
}

// Immutable implements loader.ImmutableSource. Mutability is a claim about
// the SOURCE, not about this wrapper: resolved lazily like every read, the
// answer is immutable exactly when no overlay activated (the resolved loader
// IS the base) and the base itself is immutable. An active workspace override
// tree is a hosted folder, so it is mutable.
func (d *deferredOverlay) Immutable() bool {
	active := d.resolve()
	return active == d.base && loader.IsImmutable(d.base)
}

// Compile-time check: the wrapper answers for its source.
var _ loader.ImmutableSource = (*deferredOverlay)(nil)

func (d *deferredOverlay) ReadFile(p string) ([]byte, error) { return d.resolve().ReadFile(p) }
func (d *deferredOverlay) List(pattern string) []string      { return d.resolve().List(pattern) }
func (d *deferredOverlay) Walk() []string                    { return d.resolve().Walk() }
func (d *deferredOverlay) Exists(p string) bool              { return d.resolve().Exists(p) }
func (d *deferredOverlay) Reset(p string)                    { d.resolve().Reset(p) }

// Origin and Refusal forward the resolved loader's answers (an overlay names
// its layers; a bare base answers "").
func (d *deferredOverlay) Origin(p string) string {
	if o, ok := d.resolve().(loader.OriginSource); ok {
		return o.Origin(p)
	}
	return ""
}

func (d *deferredOverlay) Refusal(p string) string {
	if r, ok := d.resolve().(loader.RefusalSource); ok {
		return r.Refusal(p)
	}
	return ""
}
