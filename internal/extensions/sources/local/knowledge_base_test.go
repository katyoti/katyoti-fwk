// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// (mock-up v6 views 15-17, 26): a holder's knowledge base
// lists every item read from all its sources once, with the source it came
// from - a record when it brings fields, a document otherwise; « À relire »
// when its source changed it since it was read -, counted by kind and state,
// filtered by source, kind and state, the sources with stale items to update,
// an item opening its source-document in a drawer.
func TestAHoldersKnowledgeBaseListsEveryItemReadOnceWithItsSource(t *testing.T) {
	ctx, _, svc, _, _ := knowledgeWorld(t)
	if _, err := svc.KnowledgeBase(ctx, api.KnowledgeBaseInput{}); err == nil {
		t.Error("a knowledge base needs its holder")
	}

	empty, err := svc.KnowledgeBase(ctx, api.KnowledgeBaseInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if d := empty.Data.(map[string]any); d["empty"] != true || len(d["items"].([]any)) != 0 {
		t.Errorf("a holder whose sources read nothing has an empty base: %v", d)
	}
	if err := render.ValidatePage(empty.DSLTree); err != nil {
		t.Errorf("the empty base's page: %v", err)
	}

	wiki, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "wiki-page"})
	if err != nil {
		t.Fatal(err)
	}
	folder, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "local-folder"})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []api.IngestInput{
		{SourceDocID: "m.wiki-acme", SourceURI: "wiki://acme", NewContent: "x", Parent: wiki.Source, Name: "Acme", ItemKey: "acme", ItemKind: "page"},
		{SourceDocID: "m.wiki-app", SourceURI: "wiki://app", NewContent: "x", Parent: wiki.Source, Name: "Acme app", ItemKey: "app", ItemKind: "application", DataFile: "m/input/app.json"},
		{SourceDocID: "m.dir-note", SourceURI: "wiki://note", NewContent: "x", Parent: folder.Source, Name: "note.md", ItemKey: "note.md"},
	} {
		if _, err := svc.Ingest(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	// The wiki changed the page since it was read.
	if _, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.wiki-acme", NewContent: "y"}); err != nil {
		t.Fatal(err)
	}

	base := func(in api.KnowledgeBaseInput) map[string]any {
		t.Helper()
		in.ID = "m"
		out, err := svc.KnowledgeBase(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if out.Kind != "page" {
			t.Errorf("the knowledge base is a page, got %q", out.Kind)
		}
		if err := render.ValidatePage(out.DSLTree); err != nil {
			t.Errorf("the knowledge base's page: %v", err)
		}
		return out.Data.(map[string]any)
	}
	rows := func(d map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, it := range d["items"].([]any) {
			row := it.(map[string]any)
			out[row["id"].(string)] = row
		}
		return out
	}

	all := base(api.KnowledgeBaseInput{})
	got := rows(all)
	if len(got) != 3 || len(all["items"].([]any)) != 3 {
		t.Fatalf("every item read, once: %v", all["items"])
	}
	for id, want := range map[string][4]string{
		"m.wiki-acme": {wiki.Source, "Wiki page", "Document", "À relire"},
		"m.wiki-app":  {wiki.Source, "Wiki page", "Enregistrement", "Lu"},
		"m.dir-note":  {folder.Source, "Dossier local", "Document", "Lu"},
	} {
		r := got[id]
		if r["source"] != want[0] || r["kind_label"] != want[2] || r["state_label"] != want[3] || (r["source_name"] != "" && r["source_name"] == r["source_label"]) {
			t.Errorf("%s: %v, want source %s, %s, %s", id, r, want[0], want[2], want[3])
		}
		if want[1] != "Dossier local" && r["source_label"] != want[1] {
			t.Errorf("%s comes from its connector's label %q, got %v", id, want[1], r["source_label"])
		}
	}
	if got["m.wiki-app"]["sub"] != "Application" {
		t.Errorf("an item says what it is in its connector's words: %v", got["m.wiki-app"]["sub"])
	}
	kpis := map[string]string{}
	for _, k := range all["kpis"].([]any) {
		kpis[k.(map[string]any)["label"].(string)] = k.(map[string]any)["value"].(string)
	}
	if kpis["éléments"] != "3" || kpis["documents — du texte"] != "2" || kpis["enregistrements — des champs"] != "1" || kpis["à relire — changés depuis"] != "1" {
		t.Errorf("the counts: %v", kpis)
	}
	if u := all["update"].([]any); len(u) != 1 || u[0].(map[string]any)["source"] != wiki.Source {
		t.Errorf("the wiki source has an item to re-read: %v", u)
	}
	if all["showing"] != "Tous les éléments" || len(all["show_all"].([]any)) != 0 {
		t.Errorf("unfiltered: %v %v", all["showing"], all["show_all"])
	}

	// Each filter alone, then together.
	for name, c := range map[string]struct {
		in   api.KnowledgeBaseInput
		want []string
	}{
		"source": {api.KnowledgeBaseInput{Source: folder.Source}, []string{"m.dir-note"}},
		"kind":   {api.KnowledgeBaseInput{Kind: api.KBKindRecord}, []string{"m.wiki-app"}},
		"state":  {api.KnowledgeBaseInput{State: api.KBStateStale}, []string{"m.wiki-acme"}},
		"all":    {api.KnowledgeBaseInput{Source: wiki.Source, Kind: api.KBKindDocument, State: api.KBStateRead}, nil},
	} {
		d := base(c.in)
		r := rows(d)
		if len(r) != len(c.want) {
			t.Errorf("filter %s: %v, want %v", name, r, c.want)
		}
		for _, id := range c.want {
			if r[id] == nil {
				t.Errorf("filter %s: %s missing", name, id)
			}
		}
		if len(d["show_all"].([]any)) != 1 || !strings.Contains(d["showing"].(string), "sur 3") {
			t.Errorf("filter %s: how many are shown and « Tout montrer »: %v %v", name, d["showing"], d["show_all"])
		}
	}

	// A filter chip carries the whole query: the others kept, its own part set.
	stale := base(api.KnowledgeBaseInput{State: api.KBStateStale})
	for _, g := range stale["filters"].([]any) {
		if g.(map[string]any)["group"] != "Nature" {
			continue
		}
		for _, c := range g.(map[string]any)["chips"].([]any) {
			chip := c.(map[string]any)
			if chip["kind"] == api.KBKindRecord && chip["state"] != api.KBStateStale {
				t.Errorf("a kind chip keeps the state filter: %v", chip)
			}
		}
	}

	out, _ := svc.KnowledgeBase(ctx, api.KnowledgeBaseInput{ID: "m"})
	tree, _ := json.Marshal(out.DSLTree)
	for _, want := range []string{`"drawer":{"object":"${item.id}","panel":"source-document"}`, `"query":{"args":`, `"uri":"ktw-agents:sources/source-sync"`} {
		if !strings.Contains(string(tree), want) {
			t.Errorf("the page misses %s", want)
		}
	}
}
