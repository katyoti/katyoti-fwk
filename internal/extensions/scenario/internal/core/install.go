// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// INSTALL — the case set up from scratch in its runtime folder: the sidecars'
// binaries copied into its bin folder (its processes are then its own), a
// universe at <case>/universe with them as members and its data kept at
// <case>/data, the case's config files, the simulated user's task, and the
// project created from the case's folder (a git repository when the case says
// so), attached to the universe. Then the first checkpoint: `installed`.

// Install sets the case up; force removes a previous install first.
func (e *Engine) Install(ctx context.Context, force bool) error {
	orch, err := application.UseService[orchapi.OrchestratorService](ctx)
	if err != nil {
		return fmt.Errorf("the orchestrator service: %w", err)
	}
	name := e.Case.UniverseName()
	if e.installedBefore() {
		if !force {
			return fmt.Errorf("%s exists — pass --force to reinstall the case from scratch", e.RT.Dir)
		}
		_ = e.Stop(ctx)
	}
	if _, err := orch.GetUniverse(ctx, orchapi.GetUniverseInput{Name: name}); err == nil {
		if !force {
			return fmt.Errorf("universe %s exists — pass --force to reinstall", name)
		}
		if _, err := orch.DeleteUniverse(ctx, orchapi.DeleteUniverseInput{Name: name}); err != nil {
			return fmt.Errorf("remove the old universe: %w", err)
		}
	}
	for _, o := range e.Case.OtherUniverses {
		other := e.Case.OtherUniverseName(o.Name)
		if _, err := orch.GetUniverse(ctx, orchapi.GetUniverseInput{Name: other}); err == nil {
			if !force {
				return fmt.Errorf("universe %s exists — pass --force to reinstall", other)
			}
			if _, err := orch.DeleteUniverse(ctx, orchapi.DeleteUniverseInput{Name: other}); err != nil {
				return fmt.Errorf("remove the old universe %s: %w", other, err)
			}
		}
	}
	if err := e.clearRuntime(); err != nil {
		return err
	}
	for _, d := range []string{e.RT.Bin(), e.RT.Data(), e.RT.Runs(), e.RT.Checkpoints()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	if err := e.copyBinaries(); err != nil {
		return err
	}
	if err := e.stampInstall(); err != nil {
		return err
	}
	// The sandbox's machine first: the universe is registered in its registry.
	if err := e.ensureMachine(); err != nil {
		return err
	}

	color := e.Case.Universe.Color
	if color == "" {
		color = "grape"
	}
	if _, err := orch.CreateUniverse(ctx, caseUniverseInput(*e.Case, name, color, e.RT.Universe())); err != nil {
		return fmt.Errorf("create universe %s: %w", name, err)
	}
	for _, s := range e.Case.Universe.Sidecars {
		if _, err := orch.AddSidecar(ctx, orchapi.AddSidecarInput{Universe: name,
			Sidecar: orchapi.CreateSidecarSelection{Name: s, BinaryPath: filepath.Join(e.RT.Bin(), exe(s))}}); err != nil {
			return fmt.Errorf("add member %s: %w", s, err)
		}
	}
	e.logf("universe %s created at %s with %s", name, e.RT.Universe(), strings.Join(e.Case.Universe.Sidecars, ", "))
	if err := setUniverseDisplay(e.RT.Universe(), e.Case.Universe.Tags, e.Case.Universe.GUI, e.Case.Universe.FirstGuide); err != nil {
		return fmt.Errorf("universe %s: %w", name, err)
	}
	if err := setUniverseSolution(e.RT.Universe(), e.Case.Universe.Solution); err != nil {
		return fmt.Errorf("universe %s: %w", name, err)
	}

	// What the case copies in (config, keys, tasks), then its data kept in the
	// case whoever starts it, the GUI included (after: a copied Settings.yaml
	// keeps its other settings).
	for i, cp := range e.Case.Universe.Copy {
		n, err := e.copyIn(cp)
		if err != nil {
			return fmt.Errorf("universe.copy %d (%s): %w", i+1, cp.From, err)
		}
		e.logf("copied %d file(s) from %s to %s", n, cp.From, "universe/"+filepath.ToSlash(cp.To))
	}
	if err := setDataDir(e.RT.UniverseConfig(), e.RT.Data()); err != nil {
		return err
	}
	// Every member's bundled defaults (Plugins.yaml, …) — what an operator's
	// `sc config set` writes; a file the case copied is kept.
	for _, m := range e.Case.Universe.Sidecars {
		if err := e.memberDefaults(m, e.RT.Universe(), e.RT.Data()); err != nil {
			return err
		}
	}
	// The solution's other universes, beside it: set up, never started.
	for _, o := range e.Case.OtherUniverses {
		if err := e.installOther(ctx, orch, o, color); err != nil {
			return err
		}
	}
	// The simulated user's task, served by the universe's ktw-agents.
	tasks := filepath.Join(e.RT.UniverseHome(), "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tasks, SimUserTask+".md"), simUserTaskMD, 0o644); err != nil {
		return err
	}

	if err := e.installProjects(ctx, orch, name); err != nil {
		return err
	}

	if err := saveState(e.RT, State{Installed: true, Saved: map[string]any{}}); err != nil {
		return err
	}
	if e.CasesDir != "" {
		if path, err := e.WriteLauncher(e.CasesDir); err != nil {
			e.logf("no GUI launcher: %v", err)
		} else {
			e.logf("GUI launcher: %s (double-click to open the case in ktw-gui)", path)
		}
	}
	return e.Checkpoint(ctx, CheckpointInstalled)
}

// installProjects creates the case's project from its folder and attaches it
// to the universe, then each of its other repositories beside it
// : laid out under repositories/<name>, recorded as created on
// this PC before its attach - a repository joins beside other members only so
// - and forgotten again when its attach is refused. A git
// project or repository is initialised and committed `installed`.
func (e *Engine) installProjects(ctx context.Context, orch orchapi.OrchestratorService, universe string) error {
	if err := os.MkdirAll(e.RT.Project(), 0o755); err != nil {
		return err
	}
	if e.Case.Project.From != "" {
		if err := copyTree(filepath.Join(e.Case.Dir, e.Case.Project.From), e.RT.Project()); err != nil {
			return fmt.Errorf("project content: %w", err)
		}
	}
	if e.Case.Project.Live != "" {
		if err := e.seedLive(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(e.RT.Project(), "modules"), 0o755); err != nil {
		return err
	}
	if err := seedProjectObject(e.RT.Project(), e.Case.Project); err != nil {
		return fmt.Errorf("the project's seed: %w", err)
	}
	if _, err := orch.AttachProject(ctx, orchapi.AttachProjectInput{ProjectDir: e.RT.Project(), Universe: universe}); err != nil {
		return fmt.Errorf("attach the project: %w", err)
	}
	if e.Case.Project.Git {
		if err := gitInit(e.RT.Project()); err != nil {
			return err
		}
	}
	e.logf("project created at %s and attached", e.RT.Project())

	for _, r := range e.Case.Repositories {
		dir := e.RT.Repository(r.Name)
		if err := os.MkdirAll(filepath.Join(dir, "modules"), 0o755); err != nil {
			return err
		}
		if r.From != "" {
			if err := copyTree(filepath.Join(e.Case.Dir, r.From), dir); err != nil {
				return fmt.Errorf("repository %s content: %w", r.Name, err)
			}
		}
		if err := seedProjectObject(dir, api.CaseProject{Name: r.Name, Type: r.Type, Title: r.Title}); err != nil {
			return fmt.Errorf("repository %s seed: %w", r.Name, err)
		}
		if _, err := orch.RecordCreatedProject(ctx, orchapi.RecordCreatedProjectInput{ProjectDir: dir}); err != nil {
			return fmt.Errorf("repository %s: record it as created here: %w", r.Name, err)
		}
		if _, err := orch.AttachProject(ctx, orchapi.AttachProjectInput{ProjectDir: dir, Universe: universe}); err != nil {
			_, _ = orch.ForgetCreatedProject(ctx, orchapi.ForgetCreatedProjectInput{ProjectDir: dir})
			return fmt.Errorf("attach the repository %s: %w", r.Name, err)
		}
		if r.Git {
			if err := gitInit(dir); err != nil {
				return err
			}
		}
		e.logf("repository %s created at %s and attached beside the project", r.Name, dir)
	}
	return nil
}

// gitInit makes dir a git repository and commits it `installed`.
func gitInit(dir string) error {
	if err := gitIn(dir, "init", "-q"); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".katyoti/*.lock\n.katyoti/cwm/index*\nnode_modules/\n"), 0o644)
	commitIn(dir, "installed")
	return nil
}

// installOther sets up one of the case's other universes beside its own, as
// its own is: created and registered with its members (the case's bin/
// copies), its copies run, its data kept in the case, each member's bundled
// defaults written. Nothing is started and no project is attached: its
// project is the case's folder, published as its seed.
func (e *Engine) installOther(ctx context.Context, orch orchapi.OrchestratorService, o api.CaseOtherUniverse, color string) error {
	name, home := e.Case.OtherUniverseName(o.Name), e.RT.OtherUniverse(o.Name)
	if o.Color != "" {
		color = o.Color
	}
	if _, err := orch.CreateUniverse(ctx, orchapi.CreateUniverseInput{
		Name: name, DisplayName: "Scenario " + e.Case.Name + " " + o.Name, Icon: "S", Color: color, HomePath: home,
	}); err != nil {
		return fmt.Errorf("create universe %s: %w", name, err)
	}
	for _, s := range o.Sidecars {
		if _, err := orch.AddSidecar(ctx, orchapi.AddSidecarInput{Universe: name,
			Sidecar: orchapi.CreateSidecarSelection{Name: s, BinaryPath: filepath.Join(e.RT.Bin(), exe(s))}}); err != nil {
			return fmt.Errorf("universe %s: add member %s: %w", name, s, err)
		}
	}
	if err := setUniverseDisplay(home, o.Tags, o.GUI, ""); err != nil {
		return fmt.Errorf("universe %s: %w", name, err)
	}
	env := application.EnvOf(home)
	for i, cp := range o.Copy {
		n, err := e.copyInto(cp, env.HomeDir())
		if err != nil {
			return fmt.Errorf("other_universes %s copy %d (%s): %w", o.Name, i+1, cp.From, err)
		}
		e.logf("copied %d file(s) from %s to %s", n, cp.From, "universes/"+o.Name+"/"+filepath.ToSlash(cp.To))
	}
	if err := setDataDir(env.ConfigDir(), e.RT.OtherData(o.Name)); err != nil {
		return err
	}
	for _, m := range o.Sidecars {
		if err := e.memberDefaults(m, home, e.RT.OtherData(o.Name)); err != nil {
			return err
		}
	}
	e.logf("universe %s created at %s with %s", name, home, strings.Join(o.Sidecars, ", "))
	return nil
}

// clearRuntime removes everything of a previous install but the sandbox's
// machine: the command runs inside it (its own logs are there), and the old
// universe left its registry above.
// installedBefore reports whether the case was installed in its runtime
// folder: anything there beside its sandbox's machine folder, which the
// command running the install made first (the same reading as clearRuntime).
func (e *Engine) installedBefore() bool {
	entries, _ := os.ReadDir(e.RT.Dir)
	machine := filepath.Base(e.Sandbox().Machine())
	for _, en := range entries {
		if en.Name() != machine {
			return true
		}
	}
	return false
}

func (e *Engine) clearRuntime() error {
	entries, err := os.ReadDir(e.RT.Dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	machine := filepath.Base(e.Sandbox().Machine())
	for _, en := range entries {
		if en.Name() == machine {
			continue
		}
		if err := os.RemoveAll(filepath.Join(e.RT.Dir, en.Name())); err != nil {
			return fmt.Errorf("clear %s: %w", e.RT.Dir, err)
		}
	}
	return nil
}

// memberDefaults writes a member's bundled config defaults into a universe of
// the case (`<member> sc config set`, run on that universe, its data given).
func (e *Engine) memberDefaults(member, universe, data string) error {
	cmd := exec.Command(filepath.Join(e.RT.Bin(), exe(member)), "sc", "config", "set", "--out", "raw")
	cmd.Dir = universe
	cmd.Env = caseProcessEnv(universe, data)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s defaults: %w: %s", member, err, out)
	}
	e.logf("%s: its bundled config defaults written", member)
	return nil
}

// copyBinaries copies the hub and every member from the bin folder, so the
// case's processes are its own.
func (e *Engine) copyBinaries() error {
	for _, b := range e.caseBinaries() {
		if err := copyFile(filepath.Join(e.BinSrc, exe(b)), filepath.Join(e.RT.Bin(), exe(b))); err != nil {
			return fmt.Errorf("binary %s: %w (build it, or pass --bin)", b, err)
		}
	}
	// The GUI and ktw-univers too, when the folder has them: the launcher
	// opens the case with the build its universe runs, and the sandbox's admin
	// universe is served by the case's own ktw-univers.
	for _, b := range []string{guiBinaryName, universBinary} {
		if src := filepath.Join(e.BinSrc, exe(b)); fileExists(src) {
			if err := copyFile(src, filepath.Join(e.RT.Bin(), exe(b))); err != nil {
				return fmt.Errorf("binary %s: %w", b, err)
			}
		}
	}
	e.logf("binaries copied from %s", e.BinSrc)
	// Where they came from, beside them: a later refresh takes the same ones.
	return os.WriteFile(filepath.Join(e.RT.Bin(), binSourceFile), []byte(e.BinSrc), 0o644)
}

// caseBinaries are the hub and every member of every universe of the case,
// once.
func (e *Engine) caseBinaries() []string {
	out := []string{hubBinary}
	seen := map[string]bool{hubBinary: true}
	add := func(names []string) {
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	add(e.Case.Universe.Sidecars)
	for _, o := range e.Case.OtherUniverses {
		add(o.Sidecars)
	}
	return out
}

// binSourceFile records, in the case's bin folder, the folder its binaries
// are copied from.
const binSourceFile = "SOURCE"

// BinSource is the folder an installed case's binaries come from; "" when it
// was never installed.
func BinSource(r Runtime) string {
	b, err := os.ReadFile(filepath.Join(r.Bin(), binSourceFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// RefreshBinaries stops the case's universe and copies its binaries again —
// after a fix was built. The binaries are no part of a checkpoint: a restore
// keeps the fixed ones.
func (e *Engine) RefreshBinaries(ctx context.Context) error {
	if err := e.Stop(ctx); err != nil {
		return err
	}
	if err := e.copyBinaries(); err != nil {
		return err
	}
	if err := e.stampBinaries(); err != nil {
		return err
	}
	return e.ensureMachine()
}

// setDataDir writes data_dir into the universe's Settings.yaml, keeping the
// rest of it.
func setDataDir(configDir, dataDir string) error {
	p := filepath.Join(configDir, "Settings.yaml")
	old, _ := os.ReadFile(p)
	var kept []string
	for _, l := range strings.Split(strings.ReplaceAll(string(old), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "data_dir:") || strings.TrimSpace(l) == "{}" {
			continue
		}
		if l != "" {
			kept = append(kept, l)
		}
	}
	kept = append(kept, fmt.Sprintf("data_dir: %q", filepath.ToSlash(dataDir)))
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}

// gitIn runs git in dir.
func gitIn(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// commit records the state of the project and of each other repository after
// a stage, each git one in its own history (nothing to commit is fine).
func (e *Engine) commit(msg string) {
	if e.Case.Project.Git {
		commitIn(e.RT.Project(), msg)
	}
	for _, r := range e.Case.Repositories {
		if r.Git {
			commitIn(e.RT.Repository(r.Name), msg)
		}
	}
}

// commitIn commits everything in the repository dir.
func commitIn(dir, msg string) {
	_ = gitIn(dir, "add", "-A")
	_ = gitIn(dir, "-c", "user.name=scenario", "-c", "user.email=scenario@example.com", "commit", "-q", "--allow-empty", "-m", "scenario: "+msg)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// seedProjectObject leaves the project object the case declares (its type,
// title and id) as the project's seed in its store, where CWM reads it at the
// first opening and mints the project object from it with its own identity
// - the same seed a typed project the hub creates gets; its id is
// the project's key among the universe's members from the start
// (cwmapi.ProjectKeyOf). A project that already carries its project object, or
// a case that declares neither type nor id, gets none: CWM gives it the
// default object, named after its folder.
func seedProjectObject(project string, p api.CaseProject) error {
	typ, name := strings.TrimSpace(p.Type), strings.TrimSpace(p.Name)
	if typ == "" && name == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(project, projectfile.FileName)); err == nil {
		return nil
	}
	data, err := marshal.Marshal(cwmapi.ProjectSeed{Name: name, Title: strings.TrimSpace(p.Title), Type: typ}, marshal.YAML)
	if err != nil {
		return err
	}
	store := application.EnvOf(project).HomeDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(store, cwmapi.ProjectSeedFile), data, 0o644)
}

// copyTree copies a folder's content into dst (files overwrite).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

// setUniverseDisplay gives a case's universe the tags, the GUI and the first
// guide the case declares, as its solution's install writes them into
// Univers.yaml - so a live test sees the universe a solution gets
// . Written once its members are there: a tag is one a member
// declares. Nothing declared: Univers.yaml is left as it is.
func setUniverseDisplay(home string, tags []string, gui *cenv.UniverseGUI, firstGuide string) error {
	if len(tags) == 0 && gui == nil && firstGuide == "" {
		return nil
	}
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		return err
	}
	if len(tags) > 0 {
		m.Tags = append([]string(nil), tags...)
	}
	if gui != nil {
		m.GUI = gui
	}
	if firstGuide != "" {
		m.FirstGuide = firstGuide
	}
	return orchcatalog.SaveManifest(home, m)
}

// setUniverseSolution records the solution a case's universe stands for in
// its Univers.yaml, as a solution's install records the one that installed
// it: a project whose type needs that solution then opens.
// None declared: Univers.yaml is left as it is.
func setUniverseSolution(home, solution string) error {
	solution = strings.TrimSpace(solution)
	if solution == "" {
		return nil
	}
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		return err
	}
	m.Solution = solution
	return orchcatalog.SaveManifest(home, m)
}

// caseUniverseInput is how a case's universe is created: « Scenario <case> »,
// in its sandbox home, its welcome screens off when the case says so
func caseUniverseInput(c api.Case, name, color, home string) orchapi.CreateUniverseInput {
	return orchapi.CreateUniverseInput{
		Name: name, DisplayName: "Scenario " + c.Name, Icon: "S", Color: color, HomePath: home,
		NoWelcome: c.Universe.NoWelcome,
	}
}
