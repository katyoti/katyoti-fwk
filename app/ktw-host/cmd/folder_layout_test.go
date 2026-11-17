// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestHostReadsAFolderAsAPack: the host reads a served folder as a pack —
// each folder a kind claims as that kind (skills/, references/, guide/), a
// folder no kind claims left out (notes/), its CWM pack from cwm/ — and its
// `dirs:` as remaps: a kind read from another folder, one switched off.
func TestHostReadsAFolderAsAPack(t *testing.T) {
	census.Isolate(t)
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ktw-host.yaml", "name: layout-pack\ndirs:\n  references: docs/refs\n  tasks: \"\"\n")
	write("skills/greet.md", "---\nname: greet\ndescription: Say hello\n---\nSay hello.\n")
	write("docs/refs/guide.md", "# A reference\n")
	write("references/ignored.md", "# not read: references come from docs/refs\n")
	write("tasks/review.md", "---\nname: review\ntype: agent\ndescription: d\n---\nReview.\n")
	write("guide/tour.yaml", "name: Tour\ndescription: a tour\nsteps: []\n")
	write("notes/a.yaml", "title: no kind's folder\n")
	write("cwm/object-types/memo.yaml", "name: memo\n")
	t.Setenv(contentRootEnv, dir)

	app := application.New(application.Config(), buildExtensions(embed.FS{})...)
	if err := app.Run(); err != nil {
		t.Fatalf("run host over the folder: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	m, err := manifestlocal.NewBuilder("layout-pack", "1.0.0").Build(ctx)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	var got []string
	for _, c := range m.Contributions {
		if c.Provider == "layout-pack" {
			got = append(got, c.Type+"/"+c.ID)
		}
	}
	sort.Strings(got)
	want := []string{"guide-recipe/tour", "object-type/memo", "reference/guide.md", "skill/greet"}
	var have []string
	for _, g := range got {
		for _, w := range want {
			if g == w {
				have = append(have, g)
			}
		}
		for _, bad := range []string{"task/", "reference/ignored.md", "notes/", "note/"} {
			if strings.HasPrefix(g, bad) || g == bad {
				t.Errorf("published %s; want it left out", g)
			}
		}
	}
	if strings.Join(have, " ") != strings.Join(want, " ") {
		t.Errorf("published %v; want %v among them", got, want)
	}
}
