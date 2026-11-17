// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// A sandbox's environment moves the global root and the cache into the case,
// records the installation it came from, and keeps nothing that would lead
// back to it: no data, workspace or universe of the caller, no provider key.
func TestTheSandboxEnvironmentLeadsNowhereElse(t *testing.T) {
	s := Of(filepath.Join(t.TempDir(), "todo-bun"))
	host := application.MachineDirs{Root: "/home/me/.config/katyoti", Data: "/home/me/.local/share/katyoti", Cache: "/home/me/.cache/katyoti"}
	env := s.Env([]string{
		"PATH=/bin", "KTW_KATYOTI_ROOT=/prod", "KTW_DATA=/prod-data", "KTW_CACHE=/prod-cache", "KTW_HOME=/x",
		"KTW_WORKSPACE=/w", "KTW_UNIVERSE=dev", "ANTHROPIC_API_KEY=secret", "openai_api_key=secret",
		Marker + "=/elsewhere", "KTW_TEST_ROOT=/tests", "KTW_INSTALLATION=/prod/katyoti.yml", "KTW_INSTANCE=prod",
	}, host)
	got := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if _, dup := got[k]; dup {
			t.Errorf("%s set twice", k)
		}
		got[k] = v
	}
	want := map[string]string{
		"PATH": "/bin", "KTW_TEST_ROOT": "/tests",
		"KTW_KATYOTI_ROOT": s.Root(), "KTW_CACHE": s.Cache(), "KTW_DATA": s.ToolData(), Marker: s.Case,
		HostRoot: host.Root, HostData: host.Data, HostCache: host.Cache,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	for _, k := range []string{"KTW_HOME", "KTW_WORKSPACE", "KTW_UNIVERSE", "ANTHROPIC_API_KEY", "openai_api_key", "KTW_INSTALLATION", "KTW_INSTANCE"} {
		if v, ok := got[k]; ok {
			t.Errorf("%s=%q reached the sandbox", k, v)
		}
	}
	if !strings.HasPrefix(s.Root(), s.Case) || !strings.HasPrefix(s.Cache(), s.Case) || !strings.HasPrefix(s.AdminData(), s.Case) || !strings.HasPrefix(s.ToolData(), s.Case) || !strings.HasPrefix(s.ControlPlane(), s.Root()) {
		t.Errorf("a sandbox folder lies outside the case: %+v", s)
	}
}

// The case a command line works on is `scenario <verb> <case>` under its
// --root (else the default root); verbs without a case, and help, name none.
func TestTheCommandLineNamesItsCase(t *testing.T) {
	t.Setenv("KTW_TEST_ROOT", filepath.FromSlash("/tests"))
	root := filepath.FromSlash("/r")
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"root flag":              {[]string{"scenario", "run", "todo-bun", "--root", root, "--from", "s2"}, filepath.Join(root, "todo-bun")},
		"root with =":            {[]string{"scenario", "status", "--root=" + root, "todo-bun"}, filepath.Join(root, "todo-bun")},
		"flags first":            {[]string{"scenario", "--cases", "c", "run", "--from", "s2", "smoke"}, filepath.FromSlash("/tests/smoke")},
		"bool flag":              {[]string{"scenario", "install", "--force", "smoke", "--bin", "b"}, filepath.FromSlash("/tests/smoke")},
		"call with its json":     {[]string{"scenario", "call", "smoke", "ktw-agents:plans/List", "{}"}, filepath.FromSlash("/tests/smoke")},
		"machine, channel first": {[]string{"scenario", "machine", "--channel", "D:/m/infra", "--kind", "local", "smoke"}, filepath.FromSlash("/tests/smoke")},
		"machine-stages":         {[]string{"scenario", "machine-stages", "plan.json", "--results", "r"}, ""},
		"list":                   {[]string{"scenario", "list"}, ""},
		"new":                    {[]string{"scenario", "new", "x"}, ""},
		"help":                   {[]string{"scenario", "run", "smoke", "--help"}, ""},
		"no case":                {[]string{"scenario", "run"}, ""},
		"another command":        {[]string{"skills", "list", "x"}, ""},
	} {
		got, ok := CaseDir(tc.args)
		if want := tc.want; ok != (want != "") || (ok && got != Of(want).Case) {
			t.Errorf("%s: %q %v, want %q", name, got, ok, want)
		}
	}
}

// A command enters the sandbox of its case unless it already runs there: the
// re-run happens once, never again from inside.
func TestACommandRunsInItsCaseSandboxOnce(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "smoke")
	t.Setenv(Marker, "")
	if Inside(caseDir) {
		t.Fatal("outside any sandbox, Inside says yes")
	}
	t.Setenv(Marker, Of(caseDir).Case)
	if !Inside(caseDir) || !Inside(filepath.Join(root, ".", "smoke")) {
		t.Fatal("inside the case's sandbox, Inside says no")
	}
	if Inside(filepath.Join(root, "other")) {
		t.Fatal("inside one case's sandbox, Inside says yes for another")
	}
	if code, entered := Enter([]string{"ktw-compose", "scenario", "status", "smoke", "--root", root}); entered || code != 0 {
		t.Fatalf("inside its sandbox, the command ran again (%d)", code)
	}
	if host := Host(); host.Root != "" {
		t.Errorf("inside a sandbox without recorded host, Host = %+v", host)
	}
}

// A GUI opened on a case enters the same world without the command's data:
// its global root and cache are the case's, and it keeps no data, workspace
// or universe of its caller (a KTW_DATA would override every universe's).
func TestAGUIEntersTheWorldWithoutData(t *testing.T) {
	s := Of(filepath.Join(t.TempDir(), "todo-bun"))
	got := map[string]string{}
	for _, v := range s.World() {
		got[v.Name] = v.Value
	}
	if got["KTW_KATYOTI_ROOT"] != s.Root() || got["KTW_CACHE"] != s.Cache() {
		t.Errorf("world = %v", got)
	}
	// A GUI or command of an installation stays out of it in the sandbox.
	if got[cenv.NoInstallationEnv] == "" {
		t.Errorf("world = %v: no %s", got, cenv.NoInstallationEnv)
	}
	for _, k := range []string{"KTW_DATA", "KTW_HOME", "KTW_WORKSPACE", "KTW_UNIVERSE"} {
		if v, ok := got[k]; !ok || v != "" {
			t.Errorf("%s: %q, %v — want it cleared", k, v, ok)
		}
	}
}
