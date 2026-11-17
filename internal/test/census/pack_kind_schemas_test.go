// SPDX-License-Identifier: Apache-2.0

package census

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
)

// A kind its pack declares is published by every binary carrying the pack,
// each under its own provider, each naming the schema it ships itself
// (`<provider>:/data-type/<schema>`). One kind holds only while those schemas
// are the same document. The hub catalog compares their digests at run time
// (schemaDisagreementsLocked); the census compares them across the goldens
// before anything runs: every binary declaring a pack kind ships the same
// schema digest, and ships the schema it names.

// golden is one binary's checked-in census, by its path.
type golden struct {
	Path string
	Text string
}

// packKindSchemaDisagreements reads the goldens and answers one finding per
// pack kind whose declarers ship a missing or differing schema. schemaOf
// names the schema id a kind's declaration names (its leaf); a kind several
// binaries declare with no leaf is a finding too: the census cannot name its
// schema, so it cannot compare it.
func packKindSchemaDisagreements(goldens []golden, schemaOf func(kind string) (string, bool)) []string {
	type declarer struct{ golden, provider string }
	declared := map[string][]declarer{}
	sources := map[string]map[string]string{} // golden -> data-type URI -> source digest
	for _, g := range goldens {
		sources[g.Path] = map[string]string{}
		for _, line := range strings.Split(normalise(g.Text), "\n") {
			cols := strings.Split(line, "\t")
			if len(cols) < 4 {
				continue
			}
			u, err := contribapi.ParseResourceURI(cols[0])
			if err != nil || u.Provider == contribapi.SystemProvider {
				continue
			}
			switch cols[1] {
			case contribapi.ResourceTypeContributionType:
				declared[u.ID] = append(declared[u.ID], declarer{g.Path, u.Provider})
			case contribapi.ResourceTypeDataType:
				sources[g.Path][cols[0]] = layerOf(cols[3], contribapi.LayerSource)
			}
		}
	}
	kinds := make([]string, 0, len(declared))
	for k := range declared {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var out []string
	for _, kind := range kinds {
		ds := declared[kind]
		if len(ds) < 2 {
			continue
		}
		schema, ok := schemaOf(kind)
		if !ok {
			out = append(out, "kind "+kind+": declared by "+strconv.Itoa(len(ds))+" binaries but no pack leaf names its schema - the census cannot compare it")
			continue
		}
		var want, wantFrom string
		for _, d := range ds {
			uri := contribapi.Contribution{Provider: d.provider, Type: contribapi.ResourceTypeDataType, ID: schema}.ResolvedURI()
			got := sources[d.golden][uri]
			if got == "" {
				out = append(out, "kind "+kind+": "+d.golden+" declares it but ships no "+uri)
				continue
			}
			if want == "" {
				want, wantFrom = got, d.golden+" ("+uri+")"
				continue
			}
			if got != want {
				out = append(out, "kind "+kind+": "+d.golden+" ("+uri+") ships schema "+got+", "+wantFrom+" ships "+want+"; one kind, one schema - the binaries carry different versions of its pack")
			}
		}
	}
	return out
}

// layerOf is one layer's hash in a census row's layers column.
func layerOf(layers, name string) string {
	for _, l := range strings.Split(layers, ",") {
		if v, ok := strings.CutPrefix(l, name+"="); ok {
			return v
		}
	}
	return ""
}

// leafSchema names a kind's schema from the pack leaves compiled in.
func leafSchema(kind string) (string, bool) {
	for _, l := range builtin.Leaves(kind) {
		return l.Schema.ID, true
	}
	return "", false
}

// TestPackKindsShipOneSchemaAcrossBinaries is the census's cross-binary guard:
// every binary declaring a pack kind ships the same schema digest.
func TestPackKindsShipOneSchemaAcrossBinaries(t *testing.T) {
	root := repoRoot(t)
	paths, _ := filepath.Glob(filepath.Join(root, "app", "*", goldenPath))
	if len(paths) == 0 {
		t.Fatal("census: no golden found: the check would prove nothing")
	}
	sort.Strings(paths)
	goldens := make([]golden, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		goldens = append(goldens, golden{Path: filepath.ToSlash(rel), Text: string(raw)})
	}
	for _, f := range packKindSchemaDisagreements(goldens, leafSchema) {
		t.Error(f)
	}
}

func TestPackKindSchemaDisagreements(t *testing.T) {
	row := func(uri, typ, source string) string {
		return uri + "\t" + typ + "\t-\tsource=" + source
	}
	kind := func(p string) string { return row(p+":/contribution-type/skill", "contribution-type", "sha256:decl") }
	schema := func(p, digest string) string { return row(p+":/data-type/skill", "data-type", digest) }
	skillLeaf := func(k string) (string, bool) { return "skill", k == "skill" }
	noLeaf := func(string) (string, bool) { return "", false }
	g := func(path string, lines ...string) golden { return golden{Path: path, Text: strings.Join(lines, "\n")} }

	cases := []struct {
		name     string
		goldens  []golden
		schemaOf func(string) (string, bool)
		want     []string // substrings, one per finding
	}{
		{"agreeing digests", []golden{
			g("a", kind("a"), schema("a", "sha256:1")),
			g("b", kind("b"), schema("b", "sha256:1")),
		}, skillLeaf, nil},
		{"one binary declares it: nothing to compare", []golden{
			g("a", kind("a"), schema("a", "sha256:1")),
			g("b", schema("b", "sha256:2")),
		}, noLeaf, nil},
		{"system kinds are not pack kinds", []golden{
			g("a", kind("system"), schema("system", "sha256:1")),
			g("b", kind("system"), schema("system", "sha256:2")),
		}, skillLeaf, nil},
		{"differing digests", []golden{
			g("a", kind("a"), schema("a", "sha256:1")),
			g("b", kind("b"), schema("b", "sha256:2")),
		}, skillLeaf, []string{"b (b:/data-type/skill) ships schema sha256:2"}},
		{"a declarer ships no schema", []golden{
			g("a", kind("a"), schema("a", "sha256:1")),
			g("b", kind("b")),
		}, skillLeaf, []string{"b declares it but ships no b:/data-type/skill"}},
		{"several declarers, no leaf", []golden{
			g("a", kind("a"), schema("a", "sha256:1")),
			g("b", kind("b"), schema("b", "sha256:1")),
		}, noLeaf, []string{"declared by 2 binaries but no pack leaf"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := packKindSchemaDisagreements(c.goldens, c.schemaOf)
			if len(got) != len(c.want) {
				t.Fatalf("findings = %q, want %d", got, len(c.want))
			}
			for i, w := range c.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("finding %q lacks %q", got[i], w)
				}
			}
		})
	}
}
