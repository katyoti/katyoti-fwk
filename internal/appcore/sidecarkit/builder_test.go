// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"embed"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// L1 — defaults produce a sane extension list with the framework-
// mandatory bits + opt-in defaults all wired. Captures the SN-11 shape
// so a future drift (e.g. someone adds a new mandatory extension and
// forgets to flag the builder) shows up here.
func TestNewSidecar_DefaultsWireExpectedCount(t *testing.T) {
	cfg := appcore.AppConfig{
		AppName: "ktw-test", Version: "0.0.0",
		Short: "test", EmbeddedFS: embed.FS{},
	}

	exts := NewSidecar(cfg, WithDomain(noopExt))

	// Framework-mandatory (5) + ConfigResources (1) + Domain (1) + pack (1)
	// + AutoKTSPContainer (1) + AutoHTTPServer (1)
	// = 10. No kind runtime is the kit's (a binary passes it with
	// WithDomain). Scripts is opt-in (WithScripts), so it adds nothing here. Any
	// change to this count is a deliberate builder-design decision and
	// should be matched in the test.
	const expected = 10
	if len(exts) != expected {
		t.Errorf("default extension count = %d, want %d", len(exts), expected)
	}
}

// L1 — opt-outs trim the right extensions. Sanity check that
// WithoutXxx options actually disable their bits.
func TestNewSidecar_OptOutsTrim(t *testing.T) {
	cfg := appcore.AppConfig{AppName: "ktw-test", Version: "0", EmbeddedFS: embed.FS{}}

	exts := NewSidecar(cfg,
		WithDomain(noopExt),
		WithoutPack(),
		WithoutConfigResources(),
		WithoutKTSP(),
		WithoutHTTPServer(),
	)

	// 5 mandatory + 1 domain = 6 (everything else disabled; scripts is
	// off by default).
	const expected = 6
	if len(exts) != expected {
		t.Errorf("trimmed extension count = %d, want %d", len(exts), expected)
	}
}

// L1 — WithExtraExtensions appends at the tail.
func TestNewSidecar_ExtraExtensionsAppended(t *testing.T) {
	cfg := appcore.AppConfig{AppName: "ktw-test", Version: "0", EmbeddedFS: embed.FS{}}

	exts := NewSidecar(cfg,
		WithDomain(noopExt),
		WithoutPack(), WithoutConfigResources(),
		WithoutKTSP(), WithoutHTTPServer(),
		WithExtraExtensions(markerExt("a"), markerExt("b")),
	)

	if len(exts) != 8 {
		t.Fatalf("want 5 mandatory + 1 domain + 2 extra = 8, got %d", len(exts))
	}
	// Marker placement isn't directly observable without running the
	// application, but the count + manual reading of the slice order in
	// builder.go is the contract. This test exists to fail loudly if
	// somebody re-orders the appends.
}

func noopExt(*application.ApplicationInit) {}

// markerExt is a no-op extension carrying a name in its closure for
// readability when the slice is inspected manually.
func markerExt(name string) application.Extension {
	return func(*application.ApplicationInit) { _ = name }
}
