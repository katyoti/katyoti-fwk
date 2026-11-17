// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestASidecarPublishesItsEmbeddedPack: a sidecar built by the kit publishes
// its embed/ pack with no option — every folder a kind claims (skills/,
// tasks/, references/) and its embed/cwm/ part (the CWM content: object
// types, edge types, data types), never config/; WithPackRemap switches a
// kind off, WithoutPack the whole pack.
func TestASidecarPublishesItsEmbeddedPack(t *testing.T) {
	census.Isolate(t)
	efs := fstest.MapFS{
		"embed/skills/greet.md":         {Data: []byte("---\nname: greet\ndescription: Say hello\n---\nSay hello.\n")},
		"embed/tasks/review.md":         {Data: []byte("---\nname: review\ntype: agent\ndescription: d\n---\nReview.\n")},
		"embed/references/notes.md":     {Data: []byte("# Notes\n")},
		"embed/config/settings.yaml":    {Data: []byte("x: 1\n")},
		"embed/cwm/object-types/a.yaml": {Data: []byte("name: a\n")},
	}
	published := func(opts ...sidecarkit.Option) string {
		app := application.New(application.Config(), sidecarkit.NewSidecar(appcore.AppConfig{
			AppName: "pack-fixture", Version: "0.0.1", Short: "pack fixture", EmbeddedFS: efs,
		}, append([]sidecarkit.Option{sidecarkit.WithoutKTSP(), sidecarkit.WithoutHTTPServer()}, opts...)...)...)
		if err := app.Run(); err != nil {
			t.Fatal(err)
		}
		m, err := manifestlocal.NewBuilder("pack-fixture", "0.0.1").Build(application.WithApplication(context.Background(), app))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, c := range m.Contributions {
			switch c.Type {
			case "skill", "task", "reference", "object-type", "config":
				if c.Provider == "pack-fixture" {
					got = append(got, c.Type+"/"+c.ID)
				}
			}
		}
		sort.Strings(got)
		return strings.Join(got, " ")
	}
	if got, want := published(), "object-type/a reference/notes.md skill/greet task/review"; got != want {
		t.Errorf("by default: published %q, want %q", got, want)
	}
	if got, want := published(sidecarkit.WithPackRemap(map[string]string{"task": ""})), "object-type/a reference/notes.md skill/greet"; got != want {
		t.Errorf("tasks off: published %q, want %q", got, want)
	}
	if got := published(sidecarkit.WithoutPack()); got != "" {
		t.Errorf("without the pack: published %q, want nothing", got)
	}
}
