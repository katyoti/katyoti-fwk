// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a new project's object can be seeded - the hub leaves the seed
// in the project's store, CWM mints the object from it at the first opening
// (its type, name and groups, at the code's data version) and removes it; no
// seed, the default object as before; a seed CWM refuses is kept and the
// project opens with the default object.
func TestAProjectObjectIsMintedFromItsSeed(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	open := func(seed string) (api.Object, string) {
		t.Helper()
		c := *cfg
		c.WorkspaceRoot = freshWorkspace(t)
		c.ProjectRoot = t.TempDir()
		path := filepath.Join(c.WorkspaceRoot, api.ProjectSeedFile)
		if seed != "" {
			if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		svc, err := New(stdctx.Background(), &c)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = svc.Close() })
		if _, err := svc.EnsureProject(stdctx.Background()); err != nil {
			t.Fatalf("EnsureProject: %v", err)
		}
		o, err := ReadObjectFile(filepath.Join(c.ProjectRoot, api.ProjectFileName))
		if err != nil {
			t.Fatal(err)
		}
		return o, path
	}

	o, seed := open("name: demos\ntitle: Les démos\ntype: dossier-architecture\n")
	if o.ID != "demos" || o.Name != "Les démos" || o.Type != "dossier-architecture" {
		t.Errorf("seeded: %s %q %s; want demos « Les démos », a dossier-architecture", o.ID, o.Name, o.Type)
	}
	if v, _ := o.Capabilities[api.CapProject]["data_version"].(int); v != dataVersionOf(migrationSteps) {
		t.Errorf("seeded at data version %v; want the code's, %d", o.Capabilities[api.CapProject]["data_version"], dataVersionOf(migrationSteps))
	}
	if _, err := os.Stat(seed); !os.IsNotExist(err) {
		t.Errorf("the seed is still there after the object was minted (%v)", err)
	}

	if o, _ := open(""); o.Type != api.DefaultProjectType {
		t.Errorf("no seed: %s; want the default project type", o.Type)
	}

	o, seed = open("name: wrong\ntype: feature\n")
	if o.Type != api.DefaultProjectType {
		t.Errorf("a seed of no project type: %s; want the default object", o.Type)
	}
	if _, err := os.Stat(seed); err != nil {
		t.Errorf("a refused seed was removed (%v); want it kept", err)
	}
}
