// SPDX-License-Identifier: Apache-2.0

package core

import (
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/universeview/api"
)

// A tag is one list of element ids `<place>:<name>`.
var declared = map[string]api.UniverseTag{
	"archifactory": {
		Elements: []api.TagItem{
			{ID: "rail:chat"}, {ID: "rail:inbox"}, {ID: "rail:marketplace", Level: api.LevelExpert},
			{ID: "home:actions"}, {ID: "home:mes-dossiers"}, {ID: "home:objects", Level: api.LevelStandard},
			{ID: "quick:start-chat"}, {ID: "quick:drawio", Level: api.LevelStandard},
			{ID: "new:dossier"}, {ID: "new:etude-existant"},
			{ID: "tab:information"}, {ID: "tab:cadrage"}, {ID: "tab:links", Level: api.LevelExpert},
		},
		First: []string{"archifactory"},
	},
	"reviewer": {
		Elements: []api.TagItem{
			{ID: "home:reviews-overview", Level: api.LevelStandard}, {ID: "home:objects"},
			{ID: "tab:reviews", Level: api.LevelStandard},
		},
		First: []string{"review"},
	},
}

// places is the view's allowed ids grouped by place, in the order rail, home,
// quick, new, tab - each in the view's order.
func places(v api.View) [][]string {
	out := make([][]string, 5)
	for i, p := range []string{"rail", "home", "quick", "new", "tab"} {
		for _, id := range v.Allowed {
			if strings.HasPrefix(id, p+":") {
				out[i] = append(out[i], id)
			}
		}
	}
	return out
}

// The view of a tagged universe at each level: Essentiel sees only the
// elements at essentiel, Standard adds its own, Expert sees every element the
// tags allow - and nothing they do not.
func TestATaggedUniverseShowsWhatItsTagAllowsAtEachLevel(t *testing.T) {
	m := &cenv.UniverseManifest{Tags: []string{"archifactory"}}
	want := map[string][][]string{
		api.LevelEssentiel: {{"rail:chat", "rail:inbox"}, {"home:actions", "home:mes-dossiers"}, {"quick:start-chat"}, {"new:dossier", "new:etude-existant"}, {"tab:information", "tab:cadrage"}},
		api.LevelStandard:  {{"rail:chat", "rail:inbox"}, {"home:actions", "home:mes-dossiers", "home:objects"}, {"quick:start-chat", "quick:drawio"}, {"new:dossier", "new:etude-existant"}, {"tab:information", "tab:cadrage"}},
		api.LevelExpert:    {{"rail:chat", "rail:inbox", "rail:marketplace"}, {"home:actions", "home:mes-dossiers", "home:objects"}, {"quick:start-chat", "quick:drawio"}, {"new:dossier", "new:etude-existant"}, {"tab:information", "tab:cadrage", "tab:links"}},
	}
	for _, level := range api.Levels {
		v := Resolve(declared, m, level)
		if v.All || !reflect.DeepEqual(places(v), want[level]) {
			t.Errorf("%s: %v (all=%v); want %v", level, places(v), v.All, want[level])
		}
		if !reflect.DeepEqual(v.First, []string{"archifactory"}) {
			t.Errorf("%s: first = %v", level, v.First)
		}
	}
}

// An untagged universe shows everything, at every level.
func TestAnUntaggedUniverseShowsEverythingAtEveryLevel(t *testing.T) {
	for _, m := range []*cenv.UniverseManifest{nil, {}, {GUI: &cenv.UniverseGUI{First: []string{"x"}}}} {
		for _, level := range api.Levels {
			v := Resolve(declared, m, level)
			if !v.All || len(v.Allowed) != 0 {
				t.Errorf("untagged %+v at %s: %+v; want everything", m, level, v)
			}
		}
	}
}

// Two tags are a union: an element in both is listed once, at the lowest level
// either gives it. The universe adjusts a level (gui.levels, by element id) -
// its gui.hide, retired, is only named - and orders the priority
// tags itself (gui.first); a tag no member declares is said and adds nothing.
func TestTagsUniteAndTheUniverseAdjustsThem(t *testing.T) {
	m := &cenv.UniverseManifest{
		Tags: []string{"archifactory", "reviewer", "ghost"},
		GUI: &cenv.UniverseGUI{
			Levels: map[string]string{"quick:drawio": api.LevelEssentiel, "tab:cadrage": api.LevelExpert},
			Hide:   []string{"rail:inbox"},
			First:  []string{"etude-existant", "archifactory"},
		},
	}
	got := places(Resolve(declared, m, api.LevelEssentiel))
	if !reflect.DeepEqual(got[1], []string{"home:actions", "home:mes-dossiers", "home:objects"}) {
		t.Errorf("home = %v; want objects at essentiel from the reviewer tag, once", got[1])
	}
	if !reflect.DeepEqual(got[2], []string{"quick:start-chat", "quick:drawio"}) {
		t.Errorf("quick actions = %v; want drawio brought down to essentiel", got[2])
	}
	if !reflect.DeepEqual(got[4], []string{"tab:information"}) {
		t.Errorf("object tabs = %v; want cadrage raised to expert", got[4])
	}
	if !reflect.DeepEqual(got[0], []string{"rail:chat", "rail:inbox"}) {
		t.Errorf("rail = %v; want inbox shown: gui.hide is retired", got[0])
	}
	v := Resolve(declared, m, api.LevelEssentiel)
	if !reflect.DeepEqual(v.First, []string{"etude-existant", "archifactory"}) {
		t.Errorf("first = %v; want the universe's own", v.First)
	}
	if !reflect.DeepEqual(v.Unknown, []string{"ghost"}) {
		t.Errorf("unknown = %v; want ghost", v.Unknown)
	}
	if got := Resolve(declared, &cenv.UniverseManifest{Tags: []string{"archifactory", "reviewer"}}, api.LevelEssentiel).First; !reflect.DeepEqual(got, []string{"archifactory", "review"}) {
		t.Errorf("first without the universe's = %v; want the tags' in tag order", got)
	}
}

// gui.hide is retired: a tagged universe whose gui.hide
// names an id its tag lists shows it - its tags alone decide - the view
// naming what gui.hide holds that it no longer applies. An untagged universe
// shows everything, its gui.hide named the same.
func TestGuiHideIsRetired(t *testing.T) {
	m := &cenv.UniverseManifest{Tags: []string{"archifactory"}, GUI: &cenv.UniverseGUI{Hide: []string{"home:objects", "rail:debug"}}}
	v := Resolve(declared, m, api.LevelExpert)
	if !contains(v.Allowed, "home:objects") {
		t.Errorf("home:objects, which the tag lists, is still left out by gui.hide: %v", v.Allowed)
	}
	if contains(v.Allowed, "rail:debug") {
		t.Errorf("rail:debug, which no tag lists, is shown: %v", v.Allowed)
	}
	if !reflect.DeepEqual(v.RetiredHide, []string{"home:objects", "rail:debug"}) {
		t.Errorf("retired_hide = %v; want what gui.hide names", v.RetiredHide)
	}
	bare := Resolve(declared, &cenv.UniverseManifest{GUI: &cenv.UniverseGUI{Hide: []string{"rail:debug"}, First: []string{"x"}}}, api.LevelStandard)
	if !bare.All || !reflect.DeepEqual(bare.RetiredHide, []string{"rail:debug"}) || !reflect.DeepEqual(bare.First, []string{"x"}) {
		t.Errorf("an untagged universe with gui.hide: %+v; want everything, its gui.hide named, its first kept", bare)
	}
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
