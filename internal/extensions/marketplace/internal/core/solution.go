// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/soldocs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// InstallSolution installs a `kind: solution` (modules/katyoti/solutions.md §4):
// its vaults opened (one team password) and its config-sets' keys checked FIRST (a dry run stops
// there), then every pinned member installed at its exact version, and each of
// its universes — the main one into the universe named, each other one beside
// it as <main>-<its name> — created from its captured wiring and registered,
// the vaults' keys imported, its config-sets applied, its seed laid out, its
// members configured; the documents go into the main universe's project. The
// output names the main universe, the one that opens. No upgrade is taken.
func (s *service) InstallSolution(ctx context.Context, in mktapi.InstallSolutionInput) (mktapi.InstallSolutionOutput, error) {
	out := mktapi.InstallSolutionOutput{}
	ctx, st := beginSteps(ctx, 0)
	home, err := s.installSolution(ctx, in, &out, st)
	out.Home = home
	return out, err
}

func (s *service) installSolution(ctx context.Context, in mktapi.InstallSolutionInput, out *mktapi.InstallSolutionOutput, st *steps) (string, error) {
	it, _, catURL, err := s.findItem(ctx, in.Ref.Name, in.Ref.Marketplace)
	if err != nil {
		return "", err
	}
	if it.Kind != mktapi.KindSolution {
		return "", fmt.Errorf("%q is a %s item, not a solution", in.Ref.Name, it.Kind)
	}
	_, iv, err := pickVersion(it, in.Ref.Version)
	if err != nil {
		return "", err
	}
	if len(iv.Includes) == 0 {
		return "", fmt.Errorf("solution %q@%s declares no members", it.Name, iv.Version)
	}
	if err := iv.ValidateUniverses(); err != nil {
		return "", fmt.Errorf("solution %s@%s: %w", it.Name, iv.Version, err)
	}
	universes := iv.SolutionUniverses() // the main one first
	// A solution that brings configuration, keys or content goes into a
	// universe the person names — never one chosen for them.
	if strings.TrimSpace(in.UniverseHome) == "" && brings(iv) != "" {
		return "", fmt.Errorf("solution %s@%s brings %s: name the universe it goes into (--universe <name|folder>)", it.Name, iv.Version, brings(iv))
	}
	universeHome := universeTarget(in.UniverseHome)
	label := it.Name + "@" + iv.Version

	// 0. THE GATES — before anything installs.
	// The vaults — the solution's key sets: all opened with the one team
	// password (a vault's own password, when given, wins).
	given := map[string]string{}
	states, vaults, refused := s.openKeySets(ctx, iv.Vaults, func(name string) string {
		if p := in.VaultPasswords[name]; p != "" {
			return p
		}
		return in.KeySetsPassword
	})
	out.Vaults = states
	for _, v := range vaults {
		for _, e := range v.bundle.Keys {
			given[e.Name] = e.Value
		}
	}
	// A keys file given with the call.
	if len(in.Keys) > 0 {
		b, err := openKeys(in.Keys, in.KeysPassword)
		if err != nil {
			return universeHome, err
		}
		for _, e := range b.Keys {
			given[e.Name] = e.Value
		}
	}
	for k, v := range in.Values {
		given[k] = v
	}
	// Every universe's config-sets' keys against its target universe.
	var blocking []string
	for _, u := range universes {
		for _, cs := range u.ConfigSets {
			dry, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{
				Name: cs.Name, Version: cs.Version, UniverseHome: solutionUniverseHome(universeHome, u), Values: given, DryRun: true,
			})
			if err != nil {
				return universeHome, fmt.Errorf("solution %s: config-set %s: %w", label, cs.Name, err)
			}
			out.ConfigSets = append(out.ConfigSets, dry)
			blocking = append(blocking, dry.Blocking...)
		}
	}
	if in.DryRun {
		return universeHome, nil
	}
	if len(refused) > 0 {
		return universeHome, fmt.Errorf("solution %s: vault %s not opened (nothing was installed or created): give the team password", label, strings.Join(refused, ", "))
	}
	if len(blocking) > 0 {
		return universeHome, fmt.Errorf("solution %s needs %s (nothing was installed or created): give them (--set, --env-file, --keys, a vault) first", label, strings.Join(blocking, ", "))
	}
	out.ConfigSets = nil
	// The report: each member installed, then each universe made.
	st.setTotal(len(iv.Includes) + len(universes))

	// 1. Install every pinned member at its exact version.
	installed := map[string]mktapi.Installed{}
	var order []string
	for _, m := range iv.Includes {
		st.begin("Installing " + itemLabel(m.Name, m.Version))
		inst, err := s.Install(ctx, mktapi.ItemRef{Name: m.Name, Version: m.Version})
		if err != nil {
			return "", fmt.Errorf("solution %s: install member %s@%s: %w", label, m.Name, m.Version, err)
		}
		st.end("Installed " + itemLabel(m.Name, m.Version))
		if _, dup := installed[m.Name]; !dup {
			order = append(order, m.Name)
		}
		installed[m.Name] = inst
	}
	if strings.TrimSpace(universeHome) == "" {
		universeHome = defaultReleaseHome(it.Name)
	}
	// 2.–5. Each universe, the main one first: created from its captured
	// wiring and registered, the keys imported, its config-sets applied, its
	// seed laid out. NEVER the process's own workspace: this is served by the
	// ADMIN universe's hub.
	reports := map[string]*sidecarapi.ConfigCheckReport{}
	mainHome := ""
	for _, u := range universes {
		var members []mktapi.Installed
		var names []string
		for _, m := range iv.UniverseMembers(u) {
			members = append(members, installed[m.Name])
			names = append(names, m.Name)
		}
		target := solutionUniverseHome(universeHome, u)
		if name := filepath.Base(target); target != "" {
			st.begin("Making the universe " + name)
		} else {
			st.begin("Making the universe " + u.Name)
		}
		// Each universe records the solution that installed it: what it needs
		// (programs, key sets) is found again from the universe.
		disp := wire.Display{DisplayName: u.DisplayName, Icon: u.Icon, GUI: u.GUI, Tags: u.Tags, FirstGuide: u.FirstGuide, Solution: label}
		// The captured display name is the source universe's: kept only when the
		// target is that universe, else derived from the new name.
		if base := filepath.Base(target); !in.SolutionName && !strings.EqualFold(base, disp.DisplayName) {
			disp.DisplayName = titleCaseName(base)
		}
		home, err := wire.CreateUniverse(members, disp, target)
		if err != nil {
			return orMain(mainHome, home), err
		}
		if u.Main {
			mainHome = home
		}
		if regPath, perr := orchcatalog.DefaultRegistryPath(); perr == nil {
			if reg, rerr := orchcatalog.LoadRegistry(regPath); rerr == nil {
				_ = reg.Add(orchcatalog.RegistryEntry{Name: filepath.Base(home), HomePath: home})
			}
		}
		done := mktapi.InstalledUniverse{Name: registeredName(home), Home: home, Main: u.Main, Members: names}
		// 3. The keys: the vaults', then the file given, each into its scope.
		for _, v := range vaults {
			if _, err := s.ImportKeys(ctx, mktapi.ImportKeysInput{UniverseHome: home, File: v.file, Password: v.password}); err != nil {
				return mainHome, fmt.Errorf("solution %s: vault keys: %w", label, err)
			}
		}
		if len(in.Keys) > 0 {
			if _, err := s.ImportKeys(ctx, mktapi.ImportKeysInput{UniverseHome: home, File: in.Keys, Password: in.KeysPassword}); err != nil {
				return mainHome, fmt.Errorf("solution %s: keys: %w", label, err)
			}
		}
		// 4. Each config-set, in order, before the members are configured.
		for _, cs := range u.ConfigSets {
			applied, err := s.ApplyConfigSet(ctx, mktapi.ApplyConfigSetInput{
				Name: cs.Name, Version: cs.Version, UniverseHome: home, Values: in.Values,
			})
			if err != nil {
				return mainHome, fmt.Errorf("solution %s: apply config-set %s: %w", label, cs.Name, err)
			}
			out.ConfigSets = append(out.ConfigSets, applied)
		}
		// 5. The seed: its project laid out and attached, its tasks given (the
		// project folder asked for is the main universe's).
		if u.Seed != nil {
			projectDir := ""
			if u.Main {
				projectDir = in.ProjectDir
			}
			lo, err := s.layOutSeed(ctx, *u.Seed, home, projectDir, true)
			if err != nil {
				return mainHome, fmt.Errorf("solution %s: seed %s@%s: %w", label, u.Seed.Name, u.Seed.Version, err)
			}
			done.Project = lo.Project
			if u.Main {
				out.Project, out.ProjectKept, out.ProjectNotRecorded = lo.Project, lo.Kept, lo.NotRecorded
			}
		}
		// 7. Configure each member (materialize defaults) in its universe.
		for _, ins := range members {
			if rep, err := s.configureInstalled(ctx, home, ins); err == nil && reports[ins.Name] == nil {
				reports[ins.Name] = &rep
			}
		}
		if u.Main {
			out.Main = done.Name
		}
		out.Universes = append(out.Universes, done)
		st.end("Made the universe " + done.Name)
	}
	// 6. The documents, into the main universe's project (a solution without a
	// seed has no project to hold them: they stay readable in the marketplace).
	if out.Project != "" && len(iv.Documents) > 0 {
		if err := s.placeDocuments(ctx, iv.Documents, catURL, label, out); err != nil {
			return mainHome, err
		}
	}
	// The programs every member's check reports, planned once per member: the
	// plan is returned, never run here: the GUI (InstallPrereqs) or `katyoti
	// prereqs install` installs it (solutions.md §10).
	members := make([]mktapi.Installed, 0, len(order))
	memberReports := make([]*sidecarapi.ConfigCheckReport, 0, len(order))
	for _, name := range order {
		members = append(members, installed[name])
		memberReports = append(memberReports, reports[name])
	}
	plan := s.solutionPrereqs(members, memberReports)
	out.Prereqs = &plan
	return mainHome, nil
}

// placeDocuments fetches a solution's documents and places them into the
// main universe's project, through CWM on the project folder: no hub needs
// to serve it yet.
func (s *service) placeDocuments(ctx context.Context, docs []mktapi.Document, catURL, label string, out *mktapi.InstallSolutionOutput) error {
	placer, err := soldocs.Open(ctx, out.Project)
	if err != nil {
		return fmt.Errorf("solution %s: %w", label, err)
	}
	defer placer.Close()
	for _, d := range docs {
		data, err := store.FetchVerified(ctx, resolveAssetURL(catURL, d.URL), d.Sha256, label+" "+d.Lang+"/"+d.Name)
		if err != nil {
			return fmt.Errorf("solution %s: document %s/%s: %w", label, d.Lang, d.Name, err)
		}
		id, err := placer.Place(ctx, d.Lang, d.Name, data)
		if err != nil {
			return fmt.Errorf("solution %s: %w", label, err)
		}
		out.Documents = append(out.Documents, id)
	}
	return nil
}

// solutionUniverseHome is where a universe of a solution goes: the main one
// into the universe the person names (mainHome), each other one beside it,
// named <main>-<its name> — so one solution installs twice under two names,
// and a one-universe solution exactly where it always did.
func solutionUniverseHome(mainHome string, u mktapi.SolutionUniverse) string {
	if u.Main || strings.TrimSpace(mainHome) == "" {
		return mainHome
	}
	return filepath.Join(filepath.Dir(mainHome), filepath.Base(mainHome)+"-"+u.Name)
}

// orMain is the main universe's home once made, else home.
func orMain(mainHome, home string) string {
	if mainHome != "" {
		return mainHome
	}
	return home
}

// vaultFile is a vault's sealed bytes, installed into the store first.
func (s *service) vaultFile(ctx context.Context, v mktapi.PinnedRef) ([]byte, error) {
	inst, err := s.Install(ctx, mktapi.ItemRef{Name: v.Name, Version: v.Version})
	if err != nil {
		return nil, err
	}
	return os.ReadFile(inst.FolderPath)
}

// installVault fetches a vault into the store.
func (s *service) installVault(ctx context.Context, it mktapi.Item, mkt, catURL, version string) (mktapi.Installed, error) {
	ver, iv, err := pickVersion(it, version)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("marketplace %q: %w", mkt, err)
	}
	asset, err := pickAsset(iv)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("%s@%s: %w", it.Name, ver, err)
	}
	asset.URL = resolveAssetURL(catURL, asset.URL)
	path, err := store.InstallVault(ctx, it.Name, ver, asset)
	if err != nil {
		return mktapi.Installed{}, err
	}
	inst := mktapi.Installed{Name: it.Name, Kind: string(it.Kind), Version: ver, Source: "marketplace",
		Marketplace: mkt, Sha256: asset.Sha256, FolderPath: path}
	return inst, available.Record(inst)
}

// brings says what a solution brings beyond its members, in any of its
// universes.
func brings(iv mktapi.ItemVersion) string {
	var configSets, seed bool
	for _, u := range iv.SolutionUniverses() {
		configSets = configSets || len(u.ConfigSets) > 0
		seed = seed || u.Seed != nil
	}
	var parts []string
	if configSets {
		parts = append(parts, "config-sets")
	}
	if len(iv.Vaults) > 0 {
		parts = append(parts, "vaults")
	}
	if seed {
		parts = append(parts, "a seed (a project and its content)")
	}
	return strings.Join(parts, ", ")
}
