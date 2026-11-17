// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// resolvedOverlay is a deferredOverlay whose lazy resolution already ran
// against `home` — the same buildOverlay the first read performs, without
// the global Env.
func resolvedOverlay(base loader.Loader, home string) *deferredOverlay {
	d := &deferredOverlay{base: base, provider: "ktw-compose"}
	d.once.Do(func() { d.active = buildOverlay(base, home, d.provider) })
	return d
}

// Mutability is a claim about the SOURCE, not about the wrapper: the
// wrapper is immutable exactly when no overlay activated and its base is
// immutable. Until e13 it did not answer at all, so every sidecarkit-wired
// embedded provider reported `mutable`.
func TestDeferredOverlay_ImmutableIsTheSourcesClaim(t *testing.T) {
	sealed := func() loader.Loader {
		return loader.NewEmbedFSLoader(fstest.MapFS{"embed/skills/greet.md": {Data: []byte("x")}}, ".").MarkImmutable()
	}

	if d := resolvedOverlay(sealed(), writeHome(t, false, "", "")); !loader.IsImmutable(d) {
		t.Error("no overlay + immutable base: want immutable")
	}
	if d := resolvedOverlay(embedBase(), writeHome(t, false, "", "")); loader.IsImmutable(d) {
		t.Error("no overlay + mutable base (map loader): want mutable")
	}
	if d := resolvedOverlay(sealed(), writeHome(t, true, "embed/skills/greet.md", "OVERRIDE")); loader.IsImmutable(d) {
		t.Error("active overlay (a hosted folder): want mutable even over an immutable base")
	}
}
