// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/sealed"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/keysfile"
)

const keysPassword = "correct horse battery"

// keysWorld is an isolated machine: a global root and one universe folder, a
// keys.yaml manifest, and helpers to read the two .env files.
type keysWorld struct {
	home, manifest string
}

func newKeysWorld(t *testing.T, manifest string) keysWorld {
	t.Helper()
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	w := keysWorld{home: filepath.Join(t.TempDir(), "uni")}
	if err := os.MkdirAll(w.home, 0o755); err != nil {
		t.Fatal(err)
	}
	w.manifest = filepath.Join(t.TempDir(), configset.ManifestFile)
	if err := os.WriteFile(w.manifest, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w keysWorld) envPath(scope string) string {
	if scope == configset.ScopeGlobal {
		return filepath.Join(application.Env().GlobalHomeDir(), ".env")
	}
	return filepath.Join(application.EnvOf(w.home).HomeDir(), ".env")
}

func (w keysWorld) set(t *testing.T, scope string, kv map[string]string) {
	t.Helper()
	if _, err := configset.WriteEnv(w.envPath(scope), kv); err != nil {
		t.Fatal(err)
	}
}

func (w keysWorld) get(t *testing.T, scope string) map[string]string {
	t.Helper()
	m, err := configset.ReadEnv(w.envPath(scope))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

const twoKeys = `keys:
  - name: API_TOKEN
    description: the token
    secret: true
  - name: SHARED_URL
    scope: global
`

// Only the keys the manifest declares leave — a key added by hand to the same
// .env, and the process environment, never do.
func TestExportSealsOnlyTheDeclaredKeys(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	w.set(t, configset.ScopeUniverse, map[string]string{"API_TOKEN": "tok-${NOT_EXPANDED}", "PRIVATE_KEY": "mine"})
	w.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://x.example"})
	t.Setenv("PRIVATE_KEY", "from-process")
	s := &service{}

	out, err := s.ExportKeys(context.Background(), mktapi.ExportKeysInput{UniverseHome: w.home, Manifest: w.manifest, Password: keysPassword})
	if err != nil {
		t.Fatal(err)
	}
	b, err := openKeys(out.File, keysPassword)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bundleEntry{}
	for _, e := range b.Keys {
		got[e.Name] = e
	}
	if len(got) != 2 || got["API_TOKEN"].Value != "tok-${NOT_EXPANDED}" || got["SHARED_URL"].Scope != configset.ScopeGlobal {
		t.Fatalf("sealed %+v, want API_TOKEN (verbatim) and SHARED_URL (global) only", b.Keys)
	}
	if strings.Contains(string(out.File), "tok-") {
		t.Fatal("the file holds a value in clear")
	}
}

// A declared key without a value in its scope refuses the export, unless it is
// left out on purpose; the process environment does not stand in for it.
func TestExportRefusesAMissingKey(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	w.set(t, configset.ScopeUniverse, map[string]string{"API_TOKEN": "tok"})
	t.Setenv("SHARED_URL", "from-process")
	s := &service{}
	in := mktapi.ExportKeysInput{UniverseHome: w.home, Manifest: w.manifest, Password: keysPassword}

	if _, err := s.ExportKeys(context.Background(), in); err == nil || !strings.Contains(err.Error(), "SHARED_URL") {
		t.Fatalf("export with SHARED_URL unset = %v, want a refusal naming it", err)
	}
	in.SkipMissing = true
	out, err := s.ExportKeys(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Keys) != 1 || len(out.Skipped) != 1 || out.Skipped[0] != "SHARED_URL" {
		t.Fatalf("skip-missing = %+v", out)
	}
}

// An export says which keys — never a whole .env.
func TestExportNeedsAManifest(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	_, err := (&service{}).ExportKeys(context.Background(), mktapi.ExportKeysInput{UniverseHome: w.home, Password: keysPassword})
	if err == nil {
		t.Fatal("an export with no manifest succeeded")
	}
}

// The import writes each key into its scope; a key set to another value is
// kept unless overwrite; a dry run writes nothing; a wrong password opens
// nothing.
func TestImportWritesEachKeyIntoItsScope(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	w.set(t, configset.ScopeUniverse, map[string]string{"API_TOKEN": "tok"})
	w.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://x.example"})
	s := &service{}
	exp, err := s.ExportKeys(context.Background(), mktapi.ExportKeysInput{UniverseHome: w.home, Manifest: w.manifest, Password: keysPassword})
	if err != nil {
		t.Fatal(err)
	}

	// A second machine: empty .env files, except a different API_TOKEN.
	w2 := newKeysWorld(t, twoKeys)
	w2.set(t, configset.ScopeUniverse, map[string]string{"API_TOKEN": "other"})
	in := mktapi.ImportKeysInput{UniverseHome: w2.home, File: exp.File, Password: "wrong password!"}
	if _, err := s.ImportKeys(context.Background(), in); !errors.Is(err, sealed.ErrWrongPassword) {
		t.Fatalf("wrong password = %v", err)
	}

	in.Password, in.DryRun = keysPassword, true
	dry, err := s.ImportKeys(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Written) != 0 || w2.get(t, configset.ScopeGlobal)["SHARED_URL"] != "" {
		t.Fatalf("a dry run wrote: %+v", dry)
	}

	in.DryRun = false
	out, err := s.ImportKeys(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if w2.get(t, configset.ScopeGlobal)["SHARED_URL"] != "https://x.example" || w2.get(t, configset.ScopeUniverse)["SHARED_URL"] != "" {
		t.Fatal("SHARED_URL did not land in the global .env only")
	}
	if w2.get(t, configset.ScopeUniverse)["API_TOKEN"] != "other" {
		t.Fatal("a key set to another value was replaced without overwrite")
	}
	states := map[string]string{}
	for _, k := range out.Keys {
		states[k.Name] = k.State
	}
	if states["API_TOKEN"] != "differs" || states["SHARED_URL"] != "written" {
		t.Fatalf("states = %v", states)
	}

	in.Overwrite = true
	if _, err := s.ImportKeys(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if w2.get(t, configset.ScopeUniverse)["API_TOKEN"] != "tok" {
		t.Fatal("overwrite did not replace API_TOKEN")
	}
}

// A key set's global key reaches every member: the global .env fills a
// process only with the keys it declares (cenv.GlobalKeysVar), so the import
// declares each global key it carries - also one already in the file from an
// earlier import, undeclared - and a member's environment (cenv.LoadDotenv,
// what a sidecar's config check reads) then holds it.
func TestImportedGlobalKeyReachesAMember(t *testing.T) {
	w := newKeysWorld(t, twoKeys)
	w.set(t, configset.ScopeUniverse, map[string]string{"API_TOKEN": "tok"})
	w.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://x.example"})
	s := &service{}
	exp, err := s.ExportKeys(context.Background(), mktapi.ExportKeysInput{UniverseHome: w.home, Manifest: w.manifest, Password: keysPassword})
	if err != nil {
		t.Fatal(err)
	}

	// member starts a member's process on the installation: its environment
	// from the universe's .env and the global one. A process reads each file
	// once (cenv dedups the paths it walked), so the global root's files are
	// seen from a fresh folder, as a new process sees them.
	member := func(t *testing.T, home string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(application.Env().GlobalHomeDir(), ".env"))
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".env"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("KTW_KATYOTI_ROOT", root)
		for _, k := range []string{"SHARED_URL", "API_TOKEN", cenv.GlobalKeysVar} {
			t.Setenv(k, "")
			_ = os.Unsetenv(k)
		}
		cenv.LoadDotenv("katyoti", home)
		return os.Getenv("SHARED_URL")
	}

	// A fresh machine: the import writes the global key and declares it.
	w2 := newKeysWorld(t, twoKeys)
	if _, err := s.ImportKeys(context.Background(), mktapi.ImportKeysInput{UniverseHome: w2.home, File: exp.File, Password: keysPassword}); err != nil {
		t.Fatal(err)
	}
	if got := member(t, w2.home); got != "https://x.example" {
		t.Fatalf("a member of the universe reads SHARED_URL=%q, want the imported global key", got)
	}
	if got := member(t, filepath.Join(t.TempDir(), "other")); got != "https://x.example" {
		t.Fatalf("a member of another universe reads SHARED_URL=%q, want the global key", got)
	}

	// A machine where an earlier import left the key undeclared: importing
	// again declares it, the value untouched.
	w3 := newKeysWorld(t, twoKeys)
	w3.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://x.example", cenv.GlobalKeysVar: "OPENAI_*"})
	if got := member(t, w3.home); got != "" {
		t.Fatalf("an undeclared global key filled a member: %q", got)
	}
	if _, err := s.ImportKeys(context.Background(), mktapi.ImportKeysInput{UniverseHome: w3.home, File: exp.File, Password: keysPassword}); err != nil {
		t.Fatal(err)
	}
	if got := w3.get(t, configset.ScopeGlobal)[cenv.GlobalKeysVar]; got != "OPENAI_*,SHARED_URL" {
		t.Fatalf("declaration = %q, want the earlier entry kept and SHARED_URL added", got)
	}
	if got := member(t, w3.home); got != "https://x.example" {
		t.Fatalf("after the import again a member reads SHARED_URL=%q", got)
	}

	// A machine holding another value, written undeclared on purpose (another
	// account's): the import keeps it without --overwrite and does not declare
	// it, so no member reads a value the key set did not bring.
	w4 := newKeysWorld(t, twoKeys)
	w4.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://other.example", cenv.GlobalKeysVar: "OPENAI_*"})
	out, err := s.ImportKeys(context.Background(), mktapi.ImportKeysInput{UniverseHome: w4.home, File: exp.File, Password: keysPassword})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range out.Keys {
		if k.Name == "SHARED_URL" && k.State != "differs" {
			t.Fatalf("SHARED_URL state = %q, want differs", k.State)
		}
	}
	if g := w4.get(t, configset.ScopeGlobal); g["SHARED_URL"] != "https://other.example" || g[cenv.GlobalKeysVar] != "OPENAI_*" {
		t.Fatalf("a differing global key was replaced or declared: %v", g)
	}
	if got := member(t, w4.home); got != "" {
		t.Fatalf("a member reads the kept other value SHARED_URL=%q", got)
	}
}

// A keys file whose key name is outside the env-name grammar does not open:
// imported as a global key, `*` would declare every key of the global .env
// . Nor does one naming a KaTyoti variable: KTW_GLOBAL_KEYS
// with the value * declares every global key, KTW_NATS_URL moves the hub -
// in either scope.
func TestImportRefusesAKeyNameOutsideTheGrammar(t *testing.T) {
	for _, name := range []string{"*", "X_*", "A,B", "KTW_GLOBAL_KEYS", "KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_NATS_URL"} {
		value := "x"
		if name == "KTW_GLOBAL_KEYS" {
			value = "*"
		}
		file, err := keysfile.Seal([]keysfile.Entry{
			{Name: name, Scope: configset.ScopeGlobal, Value: value},
			{Name: name, Scope: configset.ScopeUniverse, Value: value},
		}, keysPassword)
		if err != nil {
			t.Fatal(err)
		}
		w := newKeysWorld(t, twoKeys)
		w.set(t, configset.ScopeGlobal, map[string]string{"LEANIX_API_TOKEN": "other-account"})
		// Refused when the file opens, before any write is planned.
		if _, err := (&service{}).ImportKeys(context.Background(), mktapi.ImportKeysInput{UniverseHome: w.home, File: file, Password: keysPassword}); err == nil || !strings.Contains(err.Error(), "may not import") {
			t.Errorf("a keys file with key %q: err = %v, want refused on open", name, err)
		}
		if g := w.get(t, configset.ScopeGlobal); g[cenv.GlobalKeysVar] != "" || g[name] != "" {
			t.Errorf("%q reached the global .env: %v", name, g)
		}
		if u := w.get(t, configset.ScopeUniverse); u[name] != "" {
			t.Errorf("%q reached the universe .env: %v", name, u)
		}
	}
}

// A keys file whose value holds a line break does not open: written, the value
// `x\nKTW_GLOBAL_KEYS=*` would end its line and declare every global key, and
// `x\nKTW_NATS_URL=...` would move the hub. Neither.env
// changes.
func TestImportRefusesAValueHoldingALineBreak(t *testing.T) {
	for _, value := range []string{"x\nKTW_GLOBAL_KEYS=*", "x\rKTW_NATS_URL=nats://evil.invalid:4222", "x\r\nKTW_NATS_URL=nats://evil.invalid:4222"} {
		file, err := keysfile.Seal([]keysfile.Entry{
			{Name: "LEANIX_API_TOKEN", Scope: configset.ScopeGlobal, Value: value},
			{Name: "LEANIX_API_TOKEN", Scope: configset.ScopeUniverse, Value: value},
		}, keysPassword)
		if err != nil {
			t.Fatal(err)
		}
		w := newKeysWorld(t, twoKeys)
		w.set(t, configset.ScopeGlobal, map[string]string{"SHARED_URL": "https://x.example"})
		before := w.get(t, configset.ScopeGlobal)
		if _, err := (&service{}).ImportKeys(context.Background(), mktapi.ImportKeysInput{UniverseHome: w.home, File: file, Password: keysPassword}); err == nil || !strings.Contains(err.Error(), "line break") {
			t.Errorf("value %q: err = %v, want refused on open", value, err)
		}
		if g := w.get(t, configset.ScopeGlobal); len(g) != len(before) || g[cenv.GlobalKeysVar] != "" || g["KTW_NATS_URL"] != "" {
			t.Errorf("value %q: the global .env changed: %v", value, g)
		}
		if u := w.get(t, configset.ScopeUniverse); len(u) != 0 {
			t.Errorf("value %q: the universe .env changed: %v", value, u)
		}
	}
}
