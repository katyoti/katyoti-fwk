// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
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

// A hub's snapshot request names a project's token: a process that does not
// serve it answers nothing - an empty set would wipe the entries another
// process serves for that project - while the project it serves is answered.
func TestASnapshotForAProjectNotServedAnswersNothing(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(fstest.MapFS{"object-types/deck-theme.yaml": {Data: []byte("name: deck-theme\n")}}),
		contrib.Pack(""))
	theme := cwmapi.Object{ID: "blue", Type: "deck-theme", Name: "Blue", Body: "# Blue",
		Capabilities: map[string]map[string]any{"contributes": {"kind": "deck-theme", "id": "blue", "layers": map[string]any{"source": "body"}}}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "themes", Object: theme}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var pushes []api.EntriesPush
	p := core.NewPusher(func(_ context.Context, e api.EntriesPush) error {
		mu.Lock()
		defer mu.Unlock()
		pushes = append(pushes, e)
		return nil
	}, nil)

	if err := p.Snapshot(ctx, "not-served-here"); err != nil {
		t.Fatal(err)
	}
	if len(pushes) != 0 {
		t.Fatalf("pushes for a project not served = %+v; want none", pushes)
	}
	if err := p.Snapshot(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(pushes) != 1 || len(pushes[0].Entries) != 1 {
		t.Fatalf("full snapshot = %+v; want the served project's one entry", pushes)
	}
	token := pushes[0].Token
	pushes = nil
	if err := p.Snapshot(ctx, token); err != nil {
		t.Fatal(err)
	}
	if len(pushes) != 1 || pushes[0].Token != token || pushes[0].Op != api.PushSnapshot || len(pushes[0].Entries) != 1 {
		t.Fatalf("snapshot of the served project = %+v; want its one entry", pushes)
	}
}
