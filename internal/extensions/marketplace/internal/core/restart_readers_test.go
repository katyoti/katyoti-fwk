// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// THE MEMBERS THAT READ A KEY RESTART ONCE IT IS SAVED: only
// the running members that read a key written are restarted, through their
// universe's hub — a spy here: no hub runs, no member is started.

// spyMember is a member of a spied universe: whether it runs, the keys it
// reads, and whether its restart fails.
type spyMember struct {
	running bool
	reads   []string
	fails   bool
}

// spyWorld is the universes a spy hub answers for, and what was restarted.
type spyWorld struct {
	mu        sync.Mutex
	universes map[string]map[string]spyMember // home → member → state
	restarted []string                        // "<universe base>/<member>"
	started   []string                        // members not running that were started
}

type spyHub struct {
	w    *spyWorld
	home string
}

func (h spyHub) Running(context.Context) ([]string, error) {
	var out []string
	for name, m := range h.w.universes[h.home] {
		if m.running {
			out = append(out, name)
		}
	}
	return out, nil
}

func (h spyHub) Restart(_ context.Context, member string) (bool, error) {
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	m := h.w.universes[h.home][member]
	if !m.running {
		h.w.started = append(h.w.started, member)
		return false, nil
	}
	if m.fails {
		return false, errors.New("its start failed")
	}
	h.w.restarted = append(h.w.restarted, filepath.Base(h.home)+"/"+member)
	return true, nil
}

func (spyHub) Close() {}

// install makes s reach the spy: a universe it does not know has no hub.
func (w *spyWorld) install(s *service) {
	s.dialHub = func(_ context.Context, home string) (universeHub, error) {
		if _, ok := w.universes[filepath.Clean(home)]; !ok {
			return nil, nil
		}
		return spyHub{w: w, home: filepath.Clean(home)}, nil
	}
	s.keysOf = func(_ context.Context, home, member string) ([]string, error) {
		return w.universes[filepath.Clean(home)][member].reads, nil
	}
}

func (w *spyWorld) restartedSorted() string {
	out := append([]string(nil), w.restarted...)
	sort.Strings(out)
	return strings.Join(out, ",")
}

func restartedNames(rs []mktapi.MemberRestart) string {
	var out []string
	for _, r := range rs {
		s := filepath.Base(r.Universe) + "/" + r.Member + "[" + strings.Join(r.Keys, "+") + "]"
		if r.Error != "" {
			s += "!" + r.Error
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// A universe key restarts, in its universe, only the running members that
// read it: not a member reading another key, not a member not running.
func TestSavingAKeyRestartsOnlyTheMembersThatReadIt(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(w.home): {
			"reader":  {running: true, reads: []string{"API_TOKEN", "OTHER"}},
			"other":   {running: true, reads: []string{"OTHER"}},
			"stopped": {running: false, reads: []string{"API_TOKEN"}},
		},
	}}
	s := &service{}
	spy.install(s)

	out := s.restartReaders(context.Background(), []keyWrite{{home: w.home, scope: configset.ScopeUniverse, name: "API_TOKEN"}})

	if got := spy.restartedSorted(); got != "uni/reader" {
		t.Errorf("restarted %q, want only uni/reader", got)
	}
	if len(spy.started) != 0 {
		t.Errorf("members not running were started: %v", spy.started)
	}
	if got := restartedNames(out); got != "uni/reader[API_TOKEN]" {
		t.Errorf("the output lists %q, want uni/reader[API_TOKEN]", got)
	}
	// Idempotent: saving it again restarts the same member, nothing else.
	spy.restarted = nil
	s.restartReaders(context.Background(), []keyWrite{{home: w.home, scope: configset.ScopeUniverse, name: "API_TOKEN"}})
	if got := spy.restartedSorted(); got != "uni/reader" {
		t.Errorf("second save restarted %q", got)
	}
}

// A global key restarts the running members that read it in every
// registered universe; a universe whose hub does not run is left alone.
func TestAGlobalKeyRestartsItsReadersInEveryUniverse(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	second := filepath.Join(t.TempDir(), "second")
	stopped := filepath.Join(t.TempDir(), "stopped")
	regPath, err := orchcatalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := orchcatalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{w.home, second, stopped} {
		if err := os.MkdirAll(h, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := reg.Add(orchcatalog.RegistryEntry{Name: filepath.Base(h), HomePath: h}); err != nil {
			t.Fatal(err)
		}
	}
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(w.home): {"a": {running: true, reads: []string{"SHARED_URL"}}, "b": {running: true, reads: []string{"API_TOKEN"}}},
		filepath.Clean(second): {"c": {running: true, reads: []string{"SHARED_URL"}}},
		// "stopped" has no hub: never dialled into a member.
	}}
	s := &service{}
	spy.install(s)

	out := s.restartReaders(context.Background(), []keyWrite{{home: w.home, scope: configset.ScopeGlobal, name: "SHARED_URL"}})

	if got := spy.restartedSorted(); got != "second/c,uni/a" {
		t.Errorf("restarted %q, want second/c,uni/a", got)
	}
	if got := restartedNames(out); got != "second/c[SHARED_URL],uni/a[SHARED_URL]" {
		t.Errorf("the output lists %q", got)
	}
}

// A restart that fails is reported, the others still happen, and the save
// stands.
func TestARestartThatFailsIsReportedNotFatal(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(w.home): {
			"broken": {running: true, reads: []string{"API_TOKEN"}, fails: true},
			"fine":   {running: true, reads: []string{"API_TOKEN"}},
		},
	}}
	s := &service{}
	spy.install(s)
	out := s.restartReaders(context.Background(), []keyWrite{{home: w.home, scope: configset.ScopeUniverse, name: "API_TOKEN"}})
	if got := restartedNames(out); got != "uni/broken[API_TOKEN]!its start failed,uni/fine[API_TOKEN]" {
		t.Errorf("the output lists %q", got)
	}
}

// Through the save itself: an import writes its keys and restarts the
// members reading them; a dry run restarts nothing; a key that was already
// the same (nothing written) restarts nothing.
func TestAnImportRestartsTheMembersReadingWhatItWrote(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	w.set(t, configset.ScopeUniverse, map[string]string{"API_TOKEN": "tok"})
	w.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://x.example"})
	exp, err := (&service{}).ExportKeys(context.Background(), mktapi.ExportKeysInput{UniverseHome: w.home, Manifest: w.manifest, Password: keysPassword})
	if err != nil {
		t.Fatal(err)
	}
	w2 := newKeysWorld(t, twoKeys)
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(w2.home): {
			"agents": {running: true, reads: []string{"API_TOKEN"}},
			"web":    {running: true, reads: []string{"UNRELATED"}},
		},
	}}
	s := &service{}
	spy.install(s)
	in := mktapi.ImportKeysInput{UniverseHome: w2.home, File: exp.File, Password: keysPassword, DryRun: true}

	if out, err := s.ImportKeys(context.Background(), in); err != nil || len(out.Restarts) != 0 || len(spy.restarted) != 0 {
		t.Fatalf("a dry run restarted: %+v, %v", out.Restarts, err)
	}
	in.DryRun = false
	out, err := s.ImportKeys(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := restartedNames(out.Restarts); got != "uni/agents[API_TOKEN]" {
		t.Errorf("the import reports %q, want uni/agents[API_TOKEN]", got)
	}
	spy.restarted = nil
	again, err := s.ImportKeys(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Restarts) != 0 || len(spy.restarted) != 0 {
		t.Errorf("an import that wrote nothing restarted %v", spy.restarted)
	}
}

// The solution's key sets, imported with one password into its two
// universes: each running member reading a key written restarts ONCE —
// in each universe it runs in — after every key is saved.
func TestImportingKeySetsRestartsTheReadersOfEachUniverseOnce(t *testing.T) {
	w := newKeySetsWorld(t, teamPassword, teamPassword)
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(w.home): {
			"notes": {running: true, reads: []string{"KEY_A", "KEY_B"}},
			"idle":  {running: false, reads: []string{"KEY_A"}},
		},
		filepath.Clean(w.peer): {
			"cards": {running: true, reads: []string{"KEY_GLOBAL"}},
			"other": {running: true, reads: []string{"KEY_C"}},
		},
	}}
	spy.install(w.s)
	res, err := w.s.ImportKeySets(context.Background(), mktapi.ImportKeySetsInput{Solution: duo, UniverseHome: w.home, Password: teamPassword})
	if err != nil {
		t.Fatal(err)
	}
	if got := spy.restartedSorted(); got != "team-review/cards,team/notes" {
		t.Errorf("restarted %q, want team-review/cards,team/notes (each once)", got)
	}
	if got := restartedNames(res.Restarts); got != "team-review/cards[KEY_GLOBAL],team/notes[KEY_A+KEY_B]" {
		t.Errorf("the output lists %q", got)
	}
	if len(spy.started) != 0 {
		t.Errorf("a member not running was started: %v", spy.started)
	}
}

// The key form (a config-set applied with the values given): the running
// members reading a key it wrote restart; a dry run restarts nothing.
func TestApplyingAConfigSetRestartsTheReadersOfWhatItWrote(t *testing.T) {
	w := newKeySetsWorld(t, teamPassword, teamPassword)
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(w.home): {
			"notes": {running: true, reads: []string{"KEY_C"}},
			"cards": {running: true, reads: []string{"KEY_SET"}}, // already set: not written
		},
	}}
	spy.install(w.s)
	in := mktapi.ApplyConfigSetInput{Name: "base", Version: "1", UniverseHome: w.home, DryRun: true, Values: map[string]string{
		"KEY_A": "a", "KEY_B": "b", "KEY_C": "c", "KEY_GLOBAL": "g", "KEY_SET": "already",
	}}
	if out, err := w.s.ApplyConfigSet(context.Background(), in); err != nil || len(out.Restarts) != 0 || len(spy.restarted) != 0 {
		t.Fatalf("a dry run restarted: %+v, %v", out.Restarts, err)
	}
	in.DryRun = false
	out, err := w.s.ApplyConfigSet(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := restartedNames(out.Restarts); got != "team/notes[KEY_C]" {
		t.Errorf("the apply reports %q, want team/notes[KEY_C]", got)
	}
}

// A MEMBER READS THE KEYS ITS UNIVERSE'S CONFIG NAMES:
// ktw-agents declares ANTHROPIC_API_KEY, but a universe whose LLMProviders.yaml
// reads ${{env:ANTHROPIC_FOUNDRY_API_KEY}} reads that key: saving it restarts
// ktw-agents, so the next task finds it — the first AI task of a fresh install
// failed until the person restarted the universe by hand.
func TestAMemberReadsTheKeysItsConfigFilesName(t *testing.T) {
	dir := t.TempDir()
	providers := filepath.Join(dir, "LLMProviders.yaml")
	if err := os.WriteFile(providers, []byte("providers:\n  foundry:\n    apiKey: ${{env:ANTHROPIC_FOUNDRY_API_KEY}}\n    baseURL: ${{env:ANTHROPIC_FOUNDRY_BASE_URL}}\n  plain:\n    apiKey: ${{env:ANTHROPIC_API_KEY}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep := sidecarapi.ConfigCheckReport{
		Items: []sidecarapi.CheckItem{
			{Category: "env", Name: "ANTHROPIC_API_KEY"},
			{Category: "env", Name: "OPENAI_API_KEY"},
			{Category: "config", Name: "LLMProviders.yaml"},
		},
		ConfigFiles: []sidecarapi.ConfigFileDescriptor{
			{Name: "LLMProviders.yaml", Path: providers, Exists: true},
			{Name: "Absent.yaml", Path: filepath.Join(dir, "Absent.yaml"), Exists: false},
		},
	}
	got := strings.Join(readKeys(rep), ",")
	if want := "ANTHROPIC_API_KEY,OPENAI_API_KEY,ANTHROPIC_FOUNDRY_API_KEY,ANTHROPIC_FOUNDRY_BASE_URL"; got != want {
		t.Fatalf("readKeys = %q, want %q", got, want)
	}

	// Through the restart: the Foundry keys saved restart ktw-agents.
	home := filepath.Join(t.TempDir(), "uni")
	spy := &spyWorld{universes: map[string]map[string]spyMember{
		filepath.Clean(home): {"ktw-agents": {running: true}},
	}}
	s := &service{}
	spy.install(s)
	s.keysOf = func(context.Context, string, string) ([]string, error) { return readKeys(rep), nil }
	out := s.restartReaders(context.Background(), []keyWrite{
		{home: home, scope: configset.ScopeUniverse, name: "ANTHROPIC_FOUNDRY_API_KEY"},
		{home: home, scope: configset.ScopeUniverse, name: "ANTHROPIC_FOUNDRY_BASE_URL"},
	})
	if got := restartedNames(out); got != "uni/ktw-agents[ANTHROPIC_FOUNDRY_API_KEY+ANTHROPIC_FOUNDRY_BASE_URL]" {
		t.Errorf("restarts %q, want ktw-agents for the Foundry keys", got)
	}
}
