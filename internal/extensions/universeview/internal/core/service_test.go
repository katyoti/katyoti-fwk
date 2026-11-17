// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/universeview/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The service reads the tags a member's pack declares (`universe-tags/` in
// its pack, the universe-tag kind) through the universe's catalog, and the
// universe's own Univers.yaml: a universe carrying the tag sees, at
// Essentiel, what the tag allows there; a universe with no tag, everything.
func TestTheViewIsResolvedFromThePacksTagAndTheUniverse(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	pack := loader.NewMapLoader(map[string]string{
		"universe-tags/archifactory.yaml": "label: ArchiFactory\nelements: [{id: rail:chat}, {id: rail:marketplace, level: expert}, {id: home:mes-dossiers}]\nfirst: [archifactory]\n",
	})
	ctx, _ := cwmapp.New(t, manifestcontrib.ContributePack("ktw-test", pack, ".", kinds))

	home := t.TempDir()
	cfg := application.EnvOf(home).ConfigDir()
	write := func(body string) {
		t.Helper()
		if err := os.MkdirAll(cfg, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		cenv.ResetUniverseManifestCache()
	}
	t.Cleanup(cenv.ResetUniverseManifestCache)
	s := NewAt(application.EnvOf(home).HomeDir())

	write("name: archi\ntags: [archifactory]\n")
	v, err := s.ResolveView(ctx, api.ResolveViewInput{Level: api.LevelEssentiel})
	if err != nil {
		t.Fatal(err)
	}
	if v.All || !reflect.DeepEqual(v.Allowed, []string{"rail:chat", "home:mes-dossiers"}) || len(v.Unknown) != 0 {
		t.Fatalf("tagged, essentiel: %+v; want the pack's tag resolved", v)
	}

	write("name: archi\n")
	v, err = s.ResolveView(ctx, api.ResolveViewInput{Level: api.LevelExpert})
	if err != nil || !v.All {
		t.Fatalf("untagged: %+v, %v; want everything", v, err)
	}

	if _, err := s.ResolveView(ctx, api.ResolveViewInput{Level: "guru"}); err == nil {
		t.Fatal("an unknown level is accepted")
	}
}
