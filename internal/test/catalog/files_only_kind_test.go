// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// filesOnlyUniverse is the kebab universe token for the files-only-kind test.
const filesOnlyUniverse = "files-only-kind"

// The fixture's three URIs: the kind's declaration, its schema, one entry.
const (
	fixtureKindURI   = "files-only-pack:/contribution-type/recipe"
	fixtureSchemaURI = "files-only-pack:/data-type/recipe"
	fixtureEntryURI  = "files-only-pack:/recipe/pancakes"
)

// packMember is one stdio member of a test universe: a binary, optionally
// serving a folder (declared as its content root and passed as --folder).
type packMember struct {
	Name, Binary, Folder string
}

// makePackWorkspace declares the members of a test universe, in order (the
// hub's own record carries the system set; members are probed without it, e26).
func makePackWorkspace(t *testing.T, universe string, members []packMember) string {
	t.Helper()
	ws := t.TempDir()
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, m := range members {
		f := filepath.ToSlash(m.Folder)
		fmt.Fprintf(&b, "  - name: %s\n    transport: stdio\n    binary: %s\n", m.Name, filepath.ToSlash(m.Binary))
		if f != "" {
			fmt.Fprintf(&b, "    content_root: %s\n    args: [\"--folder\", \"%s\", \"sc\", \"ktsp\", \"stdio\"]\n", f, f)
		}
	}
	manifest := fmt.Sprintf("version: 1\nname: %s\ndisplay_name: %s\nicon: T\ncolor: gray\nsidecars:\n%s",
		universe, universe, b.String())
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# "+universe+" test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// fixtureCopy copies a fixture folder into a temp dir, so nothing a member does
// with its content root can touch the repository.
func fixtureCopy(t *testing.T, root string, rel ...string) string {
	t.Helper()
	src := filepath.Join(append([]string{root, "internal", "test", "fixtures", "contributions"}, rel...)...)
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatalf("copy fixture %s: %v", src, err)
	}
	return dst
}

// startPackUniverse boots a hub over the members and returns a sender and a
// universe-reach catalog over it.
func startPackUniverse(t *testing.T, universe string, members []packMember) (*messagebus.Sender, *manifestremote.Catalog) {
	t.Helper()
	ws := makePackWorkspace(t, universe, members)
	t.Setenv("KTW_WORKSPACE", ws)
	dataDir := filepath.Join(ws, ".katyoti")
	stop := hubharness.Start(t, dataDir)
	t.Cleanup(stop)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	t.Cleanup(func() { hc.Close() })
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, universe)
	return sender, manifestremote.New(sender)
}

// waitListed polls the universe index until every URI is listed, and returns
// the listed entries by URI.
func waitListed(t *testing.T, sender *messagebus.Sender, types []string, uris []string, within time.Duration) map[string]contribapi.Contribution {
	t.Helper()
	deadline := time.Now().Add(within)
	var lastErr error
	got := map[string]contribapi.Contribution{}
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/ListContributions",
			mbapi.CatalogListContributionsInput{Filter: contribapi.ListFilter{Types: types}})
		cancel()
		if err == nil {
			var out mbapi.CatalogListContributionsOutput
			if err = json.Unmarshal(raw, &out); err == nil {
				got = map[string]contribapi.Contribution{}
				for _, c := range out.Contributions {
					got[c.ResolvedURI()] = c
				}
				missing := 0
				for _, u := range uris {
					if _, ok := got[u]; !ok {
						missing++
					}
				}
				if missing == 0 {
					return got
				}
			}
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	var seen []string
	for u := range got {
		seen = append(seen, u)
	}
	t.Fatalf("universe never listed %v within %s (last err %v); listed: %v", uris, within, lastErr, seen)
	return nil
}

// TestFilesOnlyKindThroughTheUniverse is e19's files-only proof across
// processes: a pack that declares its own kind, the kind's schema and one entry,
// served by the generic host with no code for the kind, is listed by the
// universe, opened by layer through the one consumer lane, and validated by the
// hub against the pack's own schema. The no-framework-diff half is
// internal/core/contribution.TestPackKindNeedsNoFrameworkCode.
func TestFilesOnlyKindThroughTheUniverse(t *testing.T) {
	root := repoRoot(t)
	hostBin := binaryFor(t, "ktw-host")
	if hostBin == "" {
		hubharness.MissingBinary(t, "files-only kind: bin/ktw-host not built")
		return
	}
	folder := fixtureCopy(t, root, "files-only-pack")
	sender, cat := startPackUniverse(t, filesOnlyUniverse, []packMember{
		{Name: "files-only-pack", Binary: hostBin, Folder: folder},
	})

	listed := waitListed(t, sender,
		[]string{contribapi.ResourceTypeContributionType, contribapi.ResourceTypeDataType, "recipe"},
		[]string{fixtureKindURI, fixtureSchemaURI, fixtureEntryURI}, 45*time.Second)

	assertFixtureEntryReadable(t, cat, fixtureEntryURI, "Pancakes", "200 g flour")

	// The hub validated the entry against the PACK's schema — a verdict, not
	// "unchecked". A pass is an explicit *true; warn keeps failures as errors.
	entry := listed[fixtureEntryURI]
	if entry.Valid == nil || !*entry.Valid || len(entry.ValidationErrors) > 0 {
		t.Errorf("%s: verdict valid=%v errors=%v — want an explicit pass against %s",
			fixtureEntryURI, entry.Valid, entry.ValidationErrors, fixtureSchemaURI)
	}
}

// assertFixtureEntryReadable opens an entry's data and default (source) layers
// through mbapi.Open at universe reach — the one content lane every consumer
// uses — and checks each carries what the fixture wrote.
func assertFixtureEntryReadable(t *testing.T, cat *manifestremote.Catalog, uri, wantName, wantSource string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}

	l, err := mbapi.Open(ctx, q, uri, contribapi.LayerData)
	if err != nil {
		t.Fatalf("open %s data: %v", uri, err)
	}
	raw, err := l.Bytes()
	if err != nil {
		t.Fatalf("read %s data: %v", uri, err)
	}
	var data struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Name != wantName {
		t.Errorf("%s data layer = %s (err %v), want name %q", uri, raw, err, wantName)
	}

	// Empty layer = the KIND's default, read from the pack's declaration.
	l, err = mbapi.Open(ctx, q, uri, "")
	if err != nil {
		t.Fatalf("open %s default layer: %v", uri, err)
	}
	if l.Name != contribapi.LayerSource {
		t.Errorf("%s default layer = %q, want source (the pack's declaration says so)", uri, l.Name)
	}
	src, err := l.Bytes()
	if err != nil {
		t.Fatalf("read %s source: %v", uri, err)
	}
	if !strings.Contains(string(src), wantSource) {
		t.Errorf("%s source layer = %q, want it to contain %q", uri, src, wantSource)
	}
}
