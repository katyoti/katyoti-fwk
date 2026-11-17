// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/variables"
)

// An installation is a folder whose katyoti.yml says where its roots are and
// how it starts: every process of the installation reads it first, so two
// installations — two versions, two solutions — run side by side without one
// touching the other, and nobody sets an environment variable to choose one.
//
//	<installation>/
//	  katyoti.yml     name, solution, roots, settings, env
//	  .env            the installation's own variables (read by ${{env:KEY}})
//	  root/           the config root: registries, universes, control plane, keys
//	  data/           the data root: every universe's runtime data
//	  cache/          the cache root
//
// A process finds its installation by walking up from its own executable (the
// control plane is in root/control-plane/, the sidecars in root/bin/store/…),
// else from KTW_INSTALLATION (set only by katyoti, for the processes it
// starts): an executable of one installation never adopts another's. Found, it
// sets the roots for itself and every process it starts, over any inherited
// value, and takes nothing that says where it works from outside: a location
// variable (installationInputs) is kept only when the process that started it
// is of the same installation (it inherited this KTW_INSTALLATION), else it is
// dropped and reported (RefusedInputs). Not found (a development build, a
// test), nothing changes.

// InstallationFile is the name of an installation's file.
const InstallationFile = "katyoti.yml"

// InstallationEnv names the installation file of the process that started
// this one. katyoti sets it for its children; a person never does.
const InstallationEnv = "KTW_INSTALLATION"

// NoInstallationEnv keeps a process out of every installation, its executable's
// included, when it is also given a root of its own (KTW_KATYOTI_ROOT): the
// sandbox of a scenario case sets both for the command it runs again inside the
// case, whose executable may lie in an installation (the dev channel's bin/dev).
const NoInstallationEnv = "KTW_NO_INSTALLATION"

// ScenarioSandboxEnv names the runtime folder of the scenario case whose
// sandbox a process runs in: `ktw-compose scenario` sets it for the command it
// runs again inside the case, and every process that command starts inherits
// it (scenario/sandbox).
const ScenarioSandboxEnv = "KTW_SCENARIO_SANDBOX"

// ScenarioSandbox is the runtime folder of the scenario case this process runs
// in; false outside every case. A process inside a case changes nothing
// outside that folder.
func ScenarioSandbox() (string, bool) {
	c := strings.TrimSpace(os.Getenv(ScenarioSandboxEnv))
	if c == "" {
		return "", false
	}
	return absClean(c), true
}

// installationWalk is how many folders above its executable a process looks
// for katyoti.yml (root/bin/store/<item>/<version>/<os-arch>/ is six).
const installationWalk = 8

// Installation is the content of katyoti.yml.
type Installation struct {
	// Name names the installation (its shortcut, the GUI's title).
	Name string `yaml:"name"`
	// Solution is what it was installed from (`<solution>@<version>`), if any.
	Solution string `yaml:"solution,omitempty"`
	// Roots are relative to the file's folder (or absolute).
	Roots InstallationRoots `yaml:"roots"`
	// Settings are katyoti's start settings (launch mode, default universe…),
	// read and written by katyoti (`katyoti config set`).
	Settings map[string]any `yaml:"settings,omitempty"`
	// Env is set for every process of the installation, over inherited values;
	// a value may read a variable: ${{env:KEY}} (the .env beside the file).
	Env map[string]string `yaml:"env,omitempty"`
	// Locked keeps every version where it is: no updater moves the control
	// plane, a sidecar or a marketplace item by itself; a version changes only
	// when a person installs a release on it (`katyoti config set locked on|off`).
	Locked bool `yaml:"locked,omitempty"`

	path string
}

// InstallationRoots are an installation's three machine folders.
type InstallationRoots struct {
	Config string `yaml:"config"`
	Data   string `yaml:"data"`
	Cache  string `yaml:"cache"`
}

// DefaultInstallationRoots is the layout of a new installation.
var DefaultInstallationRoots = InstallationRoots{Config: "root", Data: "data", Cache: "cache"}

// Path is the installation's file; Dir its folder.
func (i *Installation) Path() string { return i.path }
func (i *Installation) Dir() string  { return filepath.Dir(i.path) }

// Is says whether o is the same installation (the same file).
func (i *Installation) Is(o *Installation) bool {
	return i != nil && o != nil && samePath(i.path, o.path)
}

// ConfigRoot, DataRoot and CacheRoot are the roots, absolute.
func (i *Installation) ConfigRoot() string {
	return i.abs(i.Roots.Config, DefaultInstallationRoots.Config)
}
func (i *Installation) DataRoot() string { return i.abs(i.Roots.Data, DefaultInstallationRoots.Data) }
func (i *Installation) CacheRoot() string {
	return i.abs(i.Roots.Cache, DefaultInstallationRoots.Cache)
}

func (i *Installation) abs(p, def string) string {
	if p == "" {
		p = def
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(i.Dir(), p)
}

// LoadInstallation reads an installation file (or the katyoti.yml of a folder).
// A bootstrap-layer read, like the universes registry: it runs before the
// config service exists, so it decodes the file itself.
func LoadInstallation(path string) (*Installation, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, InstallationFile)
	}
	path = absClean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var inst Installation
	if err := yaml.Unmarshal(data, &inst); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	inst.path = path
	if strings.TrimSpace(inst.Name) == "" {
		return nil, fmt.Errorf("%s: an installation has a name", path)
	}
	return &inst, nil
}

// installationHeader opens every katyoti.yml katyoti writes.
const installationHeader = `# This folder is a katyoti installation. Every katyoti program of it reads
# this file first: where its roots are (relative to this folder), how it
# starts, and the variables its processes get. katyoti rewrites it
# (katyoti config set ...); an edit by hand is kept, a comment is not.
#   name      the installation's name (its shortcut, the window title)
#   solution  what it was installed from (<solution>@<version>)
#   roots     config (registries, universes, control plane, keys), data, cache
#   settings  launch_mode (gui|cli|none), default_universe, autostart_hub, update_check (on|off), projects_folder
#   env       variables for every process; a value may read ${{env:KEY}} from .env beside this file
#   locked    true: no version moves by itself, only when a release is installed (katyoti config set locked on|off)
`

// SaveInstallation writes the installation's file.
func SaveInstallation(inst *Installation) error {
	if inst.path == "" {
		return fmt.Errorf("installation %q has no file", inst.Name)
	}
	var buf bytes.Buffer
	buf.WriteString(installationHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(inst); err != nil {
		return err
	}
	tmp := inst.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, inst.path)
}

// NewInstallation is an installation to be written in folder.
func NewInstallation(folder, name, solution string) *Installation {
	return &Installation{Name: name, Solution: solution, Roots: DefaultInstallationRoots,
		path: filepath.Join(absClean(folder), InstallationFile)}
}

// InstancesEnv names the registry of installations' file
// (controlplane.InstallationsRegistryPath reads it).
const InstancesEnv = "KTW_INSTANCES"

// TestInstancesRegistry is the registry of installations a test process and
// the programs it starts use when nothing names one: a file of its own in the
// temporary folder, never the person's.
func TestInstancesRegistry() string {
	return filepath.Join(os.TempDir(), "katyoti-test-instances", fmt.Sprintf("%d.yaml", os.Getpid()))
}

// A test takes no installation it inherits (installationPath), nor do the
// programs it starts (a hub from bin/): they run on the test's roots, not on
// those of the installation the test was run from. Nor do they record
// themselves in the person's registry of installations: KTW_INSTANCES names
// the test's own file unless the test named another than the person's
func init() {
	if testing.Testing() {
		_ = os.Unsetenv(InstallationEnv)
		if p := os.Getenv(InstancesEnv); p == "" || isPersonsInstancesRegistry(p) {
			_ = os.Setenv(InstancesEnv, TestInstancesRegistry())
		}
	}
}

// isPersonsInstancesRegistry is whether p is the person's registry of
// installations: <user config folder>/katyoti-instances.yaml.
func isPersonsInstancesRegistry(p string) bool {
	dir, err := os.UserConfigDir()
	return err == nil && strings.EqualFold(filepath.Clean(p), filepath.Clean(filepath.Join(dir, "katyoti-instances.yaml")))
}

var (
	installOnce sync.Once
	installed   *Installation
)

// CurrentInstallation is the installation this process belongs to (nil: none),
// its roots applied to the process on first use.
func CurrentInstallation() *Installation {
	installOnce.Do(func() {
		inherited := strings.TrimSpace(os.Getenv(InstallationEnv))
		path := installationPath(exeDir(), testing.Testing())
		if path == "" {
			return
		}
		inst, err := LoadInstallation(path)
		if err != nil {
			// Running on the default roots would act on another installation:
			// a broken file stops the program instead.
			fmt.Fprintf(os.Stderr, "katyoti: cannot read the installation file: %v\n", err)
			os.Exit(2)
		}
		applyInstallation(inst, inherited)
		installed = inst
	})
	return installed
}

// installationPath is the installation file of a process whose executable is
// in exeDir ("" none). A process given a root of its own and told to stay out
// of every installation (NoInstallationEnv) has none. A test never takes an
// inherited KTW_INSTALLATION: it takes roots of its own (t.TempDir,
// KTW_KATYOTI_ROOT), and run from a program of an installation (an agent's
// check, a session katyoti started) the inherited installation would set its
// roots back over them - the test would write the installation's registries.
func installationPath(exeDir string, inTest bool) string {
	if os.Getenv(NoInstallationEnv) != "" && os.Getenv("KTW_KATYOTI_ROOT") != "" {
		return ""
	}
	inherited := strings.TrimSpace(os.Getenv(InstallationEnv))
	if inTest {
		inherited = ""
	}
	return findInstallationFrom(exeDir, inherited)
}

// exeDir is the folder of this process's executable ("" unknown).
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

// InstallationAbove is the installation file a program at binary belongs to by
// its place - a katyoti.yml at most installationWalk folders above it - or ""
// when it lies in no installation (a checkout's bin/, a test's folder).
func InstallationAbove(binary string) string {
	if binary == "" {
		return ""
	}
	return walkUpFor(filepath.Dir(absClean(binary)), InstallationFile, installationWalk)
}

// RootInstallation is the installation whose config root is root (its
// katyoti.yml in root, adopted in place, or in its parent), nil when none.
func RootInstallation(root string) *Installation {
	if root == "" {
		return nil
	}
	root = absClean(root)
	for _, dir := range []string{root, filepath.Dir(root)} {
		p := filepath.Join(dir, InstallationFile)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if inst, err := LoadInstallation(p); err == nil && samePath(inst.ConfigRoot(), root) {
			return inst
		}
	}
	return nil
}

// Within says whether path lies in dir (or is dir), as the platform compares
// paths.
func Within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(envKeyName(absClean(dir)), envKeyName(absClean(path)))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// findInstallationFrom is the installation of an executable in exeDir that
// inherited KTW_INSTALLATION=inherited: its own first.
func findInstallationFrom(exeDir, inherited string) string {
	if exeDir != "" {
		if p := walkUpFor(exeDir, InstallationFile, installationWalk); p != "" {
			return p
		}
	}
	return strings.TrimSpace(inherited)
}

// walkUpFor is the first dir/name from dir upwards, at most levels folders up.
func walkUpFor(dir, name string, levels int) string {
	for n := 0; n <= levels; n++ {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// installationOwned are the variables an installation sets; inherited ones
// never win. KTW_DATA names one universe's data folder: inherited, it would
// send every universe's data elsewhere, so an installation drops it.
var installationDropped = []string{"KTW_DATA", "KTW_HOME", "KTW_INFRA_CHANNEL"}

// installationInputs say where a process works or which programs it runs:
// inherited from outside its installation, they would send it to another
// installation's universe, hub, data or binaries.
var installationInputs = []string{"KTW_WORKSPACE", "KTW_WORKSPACE_DIR", "KTW_UNIVERSE", "KTW_UNIVERSE_ID",
	"KTW_TENANT_ID", "KTW_NATS_URL", "KTW_CONTENT_ROOT", "KTW_HUB_BINARY", "KTW_DISPLAY_BIN", DotenvKeysVar}

// RefusedInput is an inherited variable an installation's process did not take.
type RefusedInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Why   string `json:"why"`
}

var refusedInputs []RefusedInput

// RefusedInputs are the inherited variables this process's installation
// dropped, with why: what `sc dirs` reports.
func RefusedInputs() []RefusedInput {
	CurrentInstallation()
	return refusedInputs
}

// samePath compares two file paths as the platform does.
func samePath(a, b string) bool {
	return envKeyName(filepath.Clean(a)) == envKeyName(filepath.Clean(b))
}

func applyInstallation(inst *Installation, inherited string) {
	// From a process of the same installation, the location variables are the
	// installation's own; from anywhere else they are not taken.
	if inherited == "" || !samePath(inherited, inst.Path()) {
		why := "not set by the installation " + inst.Name
		if inherited != "" {
			why = "set by another installation (" + inherited + ")"
		}
		for _, k := range installationInputs {
			if v, ok := os.LookupEnv(k); ok && v != "" {
				refusedInputs = append(refusedInputs, RefusedInput{Name: k, Value: v, Why: why})
				fmt.Fprintf(os.Stderr, "katyoti: %s=%s ignored: %s\n", k, v, why)
			}
			_ = os.Unsetenv(k)
		}
	}
	set := func(k, v string) {
		if old, ok := os.LookupEnv(k); ok && old != "" && filepath.Clean(old) != filepath.Clean(v) {
			fmt.Fprintf(os.Stderr, "katyoti: %s=%s ignored: the installation %s sets %s\n", k, old, inst.Name, v)
		}
		_ = os.Setenv(k, v)
	}
	set(InstallationEnv, inst.Path())
	set("KTW_KATYOTI_ROOT", inst.ConfigRoot())
	set("XDG_DATA_HOME", inst.DataRoot())
	set("KTW_CACHE", inst.CacheRoot())
	_ = os.Setenv("KTW_INSTANCE", inst.Name)
	for _, k := range installationDropped {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			refusedInputs = append(refusedInputs, RefusedInput{Name: k, Value: v, Why: "an installation sets its own"})
		}
		_ = os.Unsetenv(k)
	}
	// The installation's .env is a scope above every universe (in place, the
	// global .env itself): it fills only the keys declared global, its
	// declaration read first, as the global.env does.
	// inst.Env's ${{env:KEY}} still reads every key of it, from the file.
	instEnv := filepath.Join(inst.Dir(), ".env")
	_, _ = loadEnv(instEnv, map[string]bool{GlobalKeysVar: true})
	recordEnvCandidateOf(instEnv, isGlobalKey)
	file, _ := parseEnvFile(instEnv)
	for k, v := range inst.Env {
		_ = os.Setenv(k, variables.ExpandVars(expandFileEnv(v, file)))
	}
}

// expandFileEnv replaces in v each ${{env:KEY}} the process does not set by
// KEY's value in file (a .env's pairs), leaving the rest to ExpandVars.
func expandFileEnv(v string, file [][2]string) string {
	for _, key := range variables.Keys([]byte(v), "env") {
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		for _, e := range file {
			if e[0] == key {
				v = strings.ReplaceAll(v, "${{env:"+key+"}}", variables.ExpandVars(e[1]))
				break
			}
		}
	}
	return v
}

// AdoptRoot makes the config root at dir an installation in place (the dev
// channel): a katyoti.yml beside its registry and control
// plane whose roots are where they already are - config dir itself, data the
// platform default this process resolves, cache the cache folder it resolved
// (cacheDir) - so nothing moves and
// every program under dir (its control plane, its store) belongs to it. A
// root that already has one is refused: an installation is made once.
func AdoptRoot(dir, name, cacheDir string) (*Installation, error) {
	dir = absClean(dir)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a folder: only an existing root is adopted", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, InstallationFile)); err == nil {
		return nil, fmt.Errorf("%s already has a %s", dir, InstallationFile)
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("an installation has a name")
	}
	inst := &Installation{Name: name, Roots: InstallationRoots{
		Config: ".",
		Data:   filepath.ToSlash(xdgDataHome()),
		Cache:  filepath.ToSlash(cacheDir),
	}, path: filepath.Join(dir, InstallationFile)}
	if err := SaveInstallation(inst); err != nil {
		return nil, err
	}
	return inst, nil
}

// ForeignInstallation answers the installation a program at binary belongs to
// when this process is not of it ("" otherwise): such a program takes no
// location variable this process hands it (applyInstallation), so a universe
// passed to it is dropped. A spawner refuses rather than wait on it
func ForeignInstallation(binary string) *Installation {
	if binary == "" {
		return nil
	}
	p := findInstallationFrom(filepath.Dir(absClean(binary)), "")
	if p == "" {
		return nil
	}
	if mine := CurrentInstallation(); mine != nil && samePath(mine.Path(), p) {
		return nil
	}
	inst, err := LoadInstallation(p)
	if err != nil {
		return &Installation{Name: filepath.Base(filepath.Dir(p)), path: p}
	}
	return inst
}
