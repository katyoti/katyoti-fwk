// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/challenges/api"
	challengeslocal "github.com/katyoti/katyoti-fwk/internal/extensions/challenges/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE CHALLENGED BY TAB: every object with a `challenges` edge to this one —
// from any module, with its type and its state — and none of the objects
// linked to it otherwise.
func TestChallengersAreTheIncomingChallengesEdges(t *testing.T) {
	ctx, cwm := cwmapp.New(t, challengeslocal.Extension(), cwmapp.Pack(fstest.MapFS{
		"object-types/note.yaml":     {Data: []byte("name: note\nlabel: Note\n")},
		"edge-types/challenges.yaml": {Data: []byte("name: challenges\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n")},
		"edge-types/links.yaml":      {Data: []byte("name: links\nfrom: any\nto: any\ncardinality: M:N\ntarget: object\nobject_storage: edges\n")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	cwmapp.Module(t, ctx, cwm, "n")
	mk := func(module string, o cwmapi.Object) {
		t.Helper()
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	mk("m", cwmapi.Object{ID: "m.target", Type: "note", Name: "The design", Parent: "m"})
	mk("m", cwmapi.Object{ID: "m.doubt", Type: "note", Name: "Is it right?", Parent: "m", Edges: []cwmapi.ObjectEdge{{Kind: "challenges", To: "m.target"}}})
	mk("n", cwmapi.Object{ID: "n.risk", Type: "note", Name: "A risk", Parent: "n", Edges: []cwmapi.ObjectEdge{{Kind: "challenges", To: "m.target"}}})
	mk("m", cwmapi.Object{ID: "m.ref", Type: "note", Name: "Just a reference", Parent: "m", Edges: []cwmapi.ObjectEdge{{Kind: "links", To: "m.target"}}})
	svc, err := application.UseService[api.ChallengesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Challengers(ctx, api.ObjectInput{ID: "m.target"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("the page is off the grammar: %v", err)
	}
	raw, _ := json.Marshal(d)
	page := string(raw)
	for _, want := range []string{`"navigate":{"object":"m.doubt"}`, `"navigate":{"object":"n.risk"}`, `"badge":2`, `"text":"Note"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Contains(page, "m.ref") {
		t.Error("an object linked otherwise is listed as a challenger")
	}
	d, _ = svc.Challengers(ctx, api.ObjectInput{ID: "m.doubt", Language: "fr"})
	raw, _ = json.Marshal(d)
	if !strings.Contains(string(raw), "Rien ne le conteste.") {
		t.Errorf("nothing challenges m.doubt, in French: %s", raw)
	}
}
