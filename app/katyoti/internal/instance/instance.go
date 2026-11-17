// SPDX-License-Identifier: Apache-2.0

// Package instance runs several katyoti installations on one machine — the
// default one, a test of the next version, a demo, a customer's solution —
// each with its own registry, universes, data, caches and control plane at its
// own version, named and switched between by name.
//
// An instance is an installation folder (cenv.Installation): its katyoti.yml
// says where its roots are (root/, data/, cache/ beside it by default) and how
// it starts. Every program of the installation reads that file first — found
// above its own executable, or named by KTW_INSTALLATION, which katyoti sets
// for the programs it starts — so two installations share no file a hub or a
// registry writes, and two versions run side by side without one reusing,
// updating or stopping the other's processes. Nobody sets a variable by hand.
//
// The registry of instances (name → folder) is in the user's config folder,
// outside every installation, so it reads the same from inside any of them;
// every installation that runs katyoti records itself there.
package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// Marker names the instance a process runs in (the GUI shows it).
const Marker = "KTW_INSTANCE"

// RegistryEnv overrides where the registry of instances is kept (tests).
const RegistryEnv = controlplane.InstallationsEnv

// Instance is one named installation.
type Instance struct {
	Name   string `yaml:"name"`
	Folder string `yaml:"folder"`
}

// File is the installation's katyoti.yml.
func (i Instance) File() string { return filepath.Join(i.Folder, cenv.InstallationFile) }

// installation is the instance's file read, or the default layout when it has
// none yet.
func (i Instance) installation() *cenv.Installation {
	if inst, err := cenv.LoadInstallation(i.File()); err == nil {
		return inst
	}
	return cenv.NewInstallation(i.Folder, i.Name, "")
}

// Title is the installation's own name (its katyoti.yml `name`, what its
// shortcut and window show); the instance's when its file has none.
func (i Instance) Title() string { return i.installation().Name }

// Root, Data and Cache are the instance's machine folders.
func (i Instance) Root() string  { return i.installation().ConfigRoot() }
func (i Instance) Data() string  { return i.installation().DataRoot() }
func (i Instance) Cache() string { return i.installation().CacheRoot() }

// EnsureFile writes the instance's katyoti.yml when it has none.
func (i Instance) EnsureFile() error {
	if _, err := os.Stat(i.File()); err == nil {
		return nil
	}
	return cenv.SaveInstallation(cenv.NewInstallation(i.Folder, i.Name, ""))
}

// ControlPlane is the instance's own katyoti, at the instance's version.
func (i Instance) ControlPlane(bin string) string {
	return filepath.Join(i.Root(), "control-plane", bin+exeSuffix())
}

// dropped are the variables a process of the instance never inherits: the
// locations of the installation the command runs from, a universe or
// workspace it was in, a channel override. The instance's file sets the rest.
var dropped = []string{"KTW_KATYOTI_ROOT", "KTW_DATA", "KTW_CACHE", "XDG_DATA_HOME", "KTW_HOME",
	"KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_INFRA_CHANNEL", Marker, cenv.InstallationEnv}

// Env is base (a process's environment) turned into the environment of a
// process of the instance: what named another installation removed, its file
// named.
func (i Instance) Env(base []string) []string {
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(dropped, strings.ToUpper(k)) {
			out = append(out, kv)
		}
	}
	return append(out, cenv.InstallationEnv+"="+i.File())
}

// DefaultFolder is where an installation named name goes: the user's programs
// folder (%LOCALAPPDATA%\Programs\Katyoti\<name>, ~/.local/opt/katyoti/<name>)
// — what the installer uses too. A test process gets it under a root of its
// own (cenv.KeepTestsOut).
func DefaultFolder(name string) string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return cenv.KeepTestsOut(filepath.Join(la, "Programs", "Katyoti", name))
		}
	}
	home, _ := os.UserHomeDir()
	return cenv.KeepTestsOut(filepath.Join(home, ".local", "opt", "katyoti", name))
}

// RegisterCurrent records the installation this process belongs to in the
// registry (under its folder's name), so `katyoti instance list` shows every
// installation that ever ran. Nothing to do outside one or when recorded.
func RegisterCurrent() error {
	inst := cenv.CurrentInstallation()
	if inst == nil {
		return nil
	}
	reg, err := Load()
	if err != nil {
		return err
	}
	for _, i := range reg.Instances {
		if strings.EqualFold(filepath.Clean(i.Folder), filepath.Clean(inst.Dir())) {
			return nil
		}
	}
	name := filepath.Base(inst.Dir())
	for n := 2; ; n++ {
		if _, err := reg.Find(name); err != nil {
			break
		}
		name = fmt.Sprintf("%s-%d", filepath.Base(inst.Dir()), n)
	}
	reg.Instances = append(reg.Instances, Instance{Name: name, Folder: inst.Dir()})
	return reg.Save()
}

// Registry is the list of instances.
type Registry struct {
	Instances []Instance `yaml:"instances"`
}

// RegistryPath is where the registry is kept: $KTW_INSTANCES, else
// <user config folder>/katyoti-instances.yaml.
func RegistryPath() (string, error) { return controlplane.InstallationsRegistryPath() }

// Load reads the registry (missing: empty).
func Load() (Registry, error) {
	var r Registry
	known, err := controlplane.KnownInstallations()
	if err != nil {
		return r, err
	}
	for _, k := range known {
		r.Instances = append(r.Instances, Instance{Name: k.Name, Folder: k.Folder})
	}
	return r, nil
}

// Save writes the registry.
func (r Registry) Save() error { return controlplane.SaveInstallations(r.known()) }

// Find is the instance of that name.
func (r Registry) Find(name string) (Instance, error) {
	for _, i := range r.Instances {
		if strings.EqualFold(i.Name, name) {
			return i, nil
		}
	}
	names := make([]string, 0, len(r.Instances))
	for _, i := range r.Instances {
		names = append(names, i.Name)
	}
	if len(names) == 0 {
		return Instance{}, fmt.Errorf("no instance %q (none created: katyoti instance create <name>)", name)
	}
	return Instance{}, fmt.Errorf("no instance %q (have %s)", name, strings.Join(names, ", "))
}

// validName: one word — letters, digits, dashes, underscores.
func validName(n string) bool {
	if n == "" {
		return false
	}
	for _, c := range n {
		if !(c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

func exeSuffix() string {
	if filepath.Separator == '\\' {
		return ".exe"
	}
	return ""
}
