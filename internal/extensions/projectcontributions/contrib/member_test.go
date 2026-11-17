// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// entryOf is the entry an object publishing itself as kind/id with body
// carries.
func entryOf(t *testing.T, object, kind, id, body string) api.ObjectEntry {
	t.Helper()
	e, err := api.EntryOfObject(context.Background(), cwmapi.Object{ID: object, Body: body,
		Capabilities: map[string]map[string]any{api.CapContributes: {"kind": kind, "id": id, "layers": map[string]any{"source": "body"}}}},
		"acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// push is p sent over the wire and read back, as the hub receives it.
func wire(t *testing.T, p api.EntriesPush) api.EntriesPush {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var out api.EntriesPush
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func memberServes(t *testing.T, m *Member) map[string]string {
	t.Helper()
	ps, err := m.Providers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range ps {
		items, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range items {
			rc, _, err := p.Open(context.Background(), c.ID, "source")
			if err != nil {
				t.Fatalf("open %s: %v", c.ID, err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			out[p.Type()+"/"+c.ID] = string(b)
		}
	}
	return out
}

// The project's member serves its folder files and the object entries pushed
// to it: a put serves the entry (a kind change moves it), a remove withdraws
// it, a snapshot replaces the whole set; a file and an object with the same
// kind and id are both withheld - neither enumerates nor opens - with a
// finding naming both.
func TestTheProjectMemberServesPushedObjectsBesideItsFolder(t *testing.T) {
	ctx := context.Background()
	folder := NewObjectProvider("acme", "deck-theme") // a folder provider of the kind
	if err := folder.Put(entryOf(t, "file", "deck-theme", "red", "# Red file")); err != nil {
		t.Fatal(err)
	}
	m := NewMember("acme", []contribapi.ContributionProvider{folder})

	blue := entryOf(t, "themes.blue", "deck-theme", "blue", "# Blue")
	if err := m.Apply(wire(t, api.EntriesPush{Op: api.PushPut, Token: "acme", Object: "themes.blue", Entry: &blue})); err != nil {
		t.Fatal(err)
	}
	if got := memberServes(t, m); got["deck-theme/blue"] != "# Blue" || got["deck-theme/red"] != "# Red file" {
		t.Fatalf("served = %v; want the file and the pushed object", got)
	}
	// The object now publishes itself as a persona: the deck theme goes.
	asPersona := entryOf(t, "themes.blue", "persona", "blue", "# Blue persona")
	if err := m.Apply(api.EntriesPush{Op: api.PushPut, Token: "acme", Object: "themes.blue", Entry: &asPersona}); err != nil {
		t.Fatal(err)
	}
	if got := memberServes(t, m); got["deck-theme/blue"] != "" || got["persona/blue"] != "# Blue persona" {
		t.Fatalf("served after a kind change = %v", got)
	}
	if err := m.Apply(api.EntriesPush{Op: api.PushRemove, Token: "acme", Object: "themes.blue"}); err != nil {
		t.Fatal(err)
	}
	if got := memberServes(t, m); len(got) != 1 {
		t.Fatalf("served after the remove = %v; want the file alone", got)
	}

	// A clash: an object publishes the folder's deck-theme/red.
	red := entryOf(t, "themes.red", "deck-theme", "red", "# Red object")
	green := entryOf(t, "themes.green", "deck-theme", "green", "# Green")
	if err := m.Apply(wire(t, api.EntriesPush{Op: api.PushSnapshot, Token: "acme", Entries: []api.ObjectEntry{red, green}})); err != nil {
		t.Fatal(err)
	}
	got := memberServes(t, m)
	if _, ok := got["deck-theme/red"]; ok || got["deck-theme/green"] != "# Green" || len(got) != 1 {
		t.Fatalf("served with a clash = %v; want deck-theme/green alone (red withheld on both sides)", got)
	}
	ps, _ := m.Providers(ctx)
	for _, p := range ps {
		if _, _, err := p.Open(ctx, "red", "source"); err == nil {
			t.Fatalf("a withheld entry opens through %s", p.Type())
		}
	}
	findings, err := m.Findings(ctx)
	if err != nil || len(findings) != 1 || !strings.Contains(findings[0], "file acme:/deck-theme/red") || !strings.Contains(findings[0], "object themes.red") {
		t.Fatalf("findings = %v, %v; want one naming the file and the object", findings, err)
	}

	// A snapshot replaces the whole set.
	if err := m.Apply(api.EntriesPush{Op: api.PushSnapshot, Token: "acme"}); err != nil {
		t.Fatal(err)
	}
	if got := memberServes(t, m); len(got) != 1 || got["deck-theme/red"] != "# Red file" {
		t.Fatalf("served after an empty snapshot = %v; want the file alone", got)
	}
	// Another token's push is refused.
	if err := m.Apply(api.EntriesPush{Op: api.PushRemove, Token: "other", Object: "x"}); err == nil {
		t.Fatal("a push for another project is applied")
	}
}

// The push subjects are built per project token, and refuse a token that is
// not one.
func TestPushSubjects(t *testing.T) {
	if got := api.PushSubject("local", "dev", "acme"); got != "ktw.local.universe.dev.project-contributions.acme.push" {
		t.Fatalf("push subject = %s", got)
	}
	if got := api.SnapshotRequestSubjectAll("local", "dev"); got != "ktw.local.universe.dev.project-contributions.*.snapshot" {
		t.Fatalf("snapshot request subject = %s", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a token with a dot builds a subject")
		}
	}()
	api.PushSubject("local", "dev", "a.b")
}
