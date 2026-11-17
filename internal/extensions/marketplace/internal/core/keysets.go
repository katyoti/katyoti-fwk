// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
)

// A solution's key sets are its vaults: sealed keys files published in its
// marketplace (solutions.md §6). One team password opens them all; the keys
// none of them provides are what the universe's config check still reports,
// asked in one form (the Ready to start screen, `katyoti solution keys`).

// openedSet is one key set opened: its sealed bytes, the password that opened
// it, and what it holds.
type openedSet struct {
	file     []byte
	password string
	bundle   keysBundle
}

// openKeySets opens each vault with the password passwordFor gives it. A vault
// given no password stays closed (not refused); one the password does not open
// is refused, named in refused. Nothing is written.
func (s *service) openKeySets(ctx context.Context, vaults []mktapi.PinnedRef, passwordFor func(name string) string) (states []mktapi.VaultState, opened []openedSet, refused []string) {
	for _, v := range vaults {
		st := mktapi.VaultState{Name: v.Name, Version: v.Version}
		file, err := s.vaultFile(ctx, v)
		password := passwordFor(v.Name)
		if err == nil && password == "" {
			// No password given (the solution setup installs: nobody to ask): the
			// vault stays closed, its keys imported later (the Ready to start
			// screen, `katyoti solution keys`, `katyoti vault import`). A key
			// the universe cannot do without is still refused by the key gate.
			st.Error = "not opened: no password given — import it later"
			states = append(states, st)
			continue
		}
		if err == nil {
			var b keysBundle
			if b, err = openKeys(file, password); err == nil {
				st.Opened = true
				for _, e := range b.Keys {
					st.Keys = append(st.Keys, e.Name)
				}
				opened = append(opened, openedSet{file: file, password: password, bundle: b})
			}
		}
		if err != nil {
			st.Error = err.Error()
			refused = append(refused, v.Name)
		}
		states = append(states, st)
	}
	return states, opened, refused
}

// keySetsScope is a solution resolved against the universe it was installed
// into: its version, and each of its universes found on this machine, the
// main one first.
type keySetsScope struct {
	label     string
	iv        *mktapi.ItemVersion
	universes []universeAt
}

type universeAt struct {
	u    mktapi.SolutionUniverse
	home string
}

func (sc keySetsScope) homes() []string {
	out := make([]string, 0, len(sc.universes))
	for _, u := range sc.universes {
		out = append(out, u.home)
	}
	return out
}

// resolveKeySets finds the solution (none: the universe alone) and its
// universes: the main one is the universe named, which must exist; each other
// one is beside it as <main>-<its name>, taken when it was installed.
func (s *service) resolveKeySets(ctx context.Context, ref mktapi.ItemRef, universe string) (keySetsScope, error) {
	var sc keySetsScope
	home, err := keysUniverse(universe)
	if err != nil {
		return sc, err
	}
	if strings.TrimSpace(ref.Name) == "" {
		sc.universes = []universeAt{{home: home, u: mktapi.SolutionUniverse{Main: true}}}
		return sc, nil
	}
	it, _, _, err := s.findItem(ctx, ref.Name, ref.Marketplace)
	if err != nil {
		return sc, err
	}
	if it.Kind != mktapi.KindSolution {
		return sc, fmt.Errorf("%q is a %s item, not a solution", ref.Name, it.Kind)
	}
	_, iv, err := pickVersion(it, ref.Version)
	if err != nil {
		return sc, err
	}
	sc.label, sc.iv = it.Name+"@"+iv.Version, &iv
	for _, u := range iv.SolutionUniverses() {
		h := solutionUniverseHome(home, u)
		if st, err := os.Stat(h); err != nil || !st.IsDir() {
			continue // a universe of the solution not installed here
		}
		sc.universes = append(sc.universes, universeAt{u: u, home: h})
	}
	return sc, nil
}

func (sc keySetsScope) vaults() []mktapi.PinnedRef {
	if sc.iv == nil {
		return nil
	}
	return sc.iv.Vaults
}

// KeySets says which key sets the solution provides, tries the password when
// one is given (nothing written), and lists the keys still missing.
func (s *service) KeySets(ctx context.Context, in mktapi.KeySetsInput) (mktapi.KeySetsOutput, error) {
	var out mktapi.KeySetsOutput
	sc, err := s.resolveKeySets(ctx, in.Solution, in.UniverseHome)
	if err != nil {
		return out, err
	}
	out.Solution, out.Universes = sc.label, sc.homes()
	provided := map[string]bool{}
	for _, v := range sc.vaults() {
		out.KeySets = append(out.KeySets, mktapi.VaultState{Name: v.Name, Version: v.Version})
	}
	out.PasswordNeeded = len(out.KeySets) > 0
	if in.Password != "" && len(out.KeySets) > 0 {
		states, opened, _ := s.openKeySets(ctx, sc.vaults(), func(string) string { return in.Password })
		out.KeySets = states
		out.SharedPassword = len(opened) == len(states)
		for _, o := range opened {
			for _, e := range o.bundle.Keys {
				provided[e.Name] = true
			}
		}
	}
	out.Remaining, err = s.remainingKeys(ctx, sc, provided)
	return out, err
}

// ImportKeySets opens every key set with the one password FIRST — a set it
// does not open refuses the whole import, nothing written — then imports each
// into every universe of the solution, restarts the running members that read
// a key written, and lists the keys still missing.
func (s *service) ImportKeySets(ctx context.Context, in mktapi.ImportKeySetsInput) (mktapi.ImportKeySetsOutput, error) {
	var out mktapi.ImportKeySetsOutput
	if strings.TrimSpace(in.Solution.Name) == "" {
		return out, fmt.Errorf("name the solution whose key sets to import")
	}
	sc, err := s.resolveKeySets(ctx, in.Solution, in.UniverseHome)
	if err != nil {
		return out, err
	}
	out.Solution, out.Universes = sc.label, sc.homes()
	if len(sc.vaults()) == 0 {
		return out, fmt.Errorf("solution %s provides no key set", sc.label)
	}
	if in.Password == "" {
		return out, fmt.Errorf("give the team password: it opens the key sets of %s", sc.label)
	}
	states, opened, refused := s.openKeySets(ctx, sc.vaults(), func(string) string { return in.Password })
	out.KeySets = states
	if len(refused) > 0 {
		return out, fmt.Errorf("solution %s: the password does not open %s (nothing was imported)", sc.label, strings.Join(refused, ", "))
	}
	written := map[string]bool{}
	var writes []keyWrite
	for i, u := range sc.universes {
		for _, o := range opened {
			res, home, err := s.importKeys(ctx, mktapi.ImportKeysInput{UniverseHome: u.home, File: o.file, Password: o.password, Overwrite: in.Overwrite})
			if err != nil {
				return out, fmt.Errorf("solution %s: import into %s: %w", sc.label, u.home, err)
			}
			writes = append(writes, importWrites(home, res.Keys)...)
			if i == 0 {
				out.Keys = append(out.Keys, res.Keys...)
			}
			for _, n := range res.Written {
				if !written[n] {
					written[n] = true
					out.Written = append(out.Written, n)
				}
			}
		}
	}
	// Every key written, in every universe, then the members reading them
	// restarted once. A global key is written once (the first
	// universe's import) and reaches every universe of the solution, even one
	// the registry does not list.
	for _, wr := range append([]keyWrite(nil), writes...) {
		if wr.scope == configset.ScopeGlobal {
			for _, u := range sc.universes {
				writes = append(writes, keyWrite{home: u.home, scope: wr.scope, name: wr.name})
			}
		}
	}
	out.Restarts = s.restartReaders(ctx, writes)
	out.Remaining, err = s.remainingKeys(ctx, sc, nil)
	return out, err
}

// remainingKeys are the required keys still without a value, in the
// universe's config check: each config-set's report (its manifest's keys and
// each wired sidecar's own check — ApplyConfigSet's dry run), or, for a
// universe without one, its sidecars' checks alone (ConfigSet "": the
// universe's own key set, which ApplyConfigSet with an empty name saves); a
// key an opened key set provides is left out. A global key is listed once.
func (s *service) remainingKeys(ctx context.Context, sc keySetsScope, provided map[string]bool) ([]mktapi.RemainingKey, error) {
	var out []mktapi.RemainingKey
	seen := map[string]bool{}
	add := func(home, configSet string, k mktapi.ConfigSetKeyState) {
		if k.Present || k.Optional || provided[k.Name] {
			return
		}
		id := k.Name
		if k.Scope != configset.ScopeGlobal {
			id = home + "\x00" + k.Name
		}
		if seen[id] {
			return
		}
		seen[id] = true
		scope := k.Scope
		if scope == "" {
			scope = configset.ScopeUniverse
		}
		out = append(out, mktapi.RemainingKey{
			Name: k.Name, Description: k.Description, Secret: k.Secret, Scope: scope,
			Source: k.Source, UniverseHome: home, ConfigSet: configSet,
		})
	}
	for _, u := range sc.universes {
		if len(u.u.ConfigSets) > 0 {
			for _, cs := range u.u.ConfigSets {
				dry, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{Name: cs.Name, Version: cs.Version, UniverseHome: u.home, DryRun: true})
				if err != nil {
					return nil, fmt.Errorf("config-set %s: %w", cs.Name, err)
				}
				for _, k := range dry.Keys {
					add(u.home, dry.Name+"@"+dry.Version, k)
				}
			}
			continue
		}
		se, err := readScopedEnv(u.home)
		if err != nil {
			return nil, err
		}
		for _, k := range s.sidecarNeededKeys(ctx, u.home) {
			k.Present = strings.TrimSpace(se.values[k.Scope][k.Name]) != ""
			add(u.home, "", k)
		}
	}
	return out, nil
}
