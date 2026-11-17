// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"reflect"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// emptyCatalog is a universe that publishes no slide group.
type emptyCatalog struct{}

func (emptyCatalog) List(context.Context) ([]*mbapi.Manifest, error) { return nil, nil }
func (emptyCatalog) GetContributionsByType(context.Context, string) ([]contribapi.Contribution, error) {
	return nil, nil
}
func (emptyCatalog) GetBody(context.Context, string) ([]byte, error) { return nil, nil }

const withInclude = "---\ntitle: T\n---\n\n# Cover\n\n---\n\n<!-- slide: include=our-team -->\n\n---\n\n## Close\n- bye\n\n---\n\n<!-- slide: include=our-team -->\n"

// A deck names the groups it includes; the same group twice is one name.
func TestIncludesNamesTheGroupsOnce(t *testing.T) {
	if got := Includes(Parse(withInclude)); !reflect.DeepEqual(got, []string{"our-team"}) {
		t.Fatalf("Includes = %v", got)
	}
}

// An include slide is checked: a known group is fine, an unknown one is
// named with the known ones, one not in reach is said unchecked, and an
// include slide holding anything else is an error.
func TestLintChecksIncludes(t *testing.T) {
	rules := func(md string, g Grammar) []string {
		var out []string
		for _, f := range Lint(md, g) {
			if strings.HasPrefix(f.Rule, "group-") || strings.HasPrefix(f.Rule, "include-") {
				out = append(out, f.Rule)
			}
		}
		return out
	}
	if got := rules(withInclude, Grammar{Groups: []string{"our-team"}, GroupsComplete: true}); len(got) != 0 {
		t.Errorf("a known group: %v", got)
	}
	if got := rules(withInclude, Grammar{Groups: []string{"principles"}, GroupsComplete: true}); !reflect.DeepEqual(got, []string{"group-unknown", "group-unknown"}) {
		t.Errorf("an unknown group: %v", got)
	}
	if got := rules(withInclude, Grammar{}); !reflect.DeepEqual(got, []string{"group-unchecked", "group-unchecked"}) {
		t.Errorf("groups not in reach: %v", got)
	}
	crowded := "---\ntitle: T\n---\n\n<!-- slide: include=our-team layout=section -->\n## Also a title\n"
	if got := rules(crowded, Grammar{Groups: []string{"our-team"}, GroupsComplete: true}); !reflect.DeepEqual(got, []string{"include-not-alone"}) {
		t.Errorf("an include with content: %v", got)
	}
	// `include` is a declared slide setting: no slide-key-unknown.
	for _, f := range Lint(withInclude, Grammar{Groups: []string{"our-team"}, GroupsComplete: true}) {
		if f.Rule == "slide-key-unknown" {
			t.Errorf("include is not declared: %+v", f)
		}
	}
}

// A group written in place of its include parses to its slides: the include
// line is gone, the group's slides are where it was.
func TestInlineGroupsWritesTheGroupInPlace(t *testing.T) {
	team := "## Our team\n- Ann\n\n---\n\n## How we work\n"
	out := InlineGroups(withInclude, map[string]string{"our-team": team})
	if strings.Contains(out, "include=") {
		t.Fatalf("include left:\n%s", out)
	}
	var titles []string
	for _, s := range Parse(out).Slides {
		titles = append(titles, strings.SplitN(s.Slots[0].Markdown, "\n", 2)[0])
	}
	want := []string{"# Cover", "## Our team", "## How we work", "## Close", "## Our team", "## How we work"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("slides = %v", titles)
	}
	if InlineGroups(withInclude, nil) != withInclude {
		t.Fatal("no groups changed the deck")
	}
}

// residentObjects is a project CWM as it answers: a listed object carries
// no body (the index keeps metadata), ObjectGet reads it in full.
type residentObjects struct{ calls int }

func (r *residentObjects) ObjectList(context.Context, cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	return cwmapi.ObjectListOutput{Objects: []cwmapi.Object{{ID: "slidedeck.our-team", Type: ObjectTypeSlideGroup}}}, nil
}

func (r *residentObjects) ObjectFileRead(context.Context, cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error) {
	return cwmapi.ObjectFileReadOutput{}, nil
}

func (r *residentObjects) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	r.calls++
	o := cwmapi.Object{ID: in.ID, Type: ObjectTypeSlideGroup}
	o.Body = "## Our team\n- Ann\n"
	return cwmapi.ObjectGetOutput{Object: o}, nil
}

// A project group's slides are its body, which a listing does not carry: they
// are read from the object (the user saw « not found » for a group that was
// there). And a catalog without the kind still leaves the project's groups.
func TestGroupsForReadsAProjectGroupsBody(t *testing.T) {
	src := &residentObjects{}
	ctx := WithThemeObjects(context.Background(), src)
	got, err := GroupsFor(ctx, mbapi.Q{Catalog: emptyCatalog{}}, withInclude)
	if err != nil {
		t.Fatal(err)
	}
	if got["our-team"] != "## Our team\n- Ann\n" || src.calls != 1 {
		t.Fatalf("groups = %q (ObjectGet calls %d)", got, src.calls)
	}
}
