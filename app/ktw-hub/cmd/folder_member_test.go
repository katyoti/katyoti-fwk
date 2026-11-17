// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// A FOLDER member (content_root, no binary) is read by the hub itself: no
// process is started; its skills, tasks and own kind reach its record, and
// so do entries of a kind another member declares — the binaries are
// hydrated first. A folder no kind claims is left out.
func TestAFolderMemberIsReadByTheHubWithNoProcess(t *testing.T) {
	var probes atomic.Int32
	prev := probeManifest
	probeManifest = func(binary, _ string, _ bool) (*mbapi.Manifest, error) {
		probes.Add(1)
		widget := contribapi.Contribution{Provider: "ktw-a", Type: contribapi.ResourceTypeContributionType, ID: "widget"}
		widget.SetData([]byte(`{"name":"widget","layers":{"data":{"default":true},"source":{}},` +
			`"source":{"folder":"widgets","format":"structured","layers":{"data":{"from":"tree"},"source":{"from":"file"}}}}`))
		return &mbapi.Manifest{Provider: "ktw-a", Contributions: []contribapi.Contribution{widget}}, nil
	}
	t.Cleanup(func() { probeManifest = prev })

	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	saveHubRecord(t, cache, bin)
	exe := filepath.Join(bin, "ktw-a.exe")
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ktw-host.yaml", "name: pack\ndirs:\n  recipe: recipes\n")
	write("skills/greet.md", "---\nname: greet\ndescription: Say hello\n---\nSay hello.\n")
	write("tasks/review.md", "---\nname: review\ntype: agent\ndescription: d\n---\nReview.\n")
	write("contribution-types/recipe.yaml", "name: recipe\nlayers: {data: {default: true}, source: {}}\n")
	write("recipes/pancakes.yaml", "name: pancakes\n")
	write("widgets/clock.yaml", "face: round\n")
	write("config/settings.yaml", "x: 1\n")
	sidecars := []appcore.SidecarSpec{{Name: "ktw-a", Binary: exe}, {Name: "pack", ContentRoot: root}}

	var errs strings.Builder
	_, probed, n := hydrateCache(cache, sidecars, &errs)
	if n != 0 || probed != 2 {
		t.Fatalf("hydrate: probed=%d errs=%d (%s), want both members recorded", probed, n, errs.String())
	}
	if got := probes.Load(); got != 1 {
		t.Fatalf("%d probes, want 1: the binary member's only — a folder member starts no process", got)
	}
	m, _, err := cache.Load("pack", recordSource(sidecars[1]))
	if err != nil || m == nil {
		t.Fatalf("no record of the folder member: %v", err)
	}
	var got []string
	for _, c := range m.Contributions {
		got = append(got, c.Type+"/"+c.ID)
	}
	sort.Strings(got)
	want := "contribution-type/recipe recipe/pancakes skill/greet task/review widget/clock"
	if strings.Join(got, " ") != want {
		t.Fatalf("folder member's record = %v, want %s", got, want)
	}

	// Unchanged: fresh, nothing read again. Edited: read again.
	hydrated, probed, _ := hydrateCache(cache, sidecars, io.Discard)
	if hydrated != 2 || probed != 0 {
		t.Fatalf("unchanged folder: hydrated=%d probed=%d, want 2/0", hydrated, probed)
	}
	write("skills/wave.md", "---\nname: wave\ndescription: Wave\n---\nWave.\n")
	if _, probed, _ := hydrateCache(cache, sidecars, io.Discard); probed != 1 {
		t.Fatalf("edited folder: probed=%d, want the folder member read again", probed)
	}
}

// A folder member's layers are served by the hub from its folder: Read and
// OpenLayer answer with no process acquired — the application here has no
// connection manager at all, so a fall-through to one would fail.
func TestAFolderMembersLayersAreServedFromDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "greet.md"), []byte("---\nname: greet\ndescription: Say hello\n---\nSay hello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &appcore.UniverseSidecars{Sidecars: []appcore.SidecarSpec{{Name: "pack", ContentRoot: root}}}
	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	if _, probed, n := hydrateCache(cache, reg.Sidecars, io.Discard); probed != 1 || n != 0 {
		t.Fatalf("hydrate: probed=%d errs=%d", probed, n)
	}
	cat, err := manifestlocal.NewCatalog(cache, catalogMembers(reg.Sidecars))
	if err != nil {
		t.Fatal(err)
	}
	svc := &catalogServiceImpl{cat: cat, cache: cache,
		members: func() ([]appcore.SidecarSpec, error) { return reg.Sidecars, nil }}
	ctx := context.Background()

	// The instructions layer is not carried in the index: it comes from disk.
	opened, err := svc.OpenLayer(ctx, mbapi.OpenLayerInput{URI: "pack:/skill/greet", Layer: "instructions"})
	if err != nil || opened.Refused != "" || strings.TrimSpace(string(opened.Content)) != "Say hello." {
		t.Fatalf("OpenLayer(instructions) = %q (refused %q), %v; want the body from the folder", opened.Content, opened.Refused, err)
	}
	read, err := svc.Read(ctx, mbapi.CatalogReadInput{URI: "pack:/skill/greet"})
	if err != nil || !strings.Contains(string(read.Content), "Say hello.") {
		t.Fatalf("Read = %q, %v; want the skill's readable content from the folder", read.Content, err)
	}
}

// An entry naming the generic host with a content root that has no scripts/
// is a folder member: the hub reads it, starts nothing, and hands connmgr no
// binary to spawn.
func TestAGenericHostEntryWithoutScriptsIsAFolderMember(t *testing.T) {
	var probes atomic.Int32
	prev := probeManifest
	probeManifest = func(string, string, bool) (*mbapi.Manifest, error) {
		probes.Add(1)
		return &mbapi.Manifest{}, nil
	}
	t.Cleanup(func() { probeManifest = prev })
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	saveHubRecord(t, cache, bin)
	host := filepath.Join(bin, "ktw-host.exe")
	if err := os.WriteFile(host, []byte("host"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "greet.md"), []byte("---\nname: greet\ndescription: Say hello\n---\nSay hello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := appcore.SidecarSpec{Name: "pack", Binary: host, ContentRoot: root, Args: []string{"--folder", root, "sc", "ktsp", "nats"}}
	if _, probed, n := hydrateCache(cache, []appcore.SidecarSpec{spec}, io.Discard); probed != 1 || n != 0 {
		t.Fatalf("hydrate: probed=%d errs=%d, want the folder read", probed, n)
	}
	if got := probes.Load(); got != 0 {
		t.Fatalf("%d probes of the host, want none", got)
	}
	if s := spawnSpecOf(spec); s.Binary != "" || s.Folder == "" {
		t.Fatalf("spawn spec = %+v, want no binary and the folder", s)
	}
}

// The attached project's contributions folder is a folder member named by its
// provider token: its skill is recorded under the token with no process; a
// project with no such folder, or a token a member already answers to, adds
// no member.
func TestAProjectsContributionsAreAFolderMember(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "project.cwm.yaml"), []byte("id: Acme Shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	none := &appcore.UniverseSidecars{}
	if reg, refused, tokens := membershipOf(none, []string{project}); len(reg.Sidecars) != 0 || refused != nil || tokens != nil {
		t.Fatal("a project with no contributions folder adds a member, or is refused")
	}
	skills := filepath.Join(project, "modules", "sys-contributions", "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skills, "greet.md"), []byte("---\nname: greet\ndescription: Say hello\n---\nSay hello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, _, _ := membershipOf(none, []string{project})
	if len(reg.Sidecars) != 1 || reg.Sidecars[0].Name != "acme-shop" || !reg.Sidecars[0].IsFolder() {
		t.Fatalf("members = %+v; want the folder member acme-shop", reg.Sidecars)
	}
	m := reg.Sidecars[0]
	// The token rule: a member's name, the system's or the hub's is refused,
	// and the finding names the token and what answers to it.
	declared := &appcore.UniverseSidecars{Sidecars: []appcore.SidecarSpec{{Name: "acme-shop", Binary: "x.exe"}}}
	if reg, refused, _ := membershipOf(declared, []string{project}); len(reg.Sidecars) != 1 ||
		!strings.Contains(refused["acme-shop"], `"acme-shop"`) || !strings.Contains(refused["acme-shop"], "member acme-shop") {
		t.Fatalf("a token a member answers to: members %+v, refused %q; want refused, naming both", reg.Sidecars, refused)
	}
	for _, reserved := range []string{"system", hubProvider} {
		writeProject(t, project, "id: Acme Shop\ncapabilities:\n  contributions:\n    provider: "+reserved+"\n")
		if reg, refused, _ := membershipOf(none, []string{project}); len(reg.Sidecars) != 0 || !strings.Contains(refused[reserved], "reserved") {
			t.Fatalf("token %s: members %+v, refused %q; want refused as reserved", reserved, reg.Sidecars, refused)
		}
	}
	writeProject(t, project, "id: Acme Shop\n")
	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	if _, probed, n := hydrateCache(cache, []appcore.SidecarSpec{m}, io.Discard); probed != 1 || n != 0 {
		t.Fatalf("hydrate: probed=%d errs=%d", probed, n)
	}
	rec, _, err := cache.Load(m.Name, recordSource(m))
	if err != nil || rec == nil {
		t.Fatalf("no record of the project's contributions: %v", err)
	}
	if len(rec.Contributions) != 1 || rec.Contributions[0].Provider != "acme-shop" || rec.Contributions[0].ID != "greet" {
		t.Fatalf("record = %+v, want acme-shop's skill greet", rec.Contributions)
	}
}

func writeProject(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "project.cwm.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
