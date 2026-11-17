// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/hubdial"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/follow"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
)

// THE MEMBERS THAT READ A KEY RESTART ONCE IT IS SAVED. A
// universe's programs read their keys when they start; a key saved while the
// universe runs (ImportKeys / ImportVault, ImportKeySets, ApplyConfigSet) is
// used once each running member that reads it is started again — through the
// universe's hub, which owns its members. A member that does
// not run is left alone (it starts with the key), a member that reads none of
// the keys is not touched, and a restart that fails is reported, never undoing
// the save.

// keyWrite is one key saved in one scope of one universe.
type keyWrite struct {
	home, scope, name string
}

// universeHub is a running universe's hub, as the restart reaches it.
type universeHub interface {
	// Running is the members whose process runs.
	Running(ctx context.Context) ([]string, error)
	// Restart stops a running member and starts it again; false when it
	// was not running.
	Restart(ctx context.Context, member string) (bool, error)
	Close()
}

// hubDialer reaches the hub of the universe at home: nil (no error) when no
// hub runs there.
type hubDialer func(ctx context.Context, home string) (universeHub, error)

// keysReader is the keys a member reads in the universe at home.
type keysReader func(ctx context.Context, home, member string) ([]string, error)

// restartWait bounds one member's restart: its stop grace and its start.
const restartWait = 3 * time.Minute

// restartReaders restarts, in each universe the writes reach, the running
// members that read a key written: a universe key reaches its universe, a
// global key every registered universe.
func (s *service) restartReaders(ctx context.Context, writes []keyWrite) []mktapi.MemberRestart {
	if len(writes) == 0 {
		return nil
	}
	keysAt := map[string]map[string]bool{} // home → keys written that reach it
	add := func(home, name string) {
		home = filepath.Clean(home)
		if keysAt[home] == nil {
			keysAt[home] = map[string]bool{}
		}
		keysAt[home][name] = true
	}
	var everyUniverse []string
	for _, w := range writes {
		if w.scope != configset.ScopeGlobal {
			add(w.home, w.name)
			continue
		}
		if everyUniverse == nil {
			everyUniverse = s.universeHomes()
		}
		add(w.home, w.name)
		for _, h := range everyUniverse {
			add(h, w.name)
		}
	}
	homes := make([]string, 0, len(keysAt))
	for h := range keysAt {
		homes = append(homes, h)
	}
	sort.Strings(homes)

	dial, readsKeys := s.dialHub, s.keysOf
	if dial == nil {
		dial = dialUniverseHub
	}
	if readsKeys == nil {
		readsKeys = s.memberKeys
	}
	var out []mktapi.MemberRestart
	for _, home := range homes {
		out = append(out, restartIn(ctx, home, keysAt[home], dial, readsKeys)...)
	}
	return out
}

// restartIn restarts, in the universe at home, the running members that read
// one of keys (the reader rule is follow's, shared with the hub's watch).
func restartIn(ctx context.Context, home string, keys map[string]bool, dial hubDialer, readsKeys keysReader) []mktapi.MemberRestart {
	hub, err := dial(ctx, home)
	if err != nil {
		return []mktapi.MemberRestart{{Universe: home, Error: "its hub could not be reached: " + err.Error()}}
	}
	if hub == nil {
		return nil // the universe does not run: its members start with the keys
	}
	defer hub.Close()
	reads := func(ctx context.Context, member string) (follow.Reads, error) {
		ks, err := readsKeys(ctx, home, member)
		return follow.Reads{Keys: ks}, err
	}
	self := func(member string) bool { return isSelf(home, member) } // never stopped under the call it serves
	restarted, err := follow.RestartReaders(ctx, boundedHub{hub}, keys, nil, reads, self)
	if err != nil {
		return []mktapi.MemberRestart{{Universe: home, Error: "its hub did not say which members run: " + err.Error()}}
	}
	var out []mktapi.MemberRestart
	for _, r := range restarted {
		out = append(out, mktapi.MemberRestart{Universe: home, Member: r.Member, Keys: r.Keys, Error: r.Error})
	}
	return out
}

// boundedHub bounds each member's restart: its stop grace and its start.
type boundedHub struct{ universeHub }

func (h boundedHub) Restart(ctx context.Context, member string) (bool, error) {
	rctx, cancel := context.WithTimeout(ctx, restartWait)
	defer cancel()
	return h.universeHub.Restart(rctx, member)
}

// universeHomes is every registered universe's home.
func (s *service) universeHomes() []string {
	var out []string
	for _, u := range wire.RegisteredUniverses() {
		out = append(out, u.Home)
	}
	return out
}

// isSelf says whether member of the universe at home is this process (the
// control plane serving the save): restarting it would end the call.
func isSelf(home, member string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	ws := application.Env().WorkspaceDir()
	if ws == "" || !strings.EqualFold(name, member) {
		return false
	}
	a, b := filepath.Clean(ws), filepath.Clean(home)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// memberKeys is the keys a member reads in the universe at home: the env
// items of its config check (each key it declares, set or not) and the keys
// its config files read (readKeys), asked about
// THIS universe — the installed item's own check, else the binary the
// universe's manifest runs it with.
func (s *service) memberKeys(ctx context.Context, home, member string) ([]string, error) {
	if ins, err := s.installedItem(member); err == nil {
		rep, err := s.checkInstalled(ctx, home, ins)
		if err != nil {
			return nil, err
		}
		return readKeys(rep), nil
	}
	r, err := follow.MemberReads(ctx, home, member)
	return r.Keys, err
}

// readKeys is the keys a member reads, from its config check of a universe
// (follow.ReadsOf).
func readKeys(rep sidecarapi.ConfigCheckReport) []string { return follow.ReadsOf(rep).Keys }

// dialUniverseHub reaches the universe's hub (hubdial.Dial): nil when no
// hub runs there.
func dialUniverseHub(ctx context.Context, home string) (universeHub, error) {
	c, err := hubdial.Dial(ctx, home)
	if c == nil {
		return nil, err
	}
	return c, nil
}

// importWrites is what an import wrote into the universe at home: each key
// written, with its scope.
func importWrites(home string, keys []mktapi.KeyEntry) []keyWrite {
	var out []keyWrite
	for _, k := range keys {
		if k.State == "written" {
			out = append(out, keyWrite{home: home, scope: k.Scope, name: k.Name})
		}
	}
	return out
}

// applyWrites is what a config-set apply wrote: each key written, with the
// scope its aggregated state gives.
func applyWrites(out mktapi.ApplyConfigSetOutput) []keyWrite {
	scope := map[string]string{}
	for _, k := range out.Keys {
		scope[k.Name] = k.Scope
	}
	var w []keyWrite
	for _, n := range out.KeysWritten {
		w = append(w, keyWrite{home: out.UniverseHome, scope: scope[n], name: n})
	}
	return w
}
