// SPDX-License-Identifier: Apache-2.0

package sidecarkit

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// packPublished runs the kit's pack of a binary named ktw-test with the given
// options and counts each URI the catalog's providers enumerate.
func packPublished(t *testing.T, fsys fstest.MapFS, opts ...Option) map[string]int {
	t.Helper()
	cfg := appcore.AppConfig{AppName: "ktw-test", EmbeddedFS: fsys}
	o := defaultOptions(cfg)
	for _, opt := range opts {
		opt(&o)
	}
	exts := append([]application.Extension{testProvidersPoint, contribution.SystemContributionsExtension()}, packExtensions(cfg, o)...)
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), exts...)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			count[c.ResolvedURI()]++
		}
	}
	return count
}

// The kit's default pack reads the binary's embed/cwm as a part of its pack,
// with no option: its object types, capabilities and flat data-types are
// published under the binary's name, once, beside the pack's own data-types.
// WithoutPack publishes none of it.
func TestTheDefaultPackReadsTheCWMFolder(t *testing.T) {
	dataType := func(name string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("$schema: ktw-schema/v1\nname: " + name + "\nroot: X\ntypes:\n  X:\n    type: object\n")}
	}
	fsys := fstest.MapFS{
		"embed/cwm/object-types/memo.yaml":             {Data: []byte("name: memo\nfolder: objects\ncapabilities: [container]\n")},
		"embed/cwm/capabilities/memo-extra.yaml":       {Data: []byte("name: memo-extra\nschema: :/data-type/cap-memo-extra\n")},
		"embed/cwm/data-types/cap-memo-extra.dsl.yaml": dataType("cap-memo-extra"),
		"embed/data-types/book.dsl.yaml":               dataType("book"),
	}
	cwm := []string{
		"ktw-test:/object-type/memo",
		"ktw-test:/capability/memo-extra",
		"ktw-test:/data-type/cap-memo-extra",
	}

	got := packPublished(t, fsys)
	for _, uri := range append(cwm, "ktw-test:/data-type/book") {
		if got[uri] != 1 {
			t.Errorf("default pack: %s published %d times, want 1", uri, got[uri])
		}
	}

	got = packPublished(t, fsys, WithoutPack())
	for _, uri := range cwm {
		if got[uri] != 0 {
			t.Errorf("WithoutPack: %s published %d times, want 0", uri, got[uri])
		}
	}

	// A binary that ships no embed/cwm boots and publishes no CWM entry.
	got = packPublished(t, fstest.MapFS{"embed/data-types/book.dsl.yaml": dataType("book")})
	if got["ktw-test:/data-type/book"] != 1 {
		t.Errorf("no embed/cwm: ktw-test:/data-type/book published %d times, want 1", got["ktw-test:/data-type/book"])
	}
}
