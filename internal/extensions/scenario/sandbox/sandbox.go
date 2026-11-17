// SPDX-License-Identifier: Apache-2.0

// Package sandbox closes a scenario case into its own world. Everything a case
// installs, runs, opens and stops lives under its runtime folder <root>/<case>:
// its universe, project and data, and — under machine/ — the machine-level
// state a katyoti installation keeps (the universe registry, the admin
// universe, the control plane, the caches). Nothing is shared with the
// installation of the person running the case: no registry, cache, hub or GUI
// of theirs is read, written or stopped.
//
// A process enters a case's sandbox by its environment: the global root and
// the cache root moved into the case (KTW_KATYOTI_ROOT, KTW_CACHE). Data is not
// moved process-wide: KTW_DATA set in an environment wins over every
// universe's own data_dir, so the case universe and the sandbox's admin would
// share one data folder; each sandbox universe carries its data_dir in its
// Settings.yaml instead. `ktw-compose scenario <verb> <case>` runs itself again
// inside the case's sandbox before it bootstraps (Enter), so every verb — and
// every process it starts — works there, and nobody has to set anything.
package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// The environment of a process inside a case's sandbox.
const (
	// Marker names the case runtime folder whose sandbox the process runs in.
	Marker = cenv.ScenarioSandboxEnv
	// HostRoot, HostData and HostCache are the machine folders of the
	// installation the command was run from — what the leak check watches.
	HostRoot  = "KTW_SCENARIO_HOST_ROOT"
	HostData  = "KTW_SCENARIO_HOST_DATA"
	HostCache = "KTW_SCENARIO_HOST_CACHE"
)

// Sandbox is a case's closed world, by its runtime folder <root>/<case>.
type Sandbox struct{ Case string }

// Of is the sandbox of a case runtime folder.
func Of(caseDir string) Sandbox {
	if abs, err := filepath.Abs(caseDir); err == nil {
		caseDir = abs
	}
	return Sandbox{Case: filepath.Clean(caseDir)}
}

// Machine holds the sandbox's machine-level state.
func (s Sandbox) Machine() string { return filepath.Join(s.Case, "machine") }

// Root is the sandbox's global config root: its registries, its admin
// universe, its control plane.
func (s Sandbox) Root() string { return filepath.Join(s.Machine(), "root") }

// Cache is the sandbox's cache root.
func (s Sandbox) Cache() string { return filepath.Join(s.Machine(), "cache") }

// AdminData is the data folder of the sandbox's admin universe (set in its
// Settings.yaml).
func (s Sandbox) AdminData() string { return filepath.Join(s.Machine(), "data", "admin") }

// ToolData is the data folder of the scenario command's own process in the
// sandbox (its logs): never a universe's.
func (s Sandbox) ToolData() string { return filepath.Join(s.Machine(), "data", "tool") }

// ControlPlane is where the sandbox's control-plane binaries are found (the
// hub and GUI a GUI opened in the sandbox spawns).
func (s Sandbox) ControlPlane() string { return filepath.Join(s.Root(), "control-plane") }

// dropped are the variables a sandbox never inherits: the installation's
// locations (set again for the sandbox), a universe or workspace the caller
// was in, and provider keys — a case gets its keys only through its `copy:`
// entries, into its universe's .env.
var dropped = []string{"KTW_KATYOTI_ROOT", "KTW_DATA", "KTW_CACHE", "KTW_HOME", "KTW_WORKSPACE", "KTW_UNIVERSE",
	// An installation's file (cenv.InstallationEnv) would set every root back to it.
	"KTW_INSTALLATION", "KTW_INSTANCE", Marker, HostRoot, HostData, HostCache}

func droppedKey(k string) bool {
	return slices.Contains(dropped, strings.ToUpper(k)) || strings.HasSuffix(strings.ToUpper(k), "_API_KEY")
}

// Var is one environment variable; an empty Value clears it.
type Var struct{ Name, Value string }

// World is what puts any process in the sandbox — the scenario command, and
// a GUI opened on the case: the global root and the cache moved into the
// case, and nothing of a caller's data, workspace or universe. Data is never
// moved here: a KTW_DATA in a GUI's environment would win over every
// universe's own data_dir.
func (s Sandbox) World() []Var {
	return []Var{
		{"KTW_KATYOTI_ROOT", s.Root()},
		// A command or GUI of an installation (the dev channel's bin/dev) would
		// otherwise take the installation's roots back over the sandbox's.
		{cenv.NoInstallationEnv, "1"},
		{"KTW_CACHE", s.Cache()},
		{"KTW_DATA", ""},
		{"KTW_HOME", ""},
		{"KTW_WORKSPACE", ""},
		{"KTW_UNIVERSE", ""},
	}
}

// Env is base, the environment of a process outside the sandbox, turned into
// the scenario command's inside it; host is the installation the command was
// run from. The command's own data (its logs) is the sandbox's ToolData:
// every process it starts names its universe's data itself.
func (s Sandbox) Env(base []string, host application.MachineDirs) []string {
	out := make([]string, 0, len(base)+8)
	for _, kv := range base {
		if k, _, _ := strings.Cut(kv, "="); !droppedKey(k) {
			out = append(out, kv)
		}
	}
	for _, v := range s.World() {
		if v.Value != "" {
			out = append(out, v.Name+"="+v.Value)
		}
	}
	return append(out,
		"KTW_DATA="+s.ToolData(),
		Marker+"="+s.Case,
		HostRoot+"="+host.Root,
		HostData+"="+host.Data,
		HostCache+"="+host.Cache,
	)
}

// Current is the sandbox this process runs in; false outside any.
func Current() (Sandbox, bool) {
	if c := os.Getenv(Marker); c != "" {
		return Sandbox{Case: c}, true
	}
	return Sandbox{}, false
}

// Inside is whether this process runs in the sandbox of the case runtime
// folder caseDir.
func Inside(caseDir string) bool {
	cur, ok := Current()
	return ok && sameDir(cur.Case, Of(caseDir).Case)
}

// Host is the installation the command was run from (inside a sandbox, as the
// command that entered it recorded it).
func Host() application.MachineDirs {
	if _, ok := Current(); ok {
		return application.MachineDirs{Root: os.Getenv(HostRoot), Data: os.Getenv(HostData), Cache: os.Getenv(HostCache)}
	}
	return application.Machine()
}

// DefaultRoot is the folder cases run in when no --root is given:
// $KTW_TEST_ROOT, else ~/katyoti-tests.
func DefaultRoot() string {
	if r := os.Getenv("KTW_TEST_ROOT"); r != "" {
		return r
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "katyoti-tests")
}

// valueFlags are the scenario flags that take a value (`--from s2` as well as
// `--from=s2`); any other flag stands alone.
var valueFlags = []string{"--root", "--cases", "--bin", "--from", "--to", "--kind", "--channel", "--timeout-min"}

// noCase are the verbs that name no case: they run where they are
// (machine-stages runs on the clean machine, given a plan file).
var noCase = []string{"list", "new", "help", "completion", "machine-stages"}

// CaseDir is the runtime folder of the case a command line works on — the
// case of `scenario <verb> <case>` under its --root — and whether it names
// one. args are the command's arguments, without the program.
func CaseDir(args []string) (string, bool) {
	var pos []string
	root := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			return "", false
		}
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(a, "=")
		if !slices.Contains(valueFlags, name) {
			continue
		}
		if !hasVal && i+1 < len(args) {
			i++
			val = args[i]
		}
		if name == "--root" {
			root = val
		}
	}
	if len(pos) < 3 || pos[0] != "scenario" || slices.Contains(noCase, pos[1]) {
		return "", false
	}
	if root == "" {
		root = DefaultRoot()
	}
	return Of(filepath.Join(root, pos[2])).Case, true
}

// Enter runs the command again inside the sandbox of the case it works on,
// when it names one and does not run there yet; it answers the exit code and
// whether it did. Call it first in main, before anything resolves the
// installation's paths.
func Enter(args []string) (int, bool) {
	caseDir, ok := CaseDir(args[1:])
	if !ok || Inside(caseDir) {
		return 0, false
	}
	self, err := os.Executable()
	if err != nil {
		return 0, false
	}
	cmd := exec.Command(self, args[1:]...)
	cmd.Env = Of(caseDir).Env(os.Environ(), application.Machine())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), true
		}
		_, _ = os.Stderr.WriteString("scenario: cannot enter the case's sandbox: " + err.Error() + "\n")
		return 1, true
	}
	return 0, true
}

func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
