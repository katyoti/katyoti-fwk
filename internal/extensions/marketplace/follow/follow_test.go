// SPDX-License-Identifier: Apache-2.0

package follow

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeHub is a universe's hub: the members running, the restarts asked.
type fakeHub struct {
	mu        sync.Mutex
	running   []string
	restarted []string
	fail      map[string]bool
}

func (h *fakeHub) Running(context.Context) ([]string, error) { return h.running, nil }

func (h *fakeHub) Restart(_ context.Context, m string) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fail[m] {
		return false, errors.New("did not start")
	}
	h.restarted = append(h.restarted, m)
	return true, nil
}

func (h *fakeHub) got() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.restarted...)
}

func readsFrom(m map[string]Reads) ReadsFunc {
	return func(_ context.Context, member string) (Reads, error) {
		r, ok := m[member]
		if !ok {
			return Reads{}, errors.New("no check")
		}
		return r, nil
	}
}

// Only the running members that read a changed key or file restart; a failed
// restart is reported; the member skipped is never stopped.
func TestOnlyTheRunningReadersOfAChangeRestart(t *testing.T) {
	hub := &fakeHub{running: []string{"ktw-agents", "ktw-docs", "ktw-display", "ktw-self", "ktw-broken"}, fail: map[string]bool{"ktw-broken": true}}
	reads := readsFrom(map[string]Reads{
		"ktw-agents":  {Keys: []string{"ANTHROPIC_API_KEY"}, Files: []string{"/u/config/LLMProviders.yaml"}},
		"ktw-docs":    {Keys: []string{"CONFLUENCE_TOKEN"}, Files: []string{"/u/config/DocService.yaml"}},
		"ktw-display": {},
		"ktw-self":    {Keys: []string{"ANTHROPIC_API_KEY"}},
		"ktw-broken":  {Keys: []string{"ANTHROPIC_API_KEY"}},
		"ktw-idle":    {Keys: []string{"ANTHROPIC_API_KEY"}}, // not running
	})
	got, err := RestartReaders(context.Background(), hub,
		map[string]bool{"ANTHROPIC_API_KEY": true}, map[string]bool{"/u/config/DocService.yaml": true},
		reads, func(m string) bool { return m == "ktw-self" })
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ktw-agents", "ktw-docs"}; !reflect.DeepEqual(hub.got(), want) {
		t.Fatalf("restarted %v, want %v", hub.got(), want)
	}
	if len(got) != 3 || got[0].Member != "ktw-agents" || got[1].Member != "ktw-broken" || got[1].Error == "" || got[2].Files[0] != "/u/config/DocService.yaml" {
		t.Fatalf("the restarts said: %+v", got)
	}
	none, _ := RestartReaders(context.Background(), hub, nil, nil, reads, nil)
	if none != nil {
		t.Fatalf("nothing changed, yet %v", none)
	}
}

// A key set, changed or removed is a change; a config file changed or removed
// is one, a config file that appears is not (a member's first-run file).
func TestChangesAreTheKeysAndFilesThatMoved(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	cfg := filepath.Join(dir, "config")
	must(t, os.MkdirAll(cfg, 0o755))
	must(t, os.WriteFile(env, []byte("A=1\nB=2\nC=3\n"), 0o600))
	must(t, os.WriteFile(filepath.Join(cfg, "kept.yaml"), []byte("x: 1\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(cfg, "edited.yaml"), []byte("x: 1\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(cfg, "gone.yaml"), []byte("x: 1\n"), 0o644))
	p := Places{EnvFiles: []string{env, filepath.Join(dir, "absent.env")}, ConfigDirs: []string{cfg}}
	before := Take(p)
	must(t, os.WriteFile(env, []byte("A=1\nB=20\nD=4\n"), 0o600))
	must(t, os.WriteFile(filepath.Join(cfg, "edited.yaml"), []byte("x: 2\n"), 0o644))
	must(t, os.Remove(filepath.Join(cfg, "gone.yaml")))
	must(t, os.WriteFile(filepath.Join(cfg, "new.yaml"), []byte("x: 1\n"), 0o644))
	keys, files := Changes(before, Take(p))
	if want := map[string]bool{"B": true, "C": true, "D": true}; !reflect.DeepEqual(keys, want) {
		t.Errorf("keys changed %v, want %v", keys, want)
	}
	if want := map[string]bool{filepath.Join(cfg, "edited.yaml"): true, filepath.Join(cfg, "gone.yaml"): true}; !reflect.DeepEqual(files, want) {
		t.Errorf("files changed %v, want %v", files, want)
	}
}

// The watch of a universe's places: a key saved in its .env by hand restarts
// the member that reads it, a config file edited restarts the member that
// reads that file, and nothing else moves.
func TestTheWatchRestartsTheReadersOfAKeyOrAConfigFileChanged(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	global := filepath.Join(dir, "global", ".env")
	cfg := filepath.Join(dir, "config")
	must(t, os.MkdirAll(cfg, 0o755))
	must(t, os.MkdirAll(filepath.Dir(global), 0o755))
	must(t, os.WriteFile(env, []byte("ANTHROPIC_API_KEY=old\n"), 0o600))
	docCfg := filepath.Join(cfg, "DocService.yaml")
	must(t, os.WriteFile(docCfg, []byte("space: A\n"), 0o644))
	hub := &fakeHub{running: []string{"ktw-agents", "ktw-docs", "ktw-display"}}
	reads := readsFrom(map[string]Reads{
		"ktw-agents":  {Keys: []string{"ANTHROPIC_API_KEY"}},
		"ktw-docs":    {Keys: []string{"CONFLUENCE_TOKEN"}, Files: []string{docCfg}},
		"ktw-display": {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, Places{EnvFiles: []string{env, global}, ConfigDirs: []string{cfg}}, hub, reads, nil, io.Discard, ready)
	}()
	<-ready

	must(t, os.WriteFile(env, []byte("ANTHROPIC_API_KEY=new\n"), 0o600))
	waitFor(t, hub, []string{"ktw-agents"})
	must(t, os.WriteFile(docCfg, []byte("space: B\n"), 0o644))
	waitFor(t, hub, []string{"ktw-agents", "ktw-docs"})
	must(t, os.WriteFile(global, []byte("OTHER=1\n"), 0o600))
	time.Sleep(time.Second)
	if got := hub.got(); len(got) != 2 {
		t.Fatalf("a key no member reads restarted %v", got)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, hub *fakeHub, want []string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if reflect.DeepEqual(hub.got(), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("restarted %v, want %v", hub.got(), want)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
