// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : « Retirer » takes a source off its carrier - the files it
// mirrored deleted, its documents archived so they leave the knowledge base,
// the source archived - the other source kept; the board's switch turned off
// removes the connector's source; a source that only feeds the carrier is
// unlinked, never archived.
func TestRetirerTakesASourceOffItsCarrier(t *testing.T) {
	ctx, cwm, svc, root, _ := knowledgeWorld(t)

	wiki, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "wiki-page"})
	if err != nil || !wiki.OK {
		t.Fatalf("use wiki-page: %v %+v", err, wiki)
	}
	folder, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "local-folder"})
	if err != nil || !folder.OK {
		t.Fatalf("use local-folder: %v %+v", err, folder)
	}
	synced, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: wiki.Source})
	if err != nil || !synced.OK {
		t.Fatalf("sync: %v %+v", err, synced)
	}
	mirrored := filepath.Join(root, filepath.FromSlash(synced.Folder))
	if _, err := os.Stat(filepath.Join(mirrored, "home.md")); err != nil {
		t.Fatalf("the sync mirrored nothing: %v", err)
	}

	out, err := svc.Remove(ctx, api.RemoveInput{ID: wiki.Source})
	if err != nil || !out.OK || out.Removed != 1 {
		t.Fatalf("« Retirer »: %v %+v", err, out)
	}
	if _, err := os.Stat(mirrored); !os.IsNotExist(err) {
		t.Errorf("what it mirrored is still there: %v", err)
	}
	docs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "m", Parent: wiki.Source, Type: api.SourceDocumentType})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs.Objects) != 0 {
		t.Errorf("its documents are still in the carrier: %+v", docs.Objects)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(of.Sources) != 1 || of.Sources[0].ID != folder.Source {
		t.Fatalf("the carrier's sources after « Retirer »: %+v", of.Sources)
	}

	// The board's switch turned off.
	off, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "local-folder", On: "false"})
	if err != nil || !off.OK {
		t.Fatalf("switch off: %v %+v", err, off)
	}
	if of, _ = svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"}); len(of.Sources) != 0 {
		t.Fatalf("the switch turned off left a source: %+v", of.Sources)
	}

	// A source of another object feeding m: unlinked, kept.
	cwmapp.Module(t, ctx, cwm, "n")
	feeder := cwmapi.NewContainerObject("n", "wiki", api.InformationSourceType, "n", "Wiki", "")
	feeder.SetFieldsJSON([]byte(`{"connector":"wiki-page","sync":"manual"}`))
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "n", Object: feeder})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: made.Object.ID, To: "m", Kind: api.FeedsEdge}); err != nil {
		t.Fatal(err)
	}
	if of, _ = svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"}); len(of.Sources) != 1 {
		t.Fatalf("the feeding source is not m's: %+v", of.Sources)
	}
	if out, err = svc.Remove(ctx, api.RemoveInput{ID: made.Object.ID, Carrier: "m"}); err != nil || !out.OK {
		t.Fatalf("unlink: %v %+v", err, out)
	}
	if of, _ = svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"}); len(of.Sources) != 0 {
		t.Errorf("still fed: %+v", of.Sources)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "n", ID: made.Object.ID}); err != nil {
		t.Errorf("a feeding source was removed, not unlinked: %v", err)
	}

	// The tab: the switch says whether it turns on or off, each source row
	// has « Retirer », asking first.
	raw, err := os.ReadFile("../contrib/embed/cwm/object-panels/sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"on": "${value}"`, `label: "Retirer"`, `uri: "ktw-agents:sources/remove", args: {id: "${item.id}", carrier: "${id}"}, confirm:`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the « Sources » tab lacks %s", want)
		}
	}
}
