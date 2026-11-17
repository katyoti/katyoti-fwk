// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// published is every contribution of the application, by kind/id, with its
// data layer.
func published(t *testing.T, app *application.Application) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("Enumerate %s/%s: %v", p.Provider(), p.Type(), err)
		}
		for _, c := range got {
			key := p.Provider() + ":/" + c.Type + "/" + c.ID
			if _, dup := out[key]; dup {
				t.Errorf("%s published twice", key)
			}
			out[key] = c.Data()
		}
	}
	return out
}

// TestAPackPublishesCWMKinds: a pack whose CWM folders are read by the
// generic pack part entry publishes object types, edge types, capabilities and
// data-types — a contributor needs no cwm/contrib helper; two parts declaring
// one entry are refused naming both owners; and the CWM override tree is the
// generic one.
func TestAPackPublishesCWMKinds(t *testing.T) {
	pack := fstest.MapFS{
		"object-types/memo.yaml":             objectType("memo"),
		"edge-types/cites.yaml":              {Data: []byte("name: cites\nfrom: any\nto: any\n")},
		"capabilities/memo-extra.yaml":       {Data: []byte("name: memo-extra\nschema: :/data-type/cap-memo-extra\n")},
		"data-types/cap-memo-extra.dsl.yaml": {Data: []byte("$schema: ktw-schema/v1\nname: cap-memo-extra\nroot: X\ntypes:\n  X:\n    type: object\n")},
	}

	app, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "memos", "cwm", pack))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := published(t, app)
	for _, key := range []string{
		"ktw-host-x:/object-type/memo", "ktw-host-x:/edge-type/cites",
		"ktw-host-x:/capability/memo-extra", "ktw-host-x:/data-type/cap-memo-extra",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("the pack part does not publish %s (published %d entries)", key, len(got))
		}
	}

	// Two parts declaring one entry: refused, both owners named.
	_, err = runApp(t,
		manifestcontrib.ContributePackPart("ktw-host-x", "memos", "cwm", fstest.MapFS{"object-types/memo.yaml": objectType("memo")}),
		manifestcontrib.ContributePackPart("ktw-host-x", "notes", "cwm", fstest.MapFS{"object-types/memo.yaml": objectType("memo")}),
	)
	if err == nil {
		t.Fatal("two parts declaring object-type memo booted; want the clash refused")
	}
	for _, owner := range []string{"memos", "notes", "memo"} {
		if !strings.Contains(err.Error(), owner) {
			t.Errorf("clash error %q does not name %q", err, owner)
		}
	}

	// The CWM override tree is the generic pack's: the runtime and the reader
	// layer one tree.
	home := cwmHome(t, true, true)
	generic := manifestcontrib.PackOverlayFS(home, "ktw-foo", "cwm")
	ours := OverlayFS(home, "ktw-foo")
	if generic == nil || ours == nil {
		t.Fatalf("override tree: generic %v, cwm %v; want both", generic, ours)
	}
	a, errA := fs.ReadFile(generic, "node_types/x.yaml")
	b, errB := fs.ReadFile(ours, "node_types/x.yaml")
	if errA != nil || errB != nil || !bytes.Equal(a, b) {
		t.Fatalf("the trees differ: %q (%v) vs %q (%v)", a, errA, b, errB)
	}
}
