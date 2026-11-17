// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// The hub serves the object entries pushed for its project member, beside the
// member's folder files, with no restart (review R-A1): a put is served by
// Query, GetBody and Open; a remove withdraws it; a folder file and a pushed
// object with the same kind and id are both withheld, the finding naming
// both; a push for a token no member answers to changes nothing. On start the
// hub asks the project's snapshot.
func TestTheHubServesTheObjectEntriesPushedForItsProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nats, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(t.TempDir(), true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nats.Stop)
	bus := natsxport.New(nats.Client)
	const tenant, universe, token = "local", "example", "acme-shop"

	prev := projectMembers
	projectMembers = newProjectMemberRegistry()
	t.Cleanup(func() { projectMembers = prev })
	projectMembers.keepAll([]pcapi.ProjectMember{{Token: token}}, nil)
	// The universe's attached project answers to token: every read of the
	// membership keeps its member.
	project := t.TempDir()
	writeProject(t, project, "id: Acme Shop\n")
	if err := os.MkdirAll(filepath.Join(project, "modules", "sys-contributions"), 0o755); err != nil {
		t.Fatal(err)
	}
	prevRoots := attachedProjectRoots
	attachedProjectRoots = func() []string { return []string{project} }
	t.Cleanup(func() { attachedProjectRoots = prevRoots })

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
	write("contribution-types/recipe.yaml", "name: recipe\nlayers: {data: {default: true}, source: {}}\n")
	write("recipes/pancakes.yaml", "name: pancakes\n")
	sidecars := []appcore.SidecarSpec{{Name: token, ContentRoot: root}}
	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	if _, probed, n := hydrateCache(cache, sidecars, io.Discard); probed != 1 || n != 0 {
		t.Fatalf("hydrate: probed=%d errs=%d", probed, n)
	}
	cat, err := manifestlocal.NewCatalog(cache, catalogMembers(sidecars))
	if err != nil {
		t.Fatal(err)
	}
	svc := &catalogServiceImpl{cat: cat, cache: cache,
		members: func() ([]appcore.SidecarSpec, error) { return sidecars, nil }}

	reloaded := make(chan struct{}, 8)
	h := &projectPushHandler{
		members:  projectMembers,
		sidecars: func() ([]appcore.SidecarSpec, error) { return sidecars, nil },
		cache:    cache,
		reload: func() {
			if err := cat.(catalogReloader).Reload(); err != nil {
				t.Errorf("reload: %v", err)
			}
			reloaded <- struct{}{}
		},
	}

	asked := make(chan string, 1)
	if _, err := bus.Subscribe(ctx, pcapi.SnapshotRequestSubjectAll(tenant, universe), func(_ context.Context, m mbapi.Message) {
		asked <- m.Subject()
	}); err != nil {
		t.Fatal(err)
	}
	sub, err := serveProjectPushes(ctx, bus, tenant, universe, h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	select {
	case got := <-asked:
		if got != pcapi.SnapshotRequestSubject(tenant, universe, token) {
			t.Fatalf("snapshot asked on %s", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the hub asked no snapshot of its project")
	}

	entry := func(object, id string) pcapi.ObjectEntry {
		e, err := pcapi.EntryOfObject(ctx, cwmapi.Object{ID: object, Fields: map[string]any{"name": id},
			Capabilities: map[string]map[string]any{pcapi.CapContributes: {"kind": "recipe", "id": id, "layers": map[string]any{"data": "fields"}}}},
			token, nil)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	push := func(p pcapi.EntriesPush) {
		t.Helper()
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := bus.Publish(ctx, pcapi.PushSubject(tenant, universe, p.Token), b); err != nil {
			t.Fatal(err)
		}
	}
	waitReload := func() {
		t.Helper()
		select {
		case <-reloaded:
		case <-time.After(10 * time.Second):
			t.Fatal("the push rebuilt no record")
		}
	}
	served := func() string {
		t.Helper()
		out, err := svc.ListContributions(ctx, mbapi.CatalogListContributionsInput{Filter: contribapi.ListFilter{Providers: []string{token}, Types: []string{"recipe"}}})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, c := range out.Contributions {
			ids = append(ids, c.ID)
		}
		return strings.Join(ids, " ")
	}
	if got := served(); got != "pancakes" {
		t.Fatalf("before any push the member serves %q, want its folder file", got)
	}

	// A put: served by Query, GetBody and Open, no restart.
	uri := token + ":/recipe/waffles"
	waffles := entry("recipes.waffles", "waffles")
	push(pcapi.EntriesPush{Op: pcapi.PushPut, Token: token, Object: waffles.Object, Entry: &waffles})
	waitReload()
	if got := served(); got != "pancakes waffles" {
		t.Fatalf("after a put the member serves %q, want the file and the object", got)
	}
	body, err := svc.GetBody(ctx, mbapi.GetBodyInput{URI: uri})
	if err != nil || !strings.Contains(string(body.Content), "waffles") {
		t.Fatalf("GetBody = %q, %v; want the object's fields", body.Content, err)
	}
	opened, err := svc.OpenLayer(ctx, mbapi.OpenLayerInput{URI: uri, Layer: "data"})
	if err != nil || opened.Refused != "" || !strings.Contains(string(opened.Content), "waffles") {
		t.Fatalf("OpenLayer = %q (refused %q), %v; want the object's fields", opened.Content, opened.Refused, err)
	}

	// A push for a token no member answers to changes nothing.
	push(pcapi.EntriesPush{Op: pcapi.PushRemove, Token: "ktw-example", Object: waffles.Object})

	// A remove withdraws it (the unknown token's push, sent before, rebuilt nothing).
	push(pcapi.EntriesPush{Op: pcapi.PushRemove, Token: token, Object: waffles.Object})
	waitReload()
	if got := served(); got != "pancakes" {
		t.Fatalf("after a remove the member serves %q, want the folder file only (and the unknown token's push changed nothing)", got)
	}
	if _, err := svc.OpenLayer(ctx, mbapi.OpenLayerInput{URI: uri, Layer: "data"}); err == nil {
		t.Fatalf("OpenLayer of a removed entry answered")
	}

	// A file and an object with the same kind and id: both withheld, the
	// finding naming both.
	clash := entry("recipes.pancakes", "pancakes")
	push(pcapi.EntriesPush{Op: pcapi.PushPut, Token: token, Object: clash.Object, Entry: &clash})
	waitReload()
	if got := served(); got != "" {
		t.Fatalf("a clash serves %q, want neither side", got)
	}
	findings, err := projectMembers.get(token).Findings(ctx)
	if err != nil || len(findings) != 1 || !strings.Contains(findings[0], "object recipes.pancakes") || !strings.Contains(findings[0], "file "+token+":/recipe/pancakes") {
		t.Fatalf("findings = %v, %v; want one naming the file and the object", findings, err)
	}
	h.mu.Lock()
	logged := h.findings[token]
	h.mu.Unlock()
	if got := logged; len(got) != 1 {
		t.Fatalf("the hub logged %v, want the clash", got)
	}
	select {
	case <-reloaded:
		t.Fatal("a push rebuilt a record it should not have")
	default:
	}
}
