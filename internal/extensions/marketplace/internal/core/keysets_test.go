// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/keysfile"
)

const teamPassword = "the team password"

// keySetsWorld is a scoped katyoti root (never the development universe)
// whose marketplace offers the solution duo: two universes (team, the main
// one with the config-set base, and team-review beside it) and two key sets,
// the vaults alpha (KEY_A) and beta (KEY_B, KEY_GLOBAL), sealed with the
// passwords given. base asks KEY_A, KEY_B, KEY_C, KEY_SET (already set),
// KEY_GLOBAL (global) and KEY_OPT (optional).
type keySetsWorld struct {
	s          *service
	home, peer string
}

func newKeySetsWorld(t *testing.T, alphaPassword, betaPassword string) keySetsWorld {
	t.Helper()
	vault := func(name string) string {
		return "  - name: " + name + "\n    kind: vault\n    versions:\n      - version: \"1\"\n        assets:\n          - url: " + name + ".vault\n"
	}
	// The solution's versions, then the vault and config-set items (the
	// fixture's catalog lists the solution last).
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
          - {name: cards, version: "1.0"}
        configSets: [{name: base, version: "1"}]
        vaults: [{name: alpha, version: "1"}, {name: beta, version: "1"}]
        universes:
          - {name: studio, main: true, sidecars: [{name: notes}]}
          - {name: review, sidecars: [{name: cards}]}
`+vault("alpha")+vault("beta")+`  - name: base
    kind: config
    versions:
      - version: "1"
        assets:
          - url: base.tar.gz
`)
	seal := func(name, password string, keys ...keysfile.Entry) {
		file, err := keysfile.Seal(keys, password)
		if err != nil {
			t.Fatal(err)
		}
		p, err := store.VaultPath(name, "1")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, file, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seal("alpha", alphaPassword, keysfile.Entry{Name: "KEY_A", Scope: configset.ScopeUniverse, Value: "a-value"})
	seal("beta", betaPassword,
		keysfile.Entry{Name: "KEY_B", Scope: configset.ScopeUniverse, Value: "b-value"},
		keysfile.Entry{Name: "KEY_GLOBAL", Scope: configset.ScopeGlobal, Value: "g-value"})
	folder, err := store.ConfigSetFolderPath("base", "1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(folder, configset.TemplateDir), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `keys:
  - {name: KEY_A, secret: true}
  - {name: KEY_B, secret: true}
  - {name: KEY_C, description: the one nobody provides}
  - {name: KEY_SET}
  - {name: KEY_GLOBAL, scope: global}
  - {name: KEY_OPT, optional: true}
`
	if err := os.WriteFile(filepath.Join(folder, configset.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	w := keySetsWorld{s: s, home: filepath.Join(t.TempDir(), "team")}
	w.peer = w.home + "-review"
	for _, h := range []string{w.home, w.peer} {
		if err := os.MkdirAll(h, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := configset.WriteEnv(w.envPath(w.home), map[string]string{"KEY_SET": "already"}); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w keySetsWorld) envPath(home string) string {
	return filepath.Join(application.EnvOf(home).HomeDir(), ".env")
}

func (w keySetsWorld) env(t *testing.T, home string) map[string]string {
	t.Helper()
	path := w.envPath(home)
	if home == "" {
		path = filepath.Join(application.Env().GlobalHomeDir(), ".env")
	}
	m, err := configset.ReadEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func remainingNames(keys []mktapi.RemainingKey) string {
	var out []string
	for _, k := range keys {
		out = append(out, k.Name)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

var duo = mktapi.ItemRef{Name: "duo"}

// A solution with two key sets: both are listed as needing the password,
// one password opens them both, and importing with it writes every key of
// both sets into each universe of the solution (a global key into the
// global .env) — the keys left are those no set provides.
func TestOnePasswordImportsEveryKeySetOfASolution(t *testing.T) {
	w := newKeySetsWorld(t, teamPassword, teamPassword)
	ctx := context.Background()

	sets, err := w.s.KeySets(ctx, mktapi.KeySetsInput{Solution: duo, UniverseHome: w.home})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets.KeySets) != 2 || !sets.PasswordNeeded || sets.SharedPassword {
		t.Fatalf("key sets %+v (password needed %v, shared %v): want alpha and beta, the password needed", sets.KeySets, sets.PasswordNeeded, sets.SharedPassword)
	}
	if got := strings.Join(sets.Universes, "|"); got != w.home+"|"+w.peer {
		t.Errorf("universes %q, want the main one then its peer", got)
	}
	if got := remainingNames(sets.Remaining); got != "KEY_A,KEY_B,KEY_C,KEY_GLOBAL" {
		t.Errorf("before the password, remaining %s; want every required key without a value", got)
	}

	tried, err := w.s.KeySets(ctx, mktapi.KeySetsInput{Solution: duo, UniverseHome: w.home, Password: teamPassword})
	if err != nil {
		t.Fatal(err)
	}
	if !tried.SharedPassword || !tried.KeySets[0].Opened || !tried.KeySets[1].Opened {
		t.Fatalf("the team password: shared %v, sets %+v; want both opened", tried.SharedPassword, tried.KeySets)
	}
	if got := remainingNames(tried.Remaining); got != "KEY_C" {
		t.Errorf("with the password, remaining %s; want KEY_C alone", got)
	}
	if v := w.env(t, w.home)["KEY_A"]; v != "" {
		t.Fatalf("trying the password wrote KEY_A=%q", v)
	}

	res, err := w.s.ImportKeySets(ctx, mktapi.ImportKeySetsInput{Solution: duo, UniverseHome: w.home, Password: teamPassword})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{w.home, w.peer} {
		env := w.env(t, h)
		if env["KEY_A"] != "a-value" || env["KEY_B"] != "b-value" {
			t.Errorf("%s .env %v: want KEY_A and KEY_B from both sets", h, env)
		}
	}
	if g := w.env(t, "")["KEY_GLOBAL"]; g != "g-value" {
		t.Errorf("global KEY_GLOBAL=%q, want g-value", g)
	}
	if got := remainingNames(res.Remaining); got != "KEY_C" || res.Remaining[0].ConfigSet != "base@1" || res.Remaining[0].UniverseHome != w.home {
		t.Errorf("after import, remaining %+v; want KEY_C, from base@1, in the main universe", res.Remaining)
	}
}

// A password that does not open EVERY key set imports nothing: not even the
// keys of the set it does open.
func TestAWrongPasswordImportsNothing(t *testing.T) {
	for _, tc := range []struct{ name, alpha, beta string }{
		{"wrong for both", "another password", "another password"},
		{"wrong for one", teamPassword, "another password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newKeySetsWorld(t, tc.alpha, tc.beta)
			_, err := w.s.ImportKeySets(context.Background(), mktapi.ImportKeySetsInput{Solution: duo, UniverseHome: w.home, Password: teamPassword})
			if err == nil || !strings.Contains(err.Error(), "nothing was imported") || !strings.Contains(err.Error(), "beta") {
				t.Fatalf("err = %v, want the refusal naming beta", err)
			}
			for _, h := range []string{w.home, w.peer} {
				env := w.env(t, h)
				if env["KEY_A"] != "" || env["KEY_B"] != "" {
					t.Errorf("%s .env %v: a refused import wrote keys", h, env)
				}
			}
			if g := w.env(t, "")["KEY_GLOBAL"]; g != "" {
				t.Errorf("a refused import wrote KEY_GLOBAL=%q", g)
			}
		})
	}
}

// The keys left for the form are exactly the required keys the universe's
// config check reports missing: the config-set's report (ApplyConfigSet's
// dry run) — before and after the key sets are imported.
func TestTheRemainingKeysMatchTheUniversesConfigCheck(t *testing.T) {
	w := newKeySetsWorld(t, teamPassword, teamPassword)
	ctx := context.Background()
	check := func() string {
		dry, err := w.s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{Name: "base", UniverseHome: w.home, DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, k := range dry.Keys {
			if !k.Present && !k.Optional {
				names = append(names, k.Name)
			}
		}
		sort.Strings(names)
		return strings.Join(names, ",")
	}
	sets, err := w.s.KeySets(ctx, mktapi.KeySetsInput{Solution: duo, UniverseHome: w.home})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := remainingNames(sets.Remaining), check(); got != want {
		t.Errorf("before import: remaining %s, the config check %s", got, want)
	}
	res, err := w.s.ImportKeySets(ctx, mktapi.ImportKeySetsInput{Solution: duo, UniverseHome: w.home, Password: teamPassword})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := remainingNames(res.Remaining), check(); got != want || got != "KEY_C" {
		t.Errorf("after import: remaining %s, the config check %s; want KEY_C", got, want)
	}
}

// Installing a solution opens every vault with the one team password.
func TestInstallOpensEveryVaultWithTheTeamPassword(t *testing.T) {
	w := newKeySetsWorld(t, teamPassword, teamPassword)
	out, err := w.s.InstallSolution(context.Background(), mktapi.InstallSolutionInput{
		Ref: duo, UniverseHome: w.home, KeySetsPassword: teamPassword, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Vaults) != 2 || !out.Vaults[0].Opened || !out.Vaults[1].Opened {
		t.Fatalf("vaults %+v: want alpha and beta opened by the one password", out.Vaults)
	}
}

// keyX is the key the fixture sidecar requires: a name nothing else sets.
const keyX = "TEST_RQ003_KEY_X"

// A universe without a config-set, wired to a sidecar whose check requires
// KEY_X: its remaining key (ConfigSet "") is saved through ApplyConfigSet
// with an empty name — the universe's own key set, the keys its members
// declare — into the universe's .env; nothing then remains, the sidecar's
// check reports the key set, and a key nobody declared is never written
func TestTheUniverseOwnKeysAreSavedThroughApplyConfigSet(t *testing.T) {
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
        universe:
          sidecars: [{name: notes}]
`)
	s.dialHub = func(context.Context, string) (universeHub, error) { return nil, nil } // no universe runs
	t.Setenv(keyX, "")
	folder, err := store.ContentFolderPath("notes", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	decl := "name: notes\nconfig:\n  apiKeys:\n    - envName: " + keyX + "\n      required: true\n"
	if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte(decl), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.InstallSolution(ctx, mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "solo"}); err != nil {
		t.Fatal(err)
	}
	home := registered(t)["solo"]
	if home == "" {
		t.Fatal("solo is not registered")
	}

	sets, err := s.KeySets(ctx, mktapi.KeySetsInput{UniverseHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets.Remaining) != 1 || sets.Remaining[0].Name != keyX || sets.Remaining[0].ConfigSet != "" || sets.Remaining[0].UniverseHome != home {
		t.Fatalf("remaining %+v; want %s alone, of no config-set, in %s", sets.Remaining, keyX, home)
	}

	envPath := filepath.Join(application.EnvOf(home).HomeDir(), ".env")
	undeclared, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{UniverseHome: home, Values: map[string]string{"UNDECLARED": "v"}})
	if err != nil {
		t.Fatalf("the universe's own key set: %v", err)
	}
	if len(undeclared.KeysWritten) != 0 {
		t.Errorf("an undeclared key was written: %v", undeclared.KeysWritten)
	}
	if env, err := configset.ReadEnv(envPath); err != nil || env["UNDECLARED"] != "" {
		t.Errorf("the universe .env holds %v (%v); want no UNDECLARED", env, err)
	}

	// A value holding a line break is refused before anything is written:
	// `x\nKTW_NATS_URL=...` would add a line of its own.
	beforeApply, _ := os.ReadFile(envPath)
	if _, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{UniverseHome: home, Values: map[string]string{keyX: "x\nKTW_NATS_URL=nats://evil.invalid:4222"}}); err == nil || !strings.Contains(err.Error(), "line break") {
		t.Errorf("a value with a line break: err = %v, want refused", err)
	}
	if after, _ := os.ReadFile(envPath); string(after) != string(beforeApply) {
		t.Errorf("the universe .env changed to %q", after)
	}

	out, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{UniverseHome: home, Values: map[string]string{keyX: "x-value"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.KeysWritten, ",") != keyX {
		t.Errorf("keys written %v, want %s", out.KeysWritten, keyX)
	}
	if env, err := configset.ReadEnv(envPath); err != nil || env[keyX] != "x-value" {
		t.Errorf("the universe .env holds %v (%v); want %s", env, err, keyX)
	}

	sets, err = s.KeySets(ctx, mktapi.KeySetsInput{UniverseHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets.Remaining) != 0 {
		t.Errorf("remaining after the save: %+v, want none", sets.Remaining)
	}
	rep, err := s.checkInstalled(ctx, home, mktapi.Installed{Name: "notes", Kind: "content", FolderPath: folder})
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, it := range rep.Items {
		ok = ok || (it.Category == "env" && it.Name == keyX && it.Status == "ok")
	}
	if !ok {
		t.Errorf("the sidecar's check does not report %s set: %+v", keyX, rep.Items)
	}
}

// A sidecar reporting a key a key set may not import (a KTW_* variable, a name
// outside the grammar) has it neither offered nor written: the universe .env
// never receives KTW_NATS_URL because a sidecar's check named it
func TestASidecarReportedReservedKeyIsNeverOffered(t *testing.T) {
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
        universe:
          sidecars: [{name: notes}]
`)
	s.dialHub = func(context.Context, string) (universeHub, error) { return nil, nil }
	const reserved = "KTW_TEST_RQ1155_KEY"
	t.Setenv(reserved, "")
	folder, err := store.ContentFolderPath("notes", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	decl := "name: notes\nconfig:\n  apiKeys:\n    - envName: " + reserved + "\n      required: true\n"
	if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte(decl), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.InstallSolution(ctx, mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "solo"}); err != nil {
		t.Fatal(err)
	}
	home := registered(t)["solo"]
	sets, err := s.KeySets(ctx, mktapi.KeySetsInput{UniverseHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(sets.Remaining) != 0 {
		t.Errorf("remaining %+v; a reserved name is never offered", sets.Remaining)
	}
	out, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{UniverseHome: home, Values: map[string]string{reserved: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.KeysWritten) != 0 {
		t.Errorf("keys written %v; a reserved name is never written", out.KeysWritten)
	}
	envPath := filepath.Join(application.EnvOf(home).HomeDir(), ".env")
	if env, _ := configset.ReadEnv(envPath); env[reserved] != "" {
		t.Errorf("the universe .env holds %s", reserved)
	}
}
