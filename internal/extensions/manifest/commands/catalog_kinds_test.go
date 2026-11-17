// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// fakeIndex is an aggregate holding one manifest: two kind declarations and
// the entries of one of them, with their picker fields in meta.
type fakeIndex struct{ entries []contribapi.Contribution }

func (f fakeIndex) List(context.Context) ([]*mbapi.Manifest, error) {
	return []*mbapi.Manifest{{Provider: "ktw-example", Contributions: f.entries}}, nil
}

func (f fakeIndex) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, c := range f.entries {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f fakeIndex) GetBody(context.Context, string) ([]byte, error) { return nil, nil }

func kindDecl(name, description string) contribapi.Contribution {
	body, _ := json.Marshal(contribapi.KindDeclaration{Name: name, Description: description})
	c := contribapi.Contribution{Provider: "ktw-example", Type: contribapi.ResourceTypeContributionType, ID: name}
	c.SetData(body)
	return c
}

func describedEntry(kind, id, name, description string) contribapi.Contribution {
	return contribapi.Contribution{Provider: "ktw-example", Type: kind, ID: id,
		Metadata: &contribapi.ContributionMeta{Name: name, Description: description}}
}

func runKinds(t *testing.T, idx fakeIndex, args ...string) []byte {
	t.Helper()
	cmd := newCatalogKindsCommand(func(context.Context) (mbapi.CatalogReader, error) { return idx, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("catalog kinds %v: %v", args, err)
	}
	return out.Bytes()
}

func TestCatalogKindsVerbListsEveryKindAndOneKindsEntries(t *testing.T) {
	idx := fakeIndex{entries: []contribapi.Contribution{
		kindDecl("recipe", "A way to cook"),
		kindDecl("menu", "A list of dishes"),
		describedEntry("recipe", "soup", "Soup", "A hot soup"),
		describedEntry("recipe", "salad", "Salad", "A cold salad"),
	}}

	var kinds []map[string]any
	if err := json.Unmarshal(runKinds(t, idx), &kinds); err != nil {
		t.Fatalf("kinds output is not a JSON list: %v", err)
	}
	want := map[string][2]any{"menu": {"A list of dishes", float64(0)}, "recipe": {"A way to cook", float64(2)}}
	if len(kinds) != len(want) {
		t.Fatalf("listed %d kinds, want %d: %v", len(kinds), len(want), kinds)
	}
	for _, k := range kinds {
		w, ok := want[k["name"].(string)]
		if !ok || k["description"] != w[0] || k["count"] != w[1] {
			t.Errorf("kind printed as %v, want its description and count %v", k, w)
		}
	}

	var entries []map[string]any
	if err := json.Unmarshal(runKinds(t, idx, "recipe"), &entries); err != nil {
		t.Fatalf("kind entries output is not a JSON list: %v", err)
	}
	wantEntries := map[string][2]string{
		"ktw-example:/recipe/soup":  {"Soup", "A hot soup"},
		"ktw-example:/recipe/salad": {"Salad", "A cold salad"},
	}
	if len(entries) != len(wantEntries) {
		t.Fatalf("listed %d entries, want %d: %v", len(entries), len(wantEntries), entries)
	}
	for _, e := range entries {
		w, ok := wantEntries[e["uri"].(string)]
		if !ok || e["name"] != w[0] || e["description"] != w[1] {
			t.Errorf("entry printed as %v, want name and description %v", e, w)
		}
	}
}
