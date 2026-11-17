// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/memberversion"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/seed"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// SEEDS — a universe's content as an item of its own (`kind: seed`): the
// project of a universe (its CWM objects and the whole project folder) and
// the universe's tasks. Applied to a universe like a config-set — it adds a
// project — and pinned by a solution, whose install lays it out.

// ListSeeds returns the `kind: seed` items across enabled marketplaces.
func (s *service) ListSeeds(ctx context.Context) ([]mktapi.ItemSummary, error) {
	cats, err := s.allCatalogs(ctx)
	if err != nil {
		return nil, err
	}
	out := []mktapi.ItemSummary{}
	for _, c := range cats {
		for _, it := range c.cat.Items {
			if it.Kind != mktapi.KindSeed {
				continue
			}
			out = append(out, mktapi.ItemSummary{
				Name: it.Name, Kind: it.Kind, DisplayName: it.DisplayName, Description: it.Description,
				Marketplace: c.marketplace, Latest: latest(it), Demo: it.Demo,
			})
		}
	}
	return out, nil
}

// ShowSeed describes one seed: its versions and the members its latest
// version was made with.
func (s *service) ShowSeed(ctx context.Context, in mktapi.ShowSeedInput) (mktapi.ShowSeedOutput, error) {
	var out mktapi.ShowSeedOutput
	it, mkt, _, err := s.findItem(ctx, in.Name, "")
	if err != nil {
		return out, err
	}
	if it.Kind != mktapi.KindSeed {
		return out, fmt.Errorf("%s is a %s item, not a seed", in.Name, it.Kind)
	}
	out.Name, out.DisplayName, out.Description, out.Marketplace, out.Demo = it.Name, it.DisplayName, it.Description, mkt, it.Demo
	for _, v := range it.Versions {
		out.Versions = append(out.Versions, v.Version)
	}
	if len(it.Versions) > 0 {
		for _, m := range it.Versions[0].Requires.Members {
			out.Requires = append(out.Requires, m.Name+"@"+m.Version)
		}
	}
	return out, nil
}

// installSeed fetches, verifies and unpacks a seed into the store, and records
// it as available.
func (s *service) installSeed(ctx context.Context, it mktapi.Item, mkt, catURL, version string) (mktapi.Installed, error) {
	ver, iv, err := pickVersion(it, version)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("marketplace %q: %w", mkt, err)
	}
	asset, err := pickAsset(iv)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("%s@%s: %w", it.Name, ver, err)
	}
	asset.URL = resolveAssetURL(catURL, asset.URL)
	folder, err := store.InstallSeed(ctx, it.Name, ver, asset)
	if err != nil {
		return mktapi.Installed{}, err
	}
	inst := mktapi.Installed{Name: it.Name, Kind: string(it.Kind), Version: ver, Source: "marketplace",
		Marketplace: mkt, Sha256: asset.Sha256, FolderPath: folder}
	return inst, available.Record(inst)
}

// ApplySeed lays a seed out into an existing universe.
func (s *service) ApplySeed(ctx context.Context, in mktapi.ApplySeedInput) (mktapi.ApplySeedOutput, error) {
	out := mktapi.ApplySeedOutput{Name: in.Name}
	home := universeTarget(in.UniverseHome)
	if home == "" {
		return out, fmt.Errorf("name the universe the seed goes into (--universe <name|folder>)")
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")); err != nil {
		return out, fmt.Errorf("%s is not a universe yet: create it (or install a solution that pins this seed) first", home)
	}
	lo, err := s.layOutSeed(ctx, mktapi.PinnedRef{Name: in.Name, Version: in.Version}, home, in.ProjectDir, false)
	out.Version, out.UniverseHome, out.Project, out.ProjectID, out.Tasks, out.Differs = lo.Version, home, lo.Project, lo.ProjectID, lo.Tasks, lo.Differs
	out.NotRecorded = lo.NotRecorded
	return out, err
}

// laidOut is what layOutSeed did.
type laidOut struct {
	Version, Project, ProjectID string
	Kept                        bool
	Tasks                       int
	// NotRecorded is why the project laid out could not be recorded as
	// created on this PC; empty when it was, or was kept.
	NotRecorded string
	Differs     []string
	// recorded is whether this call recorded the project as created on this
	// PC - forgotten again when the universe refuses it.
	recorded bool
}

// layOutSeed is the one path a seed takes into a universe (apply, bootstrap):
// the members it was made with checked FIRST — a missing one refuses before
// anything is written, a different version is reported — then the seed
// installed (verified), its project laid out into projectDir (default
// <universe name>-project beside the universe) with its identity re-minted,
// its tasks given to the universe, the project attached. A project folder that
// already holds files is refused, or kept as it is when keepExisting (a
// solution installed again repairs its universe).
func (s *service) layOutSeed(ctx context.Context, ref mktapi.PinnedRef, home, projectDir string, keepExisting bool) (laidOut, error) {
	var lo laidOut
	it, mkt, catURL, err := s.findItem(ctx, ref.Name, "")
	if err != nil {
		return lo, err
	}
	if it.Kind != mktapi.KindSeed {
		return lo, fmt.Errorf("%s is a %s item, not a seed", ref.Name, it.Kind)
	}
	ver, iv, err := pickVersion(it, ref.Version)
	if err != nil {
		return lo, err
	}
	lo.Version = ver
	missing, differs := memberGaps(home, iv.Requires.Members)
	lo.Differs = differs
	if len(missing) > 0 {
		return lo, fmt.Errorf("%s@%s was made with %s, which the universe does not have: wire them first (or apply the config-set or release that brings them)",
			it.Name, ver, strings.Join(missing, ", "))
	}
	name := registeredName(home)
	// The project goes to the installation's projects folder, under the
	// universe's name - never beside the universe, inside the installation's
	// data, where an uninstall or a reinstall would take it.
	owner := cenv.CurrentInstallation()
	if strings.TrimSpace(projectDir) == "" {
		projectDir = owner.ProjectDir(name)
	}
	if err := owner.CheckProjectDir(projectDir); err != nil {
		return lo, err
	}
	lo.Project = projectDir
	// A universe attaches several projects: the seed's joins
	// the members it serves already, under the attach's member rules - the
	// orchestrator's, applied by attachProject below. A project they refuse
	// is taken back: nothing this call laid out stays (as CreateProject
	// removes the folder it made when its attach is refused).
	_, statErr := os.Stat(projectDir)
	existed := statErr == nil
	if entries, err := os.ReadDir(projectDir); err == nil && len(entries) > 0 {
		if !keepExisting {
			return lo, fmt.Errorf("%s already holds files: a seed never overwrites a project — name another folder (--project)", projectDir)
		}
		lo.Kept = true
	}
	inst, err := s.installSeed(ctx, it, mkt, catURL, ver)
	if err != nil {
		return lo, err
	}
	a, err := seed.Apply(inst.FolderPath, projectDir, filepath.Join(application.EnvOf(home).HomeDir(), "tasks"))
	if err != nil {
		return lo, err
	}
	lo.Tasks = a.Tasks
	if err := s.joinSeeded(ctx, &lo, a, home, name, projectDir); err != nil {
		if !lo.Kept {
			if lo.recorded {
				forgetCreated(ctx, projectDir)
			}
			takeBack(a, projectDir, existed)
			lo.Tasks, lo.ProjectID, lo.NotRecorded = 0, "", ""
		}
		return lo, err
	}
	return lo, nil
}

// joinSeeded gives a project just laid out from a seed its identity and the
// seed of its project object, records it as created on this PC and attaches
// it to the universe - the attach applies the member rules.
func (s *service) joinSeeded(ctx context.Context, lo *laidOut, a seed.Applied, home, name, projectDir string) error {
	var err error
	if !a.ProjectKept {
		if lo.ProjectID, err = seed.Remint(projectDir); err != nil {
			return fmt.Errorf("give the project its own identity: %w", err)
		}
	}
	// The project object's seed the seed carries goes into the new project's
	// store: CWM mints the project object from it at the first opening, with
	// its own identity.
	if a.ProjectSeed != "" {
		store := application.EnvOf(projectDir).HomeDir()
		if err := os.MkdirAll(store, 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(a.ProjectSeed)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(store, cwmapi.ProjectSeedFile), data, 0o644); err != nil {
			return fmt.Errorf("the project object's seed: %w", err)
		}
	}
	// Laid out here, it is created on this PC: recorded
	// through the orchestrator, the record's one writer, before it is
	// attached - never a kept folder, which may be a clone. A failure to
	// record is said, never fatal: only a later attach beside other members
	// would be refused.
	if !lo.Kept {
		lo.NotRecorded = recordCreated(ctx, projectDir)
		lo.recorded = lo.NotRecorded == ""
	}
	if err := attachProject(ctx, home, name, projectDir); err != nil {
		return fmt.Errorf("%s does not join universe %s: %w", projectDir, home, err)
	}
	return nil
}

// takeBack undoes what a seed laid out for a project its universe refused:
// the task files it wrote and the project folder - emptied when it existed
// (empty) before, removed when the seed made it.
func takeBack(a seed.Applied, projectDir string, existed bool) {
	for _, f := range a.TaskFiles {
		_ = os.Remove(f)
	}
	if !existed {
		_ = os.RemoveAll(projectDir)
		return
	}
	entries, _ := os.ReadDir(projectDir)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(projectDir, e.Name()))
	}
}

// recordCreated records projectDir as created on this PC through the
// orchestrator; the reason it could not, or "".
func recordCreated(ctx context.Context, projectDir string) string {
	orch, err := application.UseService[orchapi.OrchestratorService](ctx)
	if err != nil {
		return "the universe service is not here: " + err.Error()
	}
	if _, err := orch.RecordCreatedProject(ctx, orchapi.RecordCreatedProjectInput{ProjectDir: projectDir}); err != nil {
		return err.Error()
	}
	return ""
}

// forgetCreated drops projectDir from the creation record through the
// orchestrator - the other half of recordCreated, for a project its universe
// refused and that is taken back: the record never names a place a later
// clone could fill (as the setup's sample does). A failure
// to forget is not fatal: the refusal is what the caller hears.
func forgetCreated(ctx context.Context, projectDir string) {
	orch, err := application.UseService[orchapi.OrchestratorService](ctx)
	if err != nil {
		return
	}
	_, _ = orch.ForgetCreatedProject(ctx, orchapi.ForgetCreatedProjectInput{ProjectDir: projectDir})
}

func sameFolder(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// memberGaps compares a universe's members with the ones a seed was made
// with: the missing names, and the members wired at another version
// ("name: have → seed's"). A member whose version cannot be read (not wired
// from the store) is present, not different.
func memberGaps(home string, want []mktapi.PinnedRef) (missing, differs []string) {
	have := map[string]string{}
	if m, err := orchcatalog.LoadManifest(home); err == nil && m != nil {
		for _, sc := range m.Sidecars {
			have[sc.Name] = memberversion.Derive(sc)
		}
	}
	for _, w := range want {
		v, ok := have[w.Name]
		switch {
		case !ok:
			missing = append(missing, w.Name)
		case v != "" && w.Version != "" && v != w.Version:
			differs = append(differs, fmt.Sprintf("%s: %s → %s", w.Name, v, w.Version))
		}
	}
	return missing, differs
}

// attachProject attaches a project to a universe through the universe
// service — the registry's one writer for what it serves: a universe it does
// not know yet (registered on disk a moment ago, after it read the registry)
// is added through it first.
func attachProject(ctx context.Context, home, name, projectDir string) error {
	orch, err := application.UseService[orchapi.OrchestratorService](ctx)
	if err != nil {
		return fmt.Errorf("attach the project: the universe service is not here: %w", err)
	}
	if _, err := orch.GetUniverse(ctx, orchapi.GetUniverseInput{Name: name}); err != nil {
		info, err := orch.AddExistingUniverse(ctx, orchapi.AddExistingUniverseInput{Path: home})
		if err != nil {
			return fmt.Errorf("register %s: %w", home, err)
		}
		// Registered under the name its manifest gives — the one to attach to.
		if info.Summary.Name != "" {
			name = info.Summary.Name
		}
	}
	if _, err := orch.AttachProject(ctx, orchapi.AttachProjectInput{ProjectDir: projectDir, Universe: name}); err != nil {
		return fmt.Errorf("attach %s to %s: %w", projectDir, name, err)
	}
	return nil
}
