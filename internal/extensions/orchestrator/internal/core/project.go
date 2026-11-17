// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/hubdial"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmremote "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// Project attach/detach are UNIVERSE-registry concerns (a project↔universe link),
// so they live with the universe owner (OrchestratorService), NOT the marketplace.
// Moved here from marketplace/internal/wire as part of the marketplace/universe
// concept split (universe-service-consolidation / marketplace-service-ktsp).

// AttachProject links a project folder to a central universe by writing
// `<projectDir>/.katyoti/project.yaml` (`universe: <name>`) and recording the
// project in the universe's own attached-projects registry. Refuses an unknown
// central universe.
//
// SEVERAL REPOSITORIES ARE MEMBERS: the registry lists every
// member, and a second attach adds one: the CWM service opens one store per
// member, joined by the store set. A member beside another
// holds a key (the id the store set addresses its store by,
// cwmapi.ProjectKeyOf) and a contributions token no other member holds.
//
// A universe that runs serves the project at once: the member
// serving its objects binds its project when it starts, so an attach that
// changed what the universe serves restarts it through the universe's hub,
// as SwitchProject does; an attach of the project already served restarts
// nothing.
func (s *Service) AttachProject(ctx context.Context, in api.AttachProjectInput) (api.AttachProjectOutput, error) {
	changed, err := s.attachProject(in)
	if err != nil {
		return api.AttachProjectOutput{}, err
	}
	out := api.AttachProjectOutput{Universe: in.Universe, ProjectDir: in.ProjectDir}
	if changed {
		out.Restarted, out.RestartError = s.restartServing(ctx, in.Universe)
	}
	return out, nil
}

// attachProject is AttachProject without the restart - SwitchProject's
// detach and attach restart the serving member once, after both. It answers
// whether anything changed: false when the project was attached already.
func (s *Service) attachProject(in api.AttachProjectInput) (bool, error) {
	if in.Universe == "" {
		return false, fmt.Errorf("attach: universe name is required")
	}
	if in.ProjectDir == "" {
		return false, fmt.Errorf("attach: project dir is required")
	}
	// A project never lives where an uninstall or a reinstall takes it.
	if err := cenv.CurrentInstallation().CheckProjectDir(in.ProjectDir); err != nil {
		return false, fmt.Errorf("attach: %w", err)
	}
	if err := notEnclosed(in.ProjectDir); err != nil {
		return false, fmt.Errorf("attach: %w", err)
	}
	// THE REGISTRY SAYS WHERE A UNIVERSE LIVES. This used to compose
	// `<GlobalHomeDir>/universes/<name>/.katyoti` by convention — which is
	// merely where `universe create` puts one when `--home` is omitted, not
	// where a universe must be. `universes.yaml` records a `home_path` per
	// universe exactly so one can sit anywhere, and a repository checkout is
	// the ordinary case. A universe registered at D:\Projets\ktw-agents was
	// therefore unattachable, refused with a path nobody had chosen.
	reg, err := s.registryOrLoad()
	if err != nil {
		return false, fmt.Errorf("attach: read universe registry: %w", err)
	}
	entry, err := reg.Find(in.Universe)
	if err != nil {
		return false, fmt.Errorf("attach: universe %q is not registered: %w", in.Universe, err)
	}
	universeKatyoti := application.EnvOf(entry.HomePath).HomeDir()
	if fi, statErr := os.Stat(universeKatyoti); statErr != nil || !fi.IsDir() {
		return false, fmt.Errorf(
			"attach: universe %q is registered at %s but has no config root at %s — create it first",
			in.Universe, entry.HomePath, universeKatyoti)
	}
	// REFUSE BEFORE WRITING ANYTHING. This check used to sit after the
	// project.yaml write, which left a rejected project pointing at a
	// universe it was not attached to -- a refusal that still mutated the
	// caller. An operation that says no must leave nothing behind.
	// An already-attached project re-attaching is idempotent (registerProject
	// dedupes), so only a DIFFERENT member is checked.
	abs, _ := filepath.Abs(in.ProjectDir)
	if err := s.memberBeside(in.Universe, abs); err != nil {
		return false, err
	}

	// NEVER THE SAME PROJECT ID TWICE IN ONE UNIVERSE (identity-and-index.md
	// §1): the same id in two places means the same project, and a universe
	// holding it twice would index one set of names from two folders.
	if newID := projectUUIDOf(abs); newID != "" {
		for _, existing := range s.loadProjects(in.Universe).Projects {
			if existing != abs && projectUUIDOf(existing) == newID {
				return false, fmt.Errorf(
					"universe %q already mounts project %s from %s — the same project id twice is the same project; unmount it first, or fork the copy",
					in.Universe, newID, existing)
			}
		}
	}
	// ONE UNIVERSE PER PROJECT. A project's CWM store lives in the project
	// (`<project>/.katyoti`), so two universes attached to one folder are two
	// hubs writing one store; the store's lease only refuses the second writer
	// after the fact. A project whose pointer names another universe that still
	// lists it is refused; a stale pointer (that universe gone, or no longer
	// listing it) is taken over.
	if other := projectUniverse(abs); other != "" && other != in.Universe && s.servesProject(other, abs) {
		return false, fmt.Errorf(
			"project %s is attached to universe %q, which still serves it.\n"+
				"One universe per project: two universes on one folder would write one CWM store.\n"+
				"Detach it there first (katyoti project detach %s), or give this universe a copy of the project.",
			abs, other, abs)
	}
	// THE TOKEN RULE (projectfile.TokenConflict), as the hub applies it: a
	// project that holds a contributions folder publishes it under its
	// token, which no member of the universe may answer to. Refused here,
	// before the hub would refuse to serve it.
	if err := tokenConflict(entry.HomePath, abs); err != nil {
		return false, err
	}
	unchanged := projectUniverse(abs) == in.Universe && s.servesProject(in.Universe, abs)
	katyoti := application.EnvOf(in.ProjectDir).HomeDir()
	if err := os.MkdirAll(katyoti, 0o755); err != nil {
		return false, fmt.Errorf("attach: create %s: %w", katyoti, err)
	}
	// project.yaml is a bootstrap-layer pointer (read before the config service
	// exists) — NOT CWM's config/workspace.yaml.
	if err := os.WriteFile(filepath.Join(katyoti, "project.yaml"), []byte("universe: "+in.Universe+"\n"), 0o644); err != nil {
		return false, fmt.Errorf("attach: write project.yaml: %w", err)
	}
	if err := s.registerProject(in.Universe, in.ProjectDir); err != nil {
		return false, fmt.Errorf("attach: register project: %w", err)
	}
	return !unchanged, nil
}

// tokenConflict refuses a project whose contributions the universe at home
// could not serve: it holds a contributions folder, and its token is
// reserved or a member's name. A project with no such folder contributes
// nothing and is never refused for its token.
func tokenConflict(home, projectDir string) error {
	f, err := projectfile.Read(projectDir)
	if err != nil {
		return fmt.Errorf("attach: %w", err)
	}
	if f.ContributionsDir(projectDir) == "" {
		return nil
	}
	m, err := catalog.LoadManifest(home)
	if err != nil {
		return fmt.Errorf("attach: read the universe's members: %w", err)
	}
	names := make([]string, 0, len(m.Sidecars))
	for _, s := range m.Sidecars {
		names = append(names, s.Name)
	}
	if why := projectfile.TokenConflict(f.Contributions.Provider, names); why != "" {
		return fmt.Errorf("attach: %s", why)
	}
	return nil
}

// DetachProject unlinks a project from its universe: deregisters it from the
// universe's projects.yaml, then removes the project.yaml pointer. No-op when not
// attached. A universe that runs stops serving it at once: the
// member serving its objects is restarted through its hub, as on an attach.
func (s *Service) DetachProject(ctx context.Context, in api.DetachProjectInput) (api.DetachProjectOutput, error) {
	uni, err := s.detachProject(in)
	if err != nil {
		return api.DetachProjectOutput{}, err
	}
	out := api.DetachProjectOutput{ProjectDir: in.ProjectDir}
	if uni != "" {
		out.Restarted, out.RestartError = s.restartServing(ctx, uni)
	}
	return out, nil
}

// detachProject is DetachProject without the restart (SwitchProject restarts
// once, after its attach). It answers the universe the project was attached
// to - read before its pointer goes - or "" when it was attached to none.
func (s *Service) detachProject(in api.DetachProjectInput) (string, error) {
	if in.ProjectDir == "" {
		return "", fmt.Errorf("detach: project dir is required")
	}
	// The universe is the one named, else the folder's own pointer, else the
	// registered universe whose projects.yaml lists the folder: a folder that
	// no longer exists has no pointer left, and its link must still go
	//
	uni := in.Universe
	if uni == "" {
		uni = projectUniverse(in.ProjectDir)
	}
	if uni == "" {
		uni = s.universeListing(in.ProjectDir)
	}
	if uni != "" {
		_ = s.deregisterProject(uni, in.ProjectDir) // best-effort; pointer removal below is authoritative
	}
	p := filepath.Join(application.EnvOf(in.ProjectDir).HomeDir(), "project.yaml")
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("detach: remove project.yaml: %w", err)
	}
	return uni, nil
}

// SwitchProject makes a universe serve another project in place of the one
// it serves: the served one is detached first and the
// other attached; when the attach refuses, the first is attached back and
// the refusal said - the universe never ends serving nothing because of a
// refused switch.
func (s *Service) SwitchProject(ctx context.Context, in api.SwitchProjectInput) (api.SwitchProjectOutput, error) {
	if in.Universe == "" {
		return api.SwitchProjectOutput{}, fmt.Errorf("switch: universe name is required")
	}
	if in.ProjectDir == "" {
		return api.SwitchProjectOutput{}, fmt.Errorf("switch: project dir is required")
	}
	abs, err := filepath.Abs(in.ProjectDir)
	if err != nil {
		return api.SwitchProjectOutput{}, fmt.Errorf("switch: %w", err)
	}
	// Refused before the served project is detached, not rolled back after.
	if err := notEnclosed(abs); err != nil {
		return api.SwitchProjectOutput{}, fmt.Errorf("switch: %w", err)
	}
	out := api.SwitchProjectOutput{Universe: in.Universe, ProjectDir: abs, Left: []string{}}
	served := s.loadProjects(in.Universe).Projects
	if len(served) == 1 && served[0] == abs {
		return out, nil
	}
	for _, dir := range served {
		if dir == abs {
			continue
		}
		if _, err := s.detachProject(api.DetachProjectInput{ProjectDir: dir}); err != nil {
			return out, fmt.Errorf("switch: detach %s: %w", dir, err)
		}
		out.Left = append(out.Left, dir)
	}
	if _, err := s.attachProject(api.AttachProjectInput{Universe: in.Universe, ProjectDir: abs}); err != nil {
		for _, dir := range out.Left {
			_, _ = s.attachProject(api.AttachProjectInput{Universe: in.Universe, ProjectDir: dir})
		}
		return api.SwitchProjectOutput{}, fmt.Errorf("switch: %w", err)
	}
	out.Changed = true
	out.Restarted, out.RestartError = s.restartServing(ctx, in.Universe)
	return out, nil
}

// servingMember is the member that serves a universe's objects (CWM): it
// binds its project once, when it starts.
const servingMember = cwmremote.Host

// servingRestartWait bounds the serving member's restart: its stop grace and
// its start.
const servingRestartWait = 3 * time.Minute

// hubDialer reaches the hub of the universe at home: nil (no error) when no
// hub runs there.
type hubDialer func(ctx context.Context, home string) (hubdial.Conn, error)

// restartServing applies a switch: the member serving the
// universe's objects is restarted through the universe's hub, which owns its
// members, so it binds the project now attached - for the
// GUI's switch and a command line's alike. A universe that does not run, or
// whose serving member does not, starts on the project: nothing to restart.
// A restart that fails is said, never undoing the switch.
func (s *Service) restartServing(ctx context.Context, universe string) (bool, string) {
	home := s.universeHome(universe)
	if home == "" {
		return false, ""
	}
	dial := s.dialHub
	if dial == nil {
		dial = hubdial.Dial
	}
	hub, err := dial(ctx, home)
	if err != nil {
		return false, "the universe's hub could not be reached: " + err.Error()
	}
	if hub == nil {
		return false, ""
	}
	defer hub.Close()
	rctx, cancel := context.WithTimeout(ctx, servingRestartWait)
	defer cancel()
	restarted, err := hub.Restart(rctx, servingMember)
	if err != nil {
		return false, servingMember + " could not be restarted: " + err.Error()
	}
	return restarted, ""
}

// ListProjects returns the project paths attached to a universe.
func (s *Service) ListProjects(_ context.Context, in api.ListProjectsInput) (api.ListProjectsOutput, error) {
	// Same nil-slice rule as ToUniverseInfo: the schema declares `projects` a
	// required ARRAY, and Go marshals a nil slice as `null`. A universe with no
	// attached projects must send [], not null, or a schema-trusting client breaks.
	projects := s.loadProjects(in.Universe).Projects
	if projects == nil {
		projects = []string{}
	}
	return api.ListProjectsOutput{Projects: projects}, nil
}

// ── Attached-projects registry: <GlobalHomeDir>/universes/<name>/projects.yaml ──

type projectsFile struct {
	Projects []string `yaml:"projects"`
}

// universeProjectsPath returns the attached-projects registry for a universe,
// IN THAT UNIVERSE'S OWN HOME.
//
// This was the THIRD place composing `<GlobalHomeDir>/universes/<name>/`, after
// the attach check and `env.go`'s runtime resolution. The list of projects
// attached to a universe belongs beside that universe, wherever the registry
// says it lives; writing it centrally for a universe that lives in a repository
// put its state somewhere it does not live — and the write then failed on a
// directory nobody had created, AFTER the attach had already been accepted.
//
// The central path stays as the fallback, for a name with no registry entry.
func (s *Service) universeProjectsPath(universeName string) string {
	if home := s.universeHome(universeName); home != "" {
		// INSIDE the universe's CONFIG root, not beside it. A universe home is
		// commonly a repository checkout, and this file holds ABSOLUTE,
		// machine-local project paths — `modules/project/probes/pr2-no-local-paths.sh`
		// exists because such paths must never be committed. Writing it at the
		// home root dropped a `projects.yaml` into the repository on the first
		// successful attach; the config root is already ignored, and is where
		// per-universe machine state belongs.
		return filepath.Join(application.EnvOf(home).HomeDir(), cenv.AttachedProjectsFile)
	}
	return filepath.Join(application.Env().GlobalHomeDir(), "universes", universeName, cenv.AttachedProjectsFile)
}

// universeHome returns a universe's registered home, or "" if it is not
// registered. Best-effort: callers that must REFUSE an unregistered universe
// check the registry themselves and report why.
func (s *Service) universeHome(universeName string) string {
	reg, err := s.registryOrLoad()
	if err != nil {
		return ""
	}
	entry, err := reg.Find(universeName)
	if err != nil {
		return ""
	}
	return entry.HomePath
}

// loadProjects reads through cenv.AttachedProjects — the ONE reader of this
// file, shared with the CWM object scan, so the two can never disagree on
// what "attached" means.
func (s *Service) loadProjects(universeName string) projectsFile {
	return projectsFile{Projects: cenv.AttachedProjects(filepath.Dir(s.universeProjectsPath(universeName)))}
}

func (s *Service) saveProjects(universeName string, pf projectsFile) error {
	// The operator's trust of each project is kept as the file says it: a
	// rewrite on attach or detach never drops a `trusted: true`.
	dir := filepath.Dir(s.universeProjectsPath(universeName))
	var out struct {
		Projects []cenv.AttachedEntry `yaml:"projects"`
	}
	for _, p := range pf.Projects {
		out.Projects = append(out.Projects, cenv.AttachedEntry{Path: p, Trusted: cenv.ProjectTrusted(dir, p)})
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	path := s.universeProjectsPath(universeName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Service) registerProject(universeName, projectDir string) error {
	abs, _ := filepath.Abs(projectDir)
	pf := s.loadProjects(universeName)
	for _, p := range pf.Projects {
		if p == abs {
			return nil // already registered
		}
	}
	pf.Projects = append(pf.Projects, abs)
	return s.saveProjects(universeName, pf)
}

func (s *Service) deregisterProject(universeName, projectDir string) error {
	abs, _ := filepath.Abs(projectDir)
	pf := s.loadProjects(universeName)
	kept := pf.Projects[:0]
	for _, p := range pf.Projects {
		if !samePath(p, abs) {
			kept = append(kept, p)
		}
	}
	pf.Projects = kept
	return s.saveProjects(universeName, pf)
}

// universeListing answers the first registered universe whose attached-projects
// registry lists the folder, or "" when none does.
func (s *Service) universeListing(projectDir string) string {
	abs, _ := filepath.Abs(projectDir)
	reg, err := s.registryOrLoad()
	if err != nil {
		return ""
	}
	for _, e := range reg.List() {
		if s.servesProject(e.Name, abs) {
			return e.Name
		}
	}
	return ""
}

// servesProject reports whether the universe's attached-projects registry still
// lists the project folder at abs.
func (s *Service) servesProject(universeName, abs string) bool {
	for _, p := range s.loadProjects(universeName).Projects {
		if samePath(p, abs) {
			return true
		}
	}
	return false
}

// samePath compares two absolute folder paths as the file system does: case
// apart on Windows.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// projectUniverse reads the universe a project is attached to (from
// `<projectDir>/.katyoti/project.yaml`), or "" if self-contained.
func projectUniverse(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(application.EnvOf(projectDir).HomeDir(), "project.yaml"))
	if err != nil {
		return ""
	}
	var pointer struct {
		Universe string `yaml:"universe"`
	}
	if yaml.Unmarshal(data, &pointer) != nil {
		return ""
	}
	return pointer.Universe
}

// registryOrLoad returns the service's registry, loading the default one when
// the service was built without it.
//
// AttachProject genuinely needs the registry now — it used to compose the
// universe path by convention — and the zero-value Service is a real
// construction in this package's tests. Loading on demand keeps both honest
// rather than making every caller build a registry it otherwise never touches.
func (s *Service) registryOrLoad() (*catalog.Registry, error) {
	if s.registry != nil {
		return s.registry, nil
	}
	path, err := catalog.DefaultRegistryPath()
	if err != nil {
		return nil, err
	}
	return catalog.LoadRegistry(path)
}

// projectUUIDOf reads a mounted project's id from `<dir>/project.cwm.yaml`, ""
// when the project has none yet (not a fault: `cwm project-init` mints it).
// A minimal read — the orchestrator owns the registry, not the object model.
func projectUUIDOf(projectDir string) string {
	f, err := projectfile.Read(projectDir)
	if err != nil {
		return ""
	}
	return f.UUID
}

// projectNameOf reads the project object's id (its name), "" when absent.
func projectNameOf(projectDir string) string {
	if _, err := os.Stat(filepath.Join(projectDir, projectfile.FileName)); err != nil {
		return ""
	}
	f, err := projectfile.Read(projectDir)
	if err != nil {
		return ""
	}
	return f.ID
}

// ResolveProject walks the registry: every universe, every mounted project,
// the project's id. A full object name is accepted (its first segment is the
// project id); the authored id is handed back for the universe core to finish.
func (s *Service) ResolveProject(_ context.Context, in api.ResolveProjectInput) (api.ResolveProjectOutput, error) {
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		return api.ResolveProjectOutput{}, fmt.Errorf("resolve: ref is required")
	}
	projectID, objectID := ref, ""
	if pid, id, err := cwmapi.ParseFullName(ref); err == nil {
		projectID, objectID = pid, id
	}
	reg, err := s.registryOrLoad()
	if err != nil {
		return api.ResolveProjectOutput{}, fmt.Errorf("resolve: read universe registry: %w", err)
	}
	for _, u := range reg.List() {
		for _, dir := range s.loadProjects(u.Name).Projects {
			if projectUUIDOf(dir) == projectID {
				return api.ResolveProjectOutput{
					ProjectID: projectID, Universe: u.Name, ProjectDir: dir,
					Name: projectNameOf(dir), ObjectID: objectID,
				}, nil
			}
		}
	}
	return api.ResolveProjectOutput{}, fmt.Errorf("project %s is not mounted in any universe on this machine", projectID)
}

// CreateProject makes a new project under its name in the projects folder
// : refused before anything is created by placeProject's rule
// then the folder made and, when a universe is named, attached - a refused
// attach removes the folder it made.
func (s *Service) CreateProject(ctx context.Context, in api.CreateProjectInput) (api.CreateProjectOutput, error) {
	dir, reason, refused := s.placeProject(in.Name)
	u := strings.TrimSpace(in.Universe)
	out := api.CreateProjectOutput{Name: in.Name, ProjectDir: dir, Reason: reason, Refused: refused}
	if in.Check {
		return out, nil
	}
	if refused != "" {
		return out, fmt.Errorf("project %q: %s", in.Name, refused)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return out, fmt.Errorf("project %q: create %s: %w", in.Name, dir, err)
	}
	out.Created = true
	// Created on this PC: recorded once the folder is made
	// dropped when the creation removes it - a failure to record leaves only
	// a later attach beside other members refused, never the creation.
	_ = s.recordCreatedHere(dir)
	if strings.TrimSpace(in.Type) != "" {
		if err := seedTypedProject(ctx, dir, in); err != nil {
			_ = os.RemoveAll(dir)
			s.forgetCreatedHere(dir)
			return api.CreateProjectOutput{Name: in.Name, ProjectDir: dir}, fmt.Errorf("project %q is not created: %w", in.Name, err)
		}
	}
	if u != "" {
		att, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: u, ProjectDir: dir})
		if err != nil {
			_ = os.RemoveAll(dir)
			s.forgetCreatedHere(dir)
			return api.CreateProjectOutput{Name: in.Name, ProjectDir: dir}, fmt.Errorf("project %q is not created: %w", in.Name, err)
		}
		out.Universe = u
		out.Restarted, out.RestartError = att.Restarted, att.RestartError
	}
	return out, nil
}

// seedTypedProject makes a new folder a typed project: a
// git repository whose store (.katyoti, ignored by git with every secret path
// of secrets.GitignorePatterns) holds the seed of its
// project object - the type, name, title and groups given. The hub never
// writes the project object: CWM mints it from the seed at the project's
// first opening, at the code's data version, its groups checked.
func seedTypedProject(ctx context.Context, dir string, in api.CreateProjectInput) error {
	seed := cwmapi.ProjectSeed{Name: in.Name, Title: strings.TrimSpace(in.Title), Type: strings.TrimSpace(in.Type), Capabilities: in.Capabilities}
	data, err := yaml.Marshal(seed)
	if err != nil {
		return err
	}
	store := application.EnvOf(dir).HomeDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(store, cwmapi.ProjectSeedFile), data, 0o644); err != nil {
		return err
	}
	ignore := "# The project's store: machine-local state (its index, its secrets), never committed.\n" +
		filepath.Base(store) + "/\n" +
		"# Secret paths (secrets.GitignorePatterns): never committed.\n" +
		strings.Join(secrets.GitignorePatterns(), "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(ignore), 0o644); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "init", "--quiet")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// placeProject is where a new project named name goes - <projects
// folder>/<name> - and why it cannot be created there (its reason code and its
// sentence), empty when it can: the rule of a universe name. The name is
// lowercase letters, digits and dashes; no project of that name is registered
// in a universe of this machine (its folder or its project object named so);
// its folder does not exist; it is not inside the installation. No enclosure
// check here: the folder is always <projects folder>/<name>
// which notEnclosed accepts even when the projects folder lies in a
// repository - that repository encloses every project alike - so the check
// could never refuse; CreateProject attaches the made folder through
// attachProject, which runs notEnclosed on it (stays guarded).
func (s *Service) placeProject(name string) (dir, reason, refused string) {
	if !validUniverseName(name) {
		return "", "invalid-name", fmt.Sprintf("invalid project name %q: lowercase letters, digits and dashes (^[a-z0-9-]{1,64}$)", name)
	}
	dir = defaultProjectDir(name)
	if reg, err := s.registryOrLoad(); err == nil {
		for _, u := range reg.List() {
			for _, p := range s.loadProjects(u.Name).Projects {
				if strings.EqualFold(filepath.Base(filepath.Clean(p)), name) || cwmapi.ProjectKeyOf(p) == name {
					return dir, "registered", fmt.Sprintf("a project named %q is registered: %s, in universe %q - choose another name", name, p, u.Name)
				}
			}
		}
	}
	if _, err := os.Stat(dir); err == nil {
		return dir, "folder-exists", fmt.Sprintf("the folder %s already exists - choose another name, or attach that folder as it is", dir)
	}
	if err := cenv.CurrentInstallation().CheckProjectDir(dir); err != nil {
		return dir, "inside-installation", err.Error()
	}
	return dir, "", ""
}

// notEnclosed says why dir cannot hold a project: it lies inside another
// project or Git repository (: no repository ever sits inside
// another;: no project inside a project), named with
// where the project belongs - the projects folder, as a project of its own.
// A folder of the projects folder is accepted even when the projects folder
// itself lies in a repository. Nil: it can.
func notEnclosed(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("the project folder %s: %w", dir, err)
	}
	enclosing := cwmapi.EnclosingProject(abs)
	if enclosing == "" {
		return nil
	}
	projects, _ := cenv.CurrentInstallation().ProjectsFolder()
	if projects != "" && within(projects, abs) && within(enclosing, projects) {
		return nil // the projects folder's own repository encloses every project alike
	}
	return fmt.Errorf("the project folder %s is inside the project or Git repository %s: no repository sits inside another - put it in the projects folder (%s) as a project of its own",
		abs, enclosing, projects)
}

// within reports whether dir is root or lies under it.
func within(root, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dir))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// defaultProjectDir is where a universe's own project goes when none is named:
// <projects folder>/<name> - the folder the person chose for the installation's
// projects, else the one proposed (<Documents>/Katyoti).
func defaultProjectDir(name string) string {
	return cenv.CurrentInstallation().ProjectDir(name)
}
