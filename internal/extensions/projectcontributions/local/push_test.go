// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	contrib "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// recorder keeps the pushes the publisher sends.
type recorder struct {
	mu     sync.Mutex
	pushes []api.EntriesPush
}

func (r *recorder) send(_ context.Context, p api.EntriesPush) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushes = append(r.pushes, p)
	return nil
}

// await is the first push after the n-th that ok accepts.
func (r *recorder) await(t *testing.T, from int, what string, ok func(api.EntriesPush) bool) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for i := from; i < len(r.pushes); i++ {
			if ok(r.pushes[i]) {
				r.mu.Unlock()
				return i + 1
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("no %s pushed; pushes = %+v", what, r.pushes)
	return 0
}

// The process hosting CWM serves object-backed entries live: an object saved
// with `contributes` pushes its entry (a put), a change replaces it, losing
// the capability, a delete and an archive push its removal, and the first event pushes
// the project's snapshot - which a project member applies, serving the entry
// through its providers with no restart.
func TestAnObjectCarryingContributesIsPushedLive(t *testing.T) {
	rec := &recorder{}
	ctx, cwm := cwmapp.New(t,
		// The in-process bus CWM publishes on (a host binary registers it).
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(fstest.MapFS{"object-types/deck-theme.yaml": {Data: []byte("name: deck-theme\n")}}),
		contrib.Pack(""), local.PushExtensionTo(rec.send))
	theme := cwmapi.Object{ID: "blue", Type: "deck-theme", Name: "Blue", Body: "# Blue",
		Capabilities: map[string]map[string]any{"contributes": {"kind": "deck-theme", "id": "blue", "layers": map[string]any{"source": "body"}}}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "themes", Object: theme}); err != nil {
		t.Fatal(err)
	}
	isPut := func(body string) func(api.EntriesPush) bool {
		return func(p api.EntriesPush) bool {
			return p.Op == api.PushPut && p.Entry != nil && p.Entry.Entry.Type == "deck-theme" &&
				strings.TrimSpace(string(p.Entry.Entry.Layers["source"].Inline)) == body
		}
	}
	n := rec.await(t, 0, "snapshot", func(p api.EntriesPush) bool { return p.Op == api.PushSnapshot })
	n = rec.await(t, 0, "put of the created object", isPut("# Blue"))
	token := rec.pushes[n-1].Token
	member := contrib.NewMember(token, nil)
	if err := member.Apply(rec.pushes[n-1]); err != nil {
		t.Fatal(err)
	}
	if got := served(t, member); len(got) != 1 || got[0] != token+":/deck-theme/blue" {
		t.Fatalf("served after the put = %v; want the object's entry", got)
	}

	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "themes", ID: "themes.blue"})
	if err != nil {
		t.Fatal(err)
	}
	changed := got.Object
	changed.Body = "# Blue, darker"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "themes", Object: changed}); err != nil {
		t.Fatal(err)
	}
	n = rec.await(t, n, "put of the changed object", isPut("# Blue, darker"))

	plain := changed
	plain.Capabilities = nil
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "themes", Object: plain}); err != nil {
		t.Fatal(err)
	}
	n = rec.await(t, n, "removal of an object that lost the capability", func(p api.EntriesPush) bool { return p.Op == api.PushRemove })
	if err := member.Apply(rec.pushes[n-1]); err != nil {
		t.Fatal(err)
	}
	if got := served(t, member); len(got) != 0 {
		t.Fatalf("served after the remove = %v; want nothing", got)
	}

	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: "themes", ID: changed.ID}); err != nil {
		t.Fatal(err)
	}
	n = rec.await(t, n, "removal of a deleted object", func(p api.EntriesPush) bool { return p.Op == api.PushRemove && p.Object == changed.ID })

	// An archived object leaves the index with one archived event: its entry
	// is withdrawn as a deleted one's.
	red := cwmapi.Object{ID: "red", Type: "deck-theme", Name: "Red", Body: "# Red",
		Capabilities: map[string]map[string]any{"contributes": {"kind": "deck-theme", "id": "red", "layers": map[string]any{"source": "body"}}}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "themes", Object: red}); err != nil {
		t.Fatal(err)
	}
	n = rec.await(t, n, "put of the second object", isPut("# Red"))
	if err := member.Apply(rec.pushes[n-1]); err != nil {
		t.Fatal(err)
	}
	if got := served(t, member); len(got) != 1 || got[0] != token+":/deck-theme/red" {
		t.Fatalf("served after the second put = %v; want the second object's entry", got)
	}
	if _, err := cwm.ObjectArchive(ctx, cwmapi.ObjectArchiveInput{ID: "themes.red"}); err != nil {
		t.Fatal(err)
	}
	n = rec.await(t, n, "removal of an archived object", func(p api.EntriesPush) bool { return p.Op == api.PushRemove && p.Object == "themes.red" })
	if err := member.Apply(rec.pushes[n-1]); err != nil {
		t.Fatal(err)
	}
	if got := served(t, member); len(got) != 0 {
		t.Fatalf("served after the archive = %v; want nothing", got)
	}
}

// served are the URIs a member's providers enumerate.
func served(t *testing.T, m *contrib.Member) []string {
	t.Helper()
	ps, err := m.Providers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range ps {
		items, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range items {
			out = append(out, p.Provider()+":/"+p.Type()+"/"+c.ID)
		}
	}
	return out
}

// A layer read from a module file (files/<path>) follows the file: writing
// it publishes no object event, yet the object's entry is pushed again with
// the new bytes, which the project member serves with no restart - and an
// entry whose layer file is missing is served once the file is written.
func TestAWrittenLayerFilePushesItsObjectAgain(t *testing.T) {
	rec := &recorder{}
	ctx, cwm := cwmapp.New(t,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(fstest.MapFS{"object-types/deck-theme.yaml": {Data: []byte("name: deck-theme\n")}}),
		contrib.Pack(""), local.PushExtensionTo(rec.send))
	green := cwmapi.Object{ID: "green", Type: "deck-theme", Name: "Green",
		Capabilities: map[string]map[string]any{"contributes": {"kind": "deck-theme", "id": "green", "layers": map[string]any{"source": "files/themes/green.md"}}}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "themes", Object: green}); err != nil {
		t.Fatal(err)
	}
	// The layer file is not written yet: the entry does not project.
	n := rec.await(t, 0, "removal of the entry whose layer file is missing", func(p api.EntriesPush) bool {
		return p.Op == api.PushRemove && strings.HasSuffix(p.Object, "green")
	})
	isPut := func(body string) func(api.EntriesPush) bool {
		return func(p api.EntriesPush) bool {
			return p.Op == api.PushPut && p.Entry != nil &&
				strings.TrimSpace(string(p.Entry.Entry.Layers["source"].Inline)) == body
		}
	}
	write := func(text string) {
		t.Helper()
		if _, err := cwm.ObjectFileWrite(ctx, cwmapi.ObjectFileWriteInput{ID: "themes.green", Path: "themes/green.md", Content: []byte(text)}); err != nil {
			t.Fatal(err)
		}
	}
	write("# Green")
	n = rec.await(t, n, "put once the layer file is written", isPut("# Green"))
	member := contrib.NewMember(rec.pushes[n-1].Token, nil)
	if err := member.Apply(rec.pushes[n-1]); err != nil {
		t.Fatal(err)
	}

	write("# Green, lighter")
	n = rec.await(t, n, "put of the rewritten layer file", isPut("# Green, lighter"))
	if err := member.Apply(rec.pushes[n-1]); err != nil {
		t.Fatal(err)
	}
	entries := member.ObjectEntries()
	if len(entries) != 1 || strings.TrimSpace(string(entries[0].Entry.Layers["source"].Inline)) != "# Green, lighter" {
		t.Fatalf("served = %+v; want the rewritten layer's bytes", entries)
	}
}
