// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// TestAFrozenFolderPublishesWhatTheFolderMemberDoes: a files-only folder
// published by the generic host (ktw-host over the folder — what a frozen
// files-only sidecar runs) and the same folder read by the hub as a folder
// member (manifestlocal.FolderManifest) publish the same entries: the same
// URIs, data and layers. The host's own offerings (its services, its
// identity, its config surface, what it publishes over an empty folder) are
// the host's, not the folder's.
func TestAFrozenFolderPublishesWhatTheFolderMemberDoes(t *testing.T) {
	root := repoRoot(t)
	host := binaryFor(t, "ktw-host")
	if host == "" {
		t.Skip("bin/ktw-host not built")
	}
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	hostOwn := map[string]bool{"service": true, "sidecar-info": true, "config-file": true, "config-program": true, "env": true}
	// What the host publishes over an empty folder is the host's own (the
	// manifest extension's data-types every binary carries), not a folder's.
	bare, err := appcore.RunResourcesList(host, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	ownURI := map[string]bool{}
	for _, c := range bare.Contributions {
		ownURI[c.Type+"/"+c.ID] = true
	}
	for _, fixture := range []string{"files-only-pack", "foreign-kind-pack"} {
		folder := filepath.Join(root, "internal", "test", "fixtures", "contributions", fixture)
		raw, err := os.ReadFile(filepath.Join(folder, "ktw-host.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var decl struct {
			Name string `yaml:"name"`
		}
		if err := config.DecodeBytes(raw, marshal.YAML, &decl); err != nil || decl.Name == "" {
			t.Fatalf("%s: ktw-host.yaml names no member: %v", fixture, err)
		}

		served, err := appcore.RunResourcesList(host, folder, false)
		if err != nil {
			t.Fatalf("%s: ktw-host over the folder: %v", fixture, err)
		}
		read, err := manifestlocal.FolderManifest(context.Background(), decl.Name, folder, kinds)
		if err != nil {
			t.Fatalf("%s: the folder member: %v", fixture, err)
		}
		want := entries(served, decl.Name, hostOwn)
		for uri := range ownURI {
			delete(want, uri)
		}
		got := entries(read, decl.Name, hostOwn)
		if len(want) == 0 {
			t.Fatalf("%s: the host published none of the folder's entries", fixture)
		}
		if strings.Join(keysOf(want), "\n") != strings.Join(keysOf(got), "\n") {
			t.Errorf("%s: the host publishes\n  %s\nthe folder member\n  %s", fixture,
				strings.Join(keysOf(want), "\n  "), strings.Join(keysOf(got), "\n  "))
			continue
		}
		for uri, w := range want {
			if g := got[uri]; g != w {
				t.Errorf("%s %s: host %s, folder member %s", fixture, uri, w, g)
			}
		}
	}
}

// entries are a manifest's entries of the folder, by URI: its data and layer
// hashes, the host's own offerings left out.
func entries(m *mbapi.Manifest, provider string, hostOwn map[string]bool) map[string]string {
	out := map[string]string{}
	for _, c := range m.Contributions {
		if hostOwn[c.Type] || (c.Provider != "" && c.Provider != provider) {
			continue
		}
		var layers []string
		for name, ref := range c.Layers {
			if name == contribapi.LayerData {
				continue
			}
			layers = append(layers, name+"="+ref.Hash)
		}
		sort.Strings(layers)
		out[c.Type+"/"+c.ID] = "data=" + c.DataHash() + " " + strings.Join(layers, ",")
	}
	return out
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
