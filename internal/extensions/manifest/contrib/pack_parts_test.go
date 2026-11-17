// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
)

// partsProvidersPoint registers the point the manifest extension owns, so the
// test runs the real phase sequence without the manifest service.
func partsProvidersPoint(init *application.ApplicationInit) {
	init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
		return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
	})
}

// partsProviders are the providers built over a provider's pack parts.
func partsProviders(app *application.Application) []*EmbedFSResourceProvider {
	var out []*EmbedFSResourceProvider
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if t, ok := p.(*EmbedFSResourceProvider); ok {
			if _, ours := t.Loader.(*partsLoader); ours {
				out = append(out, t)
			}
		}
	}
	return out
}

// TestPackPartsOfOneProviderMergeIntoOneProviderPerKind: two parts of one
// provider publish one provider per kind holding both parts' entries; the
// same entry in both parts is refused naming both owners; a folder no kind
// reads is refused naming its owner; the universe override tree wins over
// both parts.
func TestPackPartsOfOneProviderMergeIntoOneProviderPerKind(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := kinds.Get("edge-type")
	if !ok || d.SourceFolder() == "" {
		t.Fatal("the system set declares no edge-type folder")
	}
	folder := d.SourceFolder()
	edge := func(name, desc string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("name: " + name + "\nfrom: any\nto: any\ndescription: " + desc + "\n")}
	}

	// A temporary universe allowing overwrites, overriding `a`.
	home := t.TempDir()
	cenv.ResetUniverseManifestCache()
	t.Cleanup(cenv.ResetUniverseManifestCache)
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "Univers.yaml"), []byte("name: tv\nallow_overwrites: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ov := filepath.Join(cenv.OverwritesDir(home, "ktw-example"), "embed", "cwm", folder)
	if err := os.MkdirAll(ov, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ov, "a.yaml"), []byte("name: a\nfrom: any\nto: any\ndescription: overlay\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := overlayHome
	overlayHome = func() string { return home }
	t.Cleanup(func() { overlayHome = keep })

	run := func(host, ext fstest.MapFS) (*application.Application, error) {
		app := application.New(application.Config(), partsProvidersPoint,
			ContributePackPart("ktw-example", "host", "cwm", host),
			ContributePackPart("ktw-example", "goals", "cwm", ext),
		)
		return app, app.Run()
	}

	t.Run("merged", func(t *testing.T) {
		app, err := run(
			fstest.MapFS{folder + "/a.yaml": edge("a", "host")},
			fstest.MapFS{folder + "/b.yaml": edge("b", "ext")},
		)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		perKind := map[string]int{}
		var edges *EmbedFSResourceProvider
		for _, p := range partsProviders(app) {
			if p.ProviderName != "ktw-example" {
				t.Errorf("provider %q, want ktw-example", p.ProviderName)
			}
			perKind[p.ResourceType]++
			if p.ResourceType == d.Name {
				edges = p
			}
		}
		for k, n := range perKind {
			if n != 1 {
				t.Errorf("kind %s has %d providers, want one", k, n)
			}
		}
		if edges == nil {
			t.Fatalf("no %s provider over the parts", d.Name)
		}
		got, err := edges.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		descs := map[string]string{}
		for _, c := range got {
			descs[c.ID] = string(c.Data())
		}
		if !strings.Contains(descs["a"], "overlay") {
			t.Errorf("a = %s, want the override tree's", descs["a"])
		}
		if !strings.Contains(descs["b"], "ext") {
			t.Errorf("b = %q, want the extension part's", descs["b"])
		}
	})

	t.Run("clash", func(t *testing.T) {
		_, err := run(
			fstest.MapFS{folder + "/a.yaml": edge("a", "host")},
			fstest.MapFS{folder + "/a.yaml": edge("a", "ext")},
		)
		if err == nil {
			t.Fatal("Run succeeded; want the same entry in two parts refused")
		}
		for _, want := range []string{d.Name + "/a", "host", "goals"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %q", err, want)
			}
		}
	})

	t.Run("unclaimed", func(t *testing.T) {
		_, err := run(
			fstest.MapFS{folder + "/a.yaml": edge("a", "host")},
			fstest.MapFS{"no-such-kind-folder/x.yaml": edge("x", "ext")},
		)
		if err == nil {
			t.Fatal("Run succeeded; want a folder no kind reads refused")
		}
		if !strings.Contains(err.Error(), "no-such-kind-folder/ in goals") {
			t.Errorf("error %q does not name the folder and its owner", err)
		}
	})
}
