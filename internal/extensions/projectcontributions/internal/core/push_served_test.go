// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	contrib "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The push serves only the projects the hub serves (api.ProjectMembers): two
// attached projects naming one token, the first attached is served and the
// second's object is pushed neither in a snapshot nor live - it would shadow
// the first's under its token; a project with no contributions folder gets no
// set and none of its objects is pushed.
func TestARefusedProjectsObjectsAreNeverPushedUnderAnotherProjectsToken(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	alpha := core.Project(t, repo, "alpha", "shop", true)
	beta := core.Project(t, repo, "beta", "shop", true)
	gamma := core.Project(t, repo, "gamma", "gamma", false)
	roots := []string{alpha, beta, gamma}
	core.AttachProjects(t, roots...)

	ctx, cwm := cwmapp.Members(t, root, roots,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(fstest.MapFS{"object-types/deck-theme.yaml": {Data: []byte("name: deck-theme\n")}}),
		contrib.Pack(""))
	theme := func(id string) cwmapi.Object {
		return cwmapi.Object{ID: id, Type: "deck-theme", Name: id, Body: "# " + id,
			Capabilities: map[string]map[string]any{"contributes": {"kind": "deck-theme", "id": "main", "layers": map[string]any{"source": "body"}}}}
	}
	for _, key := range []string{"alpha", "beta", "gamma"} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: key + ":themes", Object: theme(key)}); err != nil {
			t.Fatalf("create in %s: %v", key, err)
		}
	}
	var mu sync.Mutex
	var pushes []api.EntriesPush
	p := core.NewPusher(func(_ context.Context, e api.EntriesPush) error {
		mu.Lock()
		defer mu.Unlock()
		pushes = append(pushes, e)
		return nil
	}, nil)

	if err := p.Snapshot(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(pushes) != 1 || pushes[0].Token != "shop" || len(pushes[0].Entries) != 1 ||
		strings.TrimSpace(string(pushes[0].Entries[0].Entry.Layers["source"].Inline)) != "# alpha" {
		t.Fatalf("snapshot = %+v; want one set, shop, holding alpha's entry alone", pushes)
	}
	pushes = nil
	for _, key := range []string{"beta", "gamma"} {
		if err := p.PushObject(ctx, key, "themes."+key, "themes"); err != nil {
			t.Fatal(err)
		}
		if token, _ := p.TokenOf(ctx, cwm, key, "themes."+key, "themes"); token != "" {
			t.Errorf("%s's object resolves to token %q; want none (not served)", key, token)
		}
	}
	if len(pushes) != 0 {
		t.Fatalf("live pushes of objects not served = %+v; want none", pushes)
	}
	if token, err := p.TokenOf(ctx, cwm, "alpha", "themes.alpha", "themes"); err != nil || token != "shop" {
		t.Fatalf("alpha's object: token = %q, %v; want shop", token, err)
	}
}
