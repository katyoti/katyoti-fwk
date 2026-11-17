// SPDX-License-Identifier: Apache-2.0

// Package wire moves an available item into a universe (AVAILABLE → USED): it
// upserts a SidecarSpec into the universe's Univers.yaml. A sidecar pins the
// version-addressed store binary; a content item wires via `ktw-host --folder`
// (identity from the manifest). Boundary: ktw-univers still owns universe
// create/list/delete — this only edits membership.
package wire

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Into wires an installed item into a universe's Univers.yaml (upsert by name).
// universeHome is the workspace folder (parent of .katyoti); empty = the current
// workspace. Returns the resolved workspace home.
func Into(ins mktapi.Installed, universeHome string) (string, error) {
	home := universeHome
	if home == "" {
		home = application.Env().WorkspaceDir()
	}
	m, err := catalog.LoadManifestOrRepair(home, filepath.Base(home))
	if err != nil {
		return home, err
	}
	spec := buildSpec(ins)
	upserted := false
	for i := range m.Sidecars {
		if m.Sidecars[i].Name == ins.Name {
			m.Sidecars[i] = spec
			upserted = true
			break
		}
	}
	if !upserted {
		m.Sidecars = append(m.Sidecars, spec)
	}
	if err := catalog.SaveManifest(home, m); err != nil {
		return home, err
	}
	// The rest of a universe (kept when there): without Settings.yaml
	// ktw-agents refuses to serve it, and the GUI cannot read its objects.
	if err := catalog.EnsureSkeleton(home); err != nil {
		return home, err
	}
	return home, nil
}

// Central universe create/list are owned by the registry-backed UniverseService
// (orchestrator) — marketplace delegates to it. The former identity-only
// `CreateUniverseByName` (non-registering) + filesystem-scan `ListUniverses`
// were REMOVED: they produced universes invisible to `ktw-univers list`/the GUI
// (create-invisible-to-list). See universe-service-consolidation.

// OutOf removes an item from a universe's Univers.yaml (USED → AVAILABLE). It is
// the inverse of Into. universeHome empty = the current workspace. Returns the
// resolved home and whether the item was actually wired (false = nothing to do).
func OutOf(name, universeHome string) (string, bool, error) {
	home := universeHome
	if home == "" {
		home = application.Env().WorkspaceDir()
	}
	m, err := catalog.LoadManifestOrRepair(home, filepath.Base(home))
	if err != nil {
		return home, false, err
	}
	kept := m.Sidecars[:0]
	found := false
	for _, sc := range m.Sidecars {
		if sc.Name == name {
			found = true
			continue
		}
		kept = append(kept, sc)
	}
	if !found {
		return home, false, nil
	}
	m.Sidecars = kept
	if err := catalog.SaveManifest(home, m); err != nil {
		return home, false, err
	}
	return home, true, nil
}

// WiredNames returns the set of item names wired into a universe's Univers.yaml
// (USED). universeHome empty = the current workspace. A missing/unreadable
// universe yields an empty set (nothing wired), never an error — the caller just
// wants "which of these are in USE here".
func WiredNames(universeHome string) map[string]bool {
	home := universeHome
	if home == "" {
		home = application.Env().WorkspaceDir()
	}
	m, err := catalog.LoadManifest(home)
	if err != nil || m == nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(m.Sidecars))
	for _, sc := range m.Sidecars {
		out[sc.Name] = true
	}
	return out
}

// UniversesWiring returns the names of every REGISTERED universe whose manifest
// lists name as a member.
//
// `WiredNames("")` — "the current universe" — is not a usable guard in the
// katyoti model, where universes live centrally under the global root and a CLI
// run stands in none of them. Served over KTSP it is worse: the current universe
// is always admin. So the guard it fed never fired, and `katyoti uninstall
// <name>` removed the store versions of an item other universes were pinning,
// reported success, and left them pointing at a path that no longer exists.
//
// Best-effort by design: an unreadable registry or manifest yields no names
// rather than an error, so a reporting helper can never block the operation it
// is advising on. Callers that need a hard guarantee must treat "no names" as
// "unknown", not as "definitely unused" — which is why Uninstall refuses on any
// hit rather than requiring proof of absence.
func UniversesWiring(name string) []UniverseRef {
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		return nil
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		return nil
	}
	var out []UniverseRef
	for _, e := range reg.List() {
		if e.HomePath == "" {
			continue
		}
		if WiredNames(e.HomePath)[name] {
			out = append(out, UniverseRef{Name: e.Name, Home: e.HomePath})
		}
	}
	// The process's own workspace too, when it is a universe the registry does
	// not list (a self-contained project universe a CLI runs in): uninstalling
	// what it pins would strand it all the same. Over KTSP the workspace is
	// admin, which is registered — nothing is added twice.
	if ws := application.Env().WorkspaceDir(); ws != "" && !listed(out, ws) && WiredNames(ws)[name] {
		ref := UniverseRef{Name: filepath.Base(ws), Home: ws}
		if m, err := catalog.LoadManifest(ws); err == nil && m != nil && m.Name != "" {
			ref.Name = m.Name
		}
		out = append(out, ref)
	}
	return out
}

func listed(refs []UniverseRef, home string) bool {
	for _, r := range refs {
		if strings.EqualFold(filepath.Clean(r.Home), filepath.Clean(home)) {
			return true
		}
	}
	return false
}

// WiringByItem maps item name -> the registered universes that pin it.
//
// One pass over the registry, loading each universe manifest ONCE. The per-item
// UniversesWiring is fine for a single guard but is O(items x universes) when
// annotating a whole listing.
func WiringByItem() map[string][]string {
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		return map[string][]string{}
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		return map[string][]string{}
	}
	out := map[string][]string{}
	for _, e := range reg.List() {
		if e.HomePath == "" {
			continue
		}
		for name := range WiredNames(e.HomePath) {
			out[name] = append(out[name], e.Name)
		}
	}
	return out
}

// UniverseRef names a registered universe and where it lives.
type UniverseRef struct {
	Name string
	Home string
}

// RefNames is the display form of a UniverseRef set.
func RefNames(refs []UniverseRef) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Name
	}
	return out
}

// Display carries a universe's identity for CreateUniverse (from a solution's
// captured wiring).
type Display struct {
	DisplayName string
	Icon        string
	// GUI is what the universe's GUI hides and lists first (Univers.yaml
	// `gui:`); nil keeps what the universe already declares.
	GUI *cenv.UniverseGUI
	// Tags are the universe's tags (Univers.yaml `tags:`); nil keeps what
	// the universe already declares.
	Tags []string
	// Solution is the solution the universe is installed from
	// (<solution>@<version>, Univers.yaml `solution:`); empty keeps what the
	// universe already declares.
	Solution string
	// FirstGuide is the guide the universe starts with (Univers.yaml
	// `first_guide:`); empty keeps what the universe already declares.
	FirstGuide string
}

// CreateUniverse writes a universe's Univers.yaml from a set of installed
// members — used by `bootstrap` to reproduce a release. Each member's
// SidecarSpec is (re-)derived from its LOCAL install (buildSpec), so the
// version-pinned store paths are correct on THIS machine regardless of where
// the snapshot was taken. Membership is REPLACED with exactly the release's
// members. Returns the resolved workspace home.
func CreateUniverse(members []mktapi.Installed, disp Display, universeHome string) (string, error) {
	home := universeHome
	if home == "" {
		home = application.Env().WorkspaceDir()
	}
	// CREATE means create. LoadManifestOrRepair only heals a manifest that
	// EXISTS (it re-reads the file and fills a missing `name`); when the file is
	// absent it propagates, so bootstrapping a release into a NEW universe home
	// failed with "Univers.yaml ... is missing required field 'name'" for a file
	// nobody had written yet. Reproducing a release into a fresh universe is the
	// primary use of `katyoti bootstrap`, and it could not work at all.
	name := filepath.Base(home)
	m, err := catalog.LoadManifestOrRepair(home, name)
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")); statErr != nil {
			m = &catalog.Manifest{Version: 1, Name: name, DisplayName: name}
		} else {
			return home, err
		}
	}
	if disp.DisplayName != "" {
		m.DisplayName = disp.DisplayName
	}
	if disp.Icon != "" {
		m.Icon = disp.Icon
	}
	if disp.GUI != nil {
		m.GUI = disp.GUI
	}
	if disp.Tags != nil {
		m.Tags = disp.Tags
	}
	if disp.FirstGuide != "" {
		m.FirstGuide = disp.FirstGuide
	}
	if disp.Solution != "" {
		m.Solution = disp.Solution
	}
	m.Sidecars = nil
	for _, ins := range members {
		m.Sidecars = append(m.Sidecars, buildSpec(ins))
	}
	if err := catalog.SaveManifest(home, m); err != nil {
		return home, err
	}
	// The rest of a universe (kept when there): without Settings.yaml
	// ktw-agents refuses to serve it, and the GUI cannot read its objects.
	if err := catalog.EnsureSkeleton(home); err != nil {
		return home, err
	}
	return home, nil
}

// HostBinaryPath resolves the generic content host to an ABSOLUTE, stattable path.
//
// This used to be the literal "ktw-host", and a bare name does not work: the hub
// STATS spec.Binary on its spawn path ("cache check: stat binary") before ever
// exec'ing it, so every wired content member failed with
//
//	ktw-hub: <name>: cache check: stat binary: GetFileAttributesEx ktw-host: ...
//
// and never spawned. The whole `content` feature was authorable, publishable,
// installable and wirable — and dead at runtime. Same defect as admin's
// ktw-univers (see orchestrator/admin.resolveUniversBinary).
//
// The STORE is consulted first, deliberately. ktw-host is an ordinary catalog
// sidecar, so the installed record is the authoritative, version-pinned answer —
// and it is the only one that does not depend on WHICH process did the wiring.
// Resolving by "sibling of the running executable" alone would write a different
// path depending on whether the CLI or ktw-univers (in <root>/control-plane,
// where ktw-host is not) performed it.
func HostBinaryPath() string {
	name := "ktw-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if items, err := available.List(); err == nil {
		for _, it := range items {
			if it.Name != "ktw-host" || it.BinaryPath == "" {
				continue
			}
			if _, err := os.Stat(it.BinaryPath); err == nil {
				return it.BinaryPath
			}
		}
	}
	// Dev loop: built into the same bin/ as whatever is running.
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	if p, err := exec.LookPath("ktw-host"); err == nil {
		if abs, aerr := filepath.Abs(p); aerr == nil {
			return abs
		}
		return p
	}
	// Nothing resolves: emit the bare name rather than a fabricated path. The
	// spawn still fails, but the manifest stays honest and a later re-wire (once
	// ktw-host is installed) fixes it.
	return "ktw-host"
}

// hasScripts says whether a content folder ships scripts (a scripts/ folder),
// which only the generic host's process can run.
func hasScripts(folder string) bool {
	fi, err := os.Stat(filepath.Join(folder, "scripts"))
	return err == nil && fi.IsDir()
}

// buildSpec derives a universe membership entry. Transport is `nats` — the
// canonical bus-split convention for a hub-served universe (DO.3), matching the
// orchestrator's AddSidecar. (Was `stdio`, the divergent transport that made the
// same universe get different membership shapes depending on which surface wrote it.)
func buildSpec(ins mktapi.Installed) orchapi.SidecarSpec {
	if ins.Kind == "content" && ins.FolderPath != "" && !hasScripts(ins.FolderPath) {
		// A FOLDER member: the hub reads the folder itself, nothing runs
		// (appcore.SidecarSpec.IsFolder).
		return orchapi.SidecarSpec{Name: ins.Name, Transport: "nats", ContentRoot: ins.FolderPath}
	}
	if ins.Kind == "content" && ins.FolderPath != "" {
		// Its scripts need a process: the generic host runs the folder.
		return orchapi.SidecarSpec{
			Name:        ins.Name,
			Transport:   "nats",
			Binary:      HostBinaryPath(),
			ContentRoot: ins.FolderPath,
			Args:        []string{"--folder", ins.FolderPath, "sc", "ktsp", "nats"},
			// The host resolves its folder from the environment, before Cobra
			// parses anything — the flag stays for a human running it directly.
			Env: map[string]string{"KTW_CONTENT_ROOT": ins.FolderPath},
		}
	}
	return orchapi.SidecarSpec{
		Name:      ins.Name,
		Transport: "nats",
		Binary:    ins.BinaryPath, // version-pinned store path
		Args:      []string{"sc", "ktsp", "nats"},
	}
}

// PinsVersion reports whether the universe at home pins name at exactly version.
//
// The pin is a versioned STORE PATH (…/store/<name>/<version>/<bin>), so the
// version is read from the path rather than from a field — that path IS the pin,
// and it is what the hub execs.
func PinsVersion(home, name, version string) bool {
	m, err := catalog.LoadManifest(home)
	if err != nil || m == nil {
		return false
	}
	want := string(os.PathSeparator) + version + string(os.PathSeparator)
	for _, sc := range m.Sidecars {
		if sc.Name != name {
			continue
		}
		if strings.Contains(sc.Binary, want) {
			return true
		}
		// A folder member's pin is its store folder, which ends on the version.
		if sc.ContentRoot != "" && strings.Contains(filepath.Clean(sc.ContentRoot)+string(os.PathSeparator), want) {
			return true
		}
		for _, a := range sc.Args {
			if strings.Contains(a, want) {
				return true
			}
		}
	}
	return false
}

// UniversesPinning is the registered universes whose wiring of name runs its
// store version version (the version folder is in the path of the binary, or
// of the folder a content host is given): uninstalling that version would
// leave them pointing at a deleted path.
func UniversesPinning(name, version string) []UniverseRef {
	return universesRunning(name, "/"+name+"/"+version+"/")
}

// UniversesHosting is the registered universes whose members run the store's
// name — any version: a content member's binary is the store's ktw-host, so a
// universe runs ktw-host without having a member of that name.
func UniversesHosting(name string) []UniverseRef {
	return universesRunning(name, "/store/"+name+"/")
}

// universesRunning is the universes with a member whose binary path holds seg
// (a member of ANY name: content members run the store's ktw-host), or whose
// member named name is given a folder holding it (a content item's folder).
func universesRunning(name, seg string) []UniverseRef {
	var out []UniverseRef
	for _, u := range registeredUniverses() {
		m, err := catalog.LoadManifest(u.Home)
		if err != nil || m == nil {
			continue
		}
	members:
		for _, sc := range m.Sidecars {
			paths := []string{sc.Binary}
			if sc.Name == name {
				paths = append(paths, sc.Args...)
			}
			for _, p := range paths {
				if strings.Contains(filepath.ToSlash(p)+"/", seg) {
					out = append(out, u)
					break members
				}
			}
		}
	}
	return out
}

// RegisteredUniverses is every registered universe, and the process's own
// workspace when it is a universe the registry does not list.
func RegisteredUniverses() []UniverseRef { return registeredUniverses() }

// registeredUniverses is RegisteredUniverses.
func registeredUniverses() []UniverseRef {
	var out []UniverseRef
	if regPath, err := catalog.DefaultRegistryPath(); err == nil {
		if reg, err := catalog.LoadRegistry(regPath); err == nil {
			for _, e := range reg.List() {
				if e.HomePath != "" {
					out = append(out, UniverseRef{Name: e.Name, Home: e.HomePath})
				}
			}
		}
	}
	if ws := application.Env().WorkspaceDir(); ws != "" && !listed(out, ws) {
		if m, err := catalog.LoadManifest(ws); err == nil && m != nil {
			name := m.Name
			if name == "" {
				name = filepath.Base(ws)
			}
			out = append(out, UniverseRef{Name: name, Home: ws})
		}
	}
	return out
}

// RepinToPrograms re-points, in every registered universe, the members whose
// binary lies in no installation (a checkout's bin/, a test's folder) to the
// same-named program program answers (the installation's own copy). A member
// whose program the installation does not have is reported, not moved. With
// dry, nothing is written.
func RepinToPrograms(program func(name string) (string, error), dry bool) (moved, missing []mktapi.ProgramRepin, err error) {
	for _, u := range registeredUniverses() {
		m, lerr := catalog.LoadManifest(u.Home)
		if lerr != nil || m == nil {
			continue
		}
		changed := false
		for i := range m.Sidecars {
			sc := &m.Sidecars[i]
			if sc.Binary == "" || !filepath.IsAbs(sc.Binary) || cenv.InstallationAbove(sc.Binary) != "" {
				continue
			}
			base := filepath.Base(sc.Binary)
			name := strings.TrimSuffix(base, filepath.Ext(base))
			r := mktapi.ProgramRepin{Universe: u.Name, Member: sc.Name, From: sc.Binary}
			to, perr := program(name)
			if perr != nil {
				return moved, missing, perr
			}
			if _, serr := os.Stat(to); serr != nil {
				missing = append(missing, r)
				continue
			}
			r.To = to
			moved = append(moved, r)
			sc.Binary = to
			changed = true
		}
		if changed && !dry {
			if err := catalog.SaveManifest(u.Home, m); err != nil {
				return moved, missing, err
			}
		}
	}
	return moved, missing, nil
}

// StorePin is the store version a universe member runs item name at: the
// version folder of …/store/<name>/<version>/ (a sidecar's binary) or
// …/store-content/<name>/<version> (a content item's folder, given to the
// host or read as a folder member). ok is false when the member runs a
// program outside the store (a development build, the installation's own
// program): no version of the store is pinned, so none is behind.
func StorePin(sc orchapi.SidecarSpec, name string) (version string, ok bool) {
	paths := append([]string{sc.Binary, sc.ContentRoot}, sc.Args...)
	for _, p := range paths {
		if p == "" {
			continue
		}
		s := filepath.ToSlash(filepath.Clean(p)) + "/"
		for _, seg := range []string{"/store/" + name + "/", "/store-content/" + name + "/"} {
			i := strings.Index(s, seg)
			if i < 0 {
				continue
			}
			rest := s[i+len(seg):]
			if j := strings.Index(rest, "/"); j > 0 {
				return rest[:j], true
			}
		}
	}
	return "", false
}

// PinsByItem maps item name -> the version each registered universe runs it
// at (one pass over the registry, each manifest loaded once). Version is
// empty for a member running a program outside the store.
func PinsByItem() map[string][]mktapi.UniversePin {
	out := map[string][]mktapi.UniversePin{}
	for _, u := range registeredUniverses() {
		m, err := catalog.LoadManifest(u.Home)
		if err != nil || m == nil {
			continue
		}
		for _, sc := range m.Sidecars {
			v, _ := StorePin(sc, sc.Name)
			out[sc.Name] = append(out[sc.Name], mktapi.UniversePin{Universe: u.Name, Version: v})
		}
	}
	return out
}

// Repinned is one universe whose member moved to the installed version.
type Repinned struct {
	Universe UniverseRef
	Member   string // the member moved (set by RepinVanished)
	From     string // the store version it ran (empty: unknown)
	Vanished bool   // the program it was pinned to no longer exists
}

// RepinToInstalled points, in every registered universe, the member named
// ins.Name that runs ANOTHER store version of it at ins (
// RQ-063): the version an update or a version change made current, or the
// installed one when the pinned program is gone. A member running a program
// outside the store is left alone (the developer's build, the installation's
// own program). A universe installed from a solution pins its members on
// purpose: it keeps a version that still exists - reported in behind, for its
// own update - and is moved only off a program that no longer exists, which
// nothing can run. Univers.yaml is written by this, the hub's marketplace
// service; each universe's hub reloads it.
func RepinToInstalled(ins mktapi.Installed) (moved []Repinned, behind []UniverseRef, err error) {
	if ins.Version == "" {
		return nil, nil, nil
	}
	spec := buildSpec(ins)
	for _, u := range registeredUniverses() {
		m, lerr := catalog.LoadManifest(u.Home)
		if lerr != nil || m == nil {
			continue
		}
		changed := false
		for i := range m.Sidecars {
			sc := &m.Sidecars[i]
			if sc.Name != ins.Name {
				continue
			}
			from, inStore := StorePin(*sc, ins.Name)
			if !inStore || from == ins.Version {
				continue
			}
			vanished := !pinnedProgramExists(*sc)
			if m.Solution != "" && !vanished {
				behind = append(behind, u)
				continue
			}
			repoint(sc, spec)
			changed = true
			moved = append(moved, Repinned{Universe: u, From: from, Vanished: vanished})
		}
		if changed {
			if err := catalog.SaveManifest(u.Home, m); err != nil {
				return moved, behind, err
			}
		}
	}
	return moved, behind, nil
}

// repoint moves a member onto what spec runs - its program, its folder, its
// arguments and the content root it is told - and keeps everything else the
// universe gave it (its config dir, env, grants, lifecycle, timeouts…).
func repoint(sc *orchapi.SidecarSpec, spec orchapi.SidecarSpec) {
	sc.Binary, sc.ContentRoot, sc.Args = spec.Binary, spec.ContentRoot, spec.Args
	if len(spec.Env) > 0 {
		env := make(map[string]string, len(sc.Env)+len(spec.Env))
		for k, v := range sc.Env {
			env[k] = v
		}
		for k, v := range spec.Env {
			env[k] = v
		}
		sc.Env = env
	}
}

// pinnedProgramExists says whether what a member runs is still on disk: its
// binary, and the folder it is given (a content member).
func pinnedProgramExists(sc orchapi.SidecarSpec) bool {
	for _, p := range []string{sc.Binary, sc.ContentRoot} {
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// MissingPin is a universe member whose program is gone and that no
// installed item can replace: it is kept (never dropped from its universe)
// and reported not installed, for the person to reinstall it.
type MissingPin struct {
	Universe UniverseRef
	Member   string
	Program  string // the binary or folder it was pinned to
}

// RepinVanished rewires, in every registered universe, each member pinned to
// a program or folder that no longer exists (deleted by hand, a store version
// removed) to the installed item of the same name: only its
// program, folder, arguments and content root move (repoint), what the
// universe gave it stays. A member no installed item can replace is kept and
// reported in missing. A member whose program is there - a store version, a
// development build - is never touched. Univers.yaml is written here, by the
// hub's marketplace service; each hub reloads it.
func RepinVanished(installed []mktapi.Installed) (moved []Repinned, missing []MissingPin, err error) {
	byName := map[string]mktapi.Installed{}
	for _, ins := range installed {
		if ins.BinaryPath == "" && ins.FolderPath == "" {
			continue
		}
		if !pinnedProgramExists(orchapi.SidecarSpec{Binary: ins.BinaryPath, ContentRoot: ins.FolderPath}) {
			continue
		}
		byName[ins.Name] = ins
	}
	for _, u := range registeredUniverses() {
		m, lerr := catalog.LoadManifest(u.Home)
		if lerr != nil || m == nil {
			continue
		}
		changed := false
		for i := range m.Sidecars {
			sc := &m.Sidecars[i]
			if !cenv.Vanished(sc.Binary) && !cenv.Vanished(sc.ContentRoot) {
				continue
			}
			program := sc.Binary
			if !cenv.Vanished(program) {
				program = sc.ContentRoot
			}
			ins, ok := byName[sc.Name]
			if !ok {
				missing = append(missing, MissingPin{Universe: u, Member: sc.Name, Program: program})
				continue
			}
			from, _ := StorePin(*sc, sc.Name)
			repoint(sc, buildSpec(ins))
			changed = true
			moved = append(moved, Repinned{Universe: u, Member: sc.Name, From: from, Vanished: true})
		}
		if changed {
			if err := catalog.SaveManifest(u.Home, m); err != nil {
				return moved, missing, err
			}
		}
	}
	return moved, missing, nil
}
