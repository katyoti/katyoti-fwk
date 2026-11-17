// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// SnapshotOptions publish a case as a marketplace release.
type SnapshotOptions struct {
	// Checkpoint is the checkpoint whose folders are published; empty: the
	// case as it stands.
	Checkpoint string
	// Version and Marketplace (the catalog file) are required.
	Version, Marketplace string
	// Name of the solution (default: the case's name); SeedName of its seed
	// (default: <name>-seed).
	Name, SeedName string
	// ConfigSets and Vaults it pins (name@version); Docs its documents folder
	// (<lang>/*.md); WhatsNew a change log made into « Quoi de neuf ».
	ConfigSets, Vaults []string
	Docs, WhatsNew     string
	// LetterCovers are the change logs a written « Quoi de neuf » covers
	// (empty: WhatsNew alone) -.
	LetterCovers []string
	// LetterRequests is how many requests their changes name (0: unknown).
	LetterRequests int
	// OtherConfigSets are the config-sets each of the case's other universes
	// applies (name@version), by its name.
	OtherConfigSets map[string][]string
	// SecretKey re-signs a signed catalog.
	SecretKey string
}

// Snapshot publishes the case — its universe's members, its project and its
// tasks, from a checkpoint or as it stands — as a release with its seed: the
// same snapshot as `snapshot-universe`, read from the case's folders (nothing
// restored, nothing started). The members run copies in the case's bin/, so
// each is pinned by content: the catalog must publish those very bytes. The
// case's universe is the solution's main one; each of its other universes is
// published beside it, with its display, its config-sets and its project as
// its seed.
func (e *Engine) Snapshot(o SnapshotOptions) (authoring.AddSolutionResult, error) {
	if len(e.Case.Repositories) > 0 {
		// A solution's seed carries one project per universe: the case's
		// other repositories would be dropped without a word.
		return authoring.AddSolutionResult{}, fmt.Errorf("case %s serves other repositories beside its project (repositories:): a solution's seed carries one project per universe, so it is not published", e.Case.Name)
	}
	root := e.RT.Dir
	if o.Checkpoint != "" {
		root = filepath.Join(e.RT.Checkpoints(), o.Checkpoint)
		if _, err := os.Stat(root); err != nil {
			return authoring.AddSolutionResult{}, fmt.Errorf("no checkpoint %q (have %v)", o.Checkpoint, e.Checkpoints())
		}
	}
	rt := Runtime{Dir: root}
	if _, err := os.Stat(rt.Universe()); err != nil {
		return authoring.AddSolutionResult{}, fmt.Errorf("case %s has no universe at %s: install it first", e.Case.Name, rt.Universe())
	}
	name := o.Name
	if name == "" {
		name = e.Case.Name
	}
	// Its documents: the case's docs/ folder unless another is given.
	if o.Docs == "" && e.Case.Dir != "" && exists(filepath.Join(e.Case.Dir, "docs")) {
		o.Docs = filepath.Join(e.Case.Dir, "docs")
	}
	sd := authoring.SolutionSeed{Name: o.SeedName, Demo: e.Case.Project.Demo}
	if _, err := os.Stat(rt.Project()); err == nil {
		sd.Project = rt.Project()
		if tasks := filepath.Join(application.EnvOf(rt.Universe()).HomeDir(), "tasks"); exists(tasks) {
			// The universe's own tasks, never the simulated user's: the tool
			// installs it into every case, it is no part of the solution.
			own, cleanup, err := withoutSimUser(tasks)
			if err != nil {
				return authoring.AddSolutionResult{}, err
			}
			defer cleanup()
			sd.Tasks = own
		}
	}
	// Its universe's name: the case's own choice, else its name — never the
	// sandbox's « Scenario <case> ».
	displayName := e.Case.Universe.DisplayName
	if displayName == "" {
		displayName = e.Case.Name
	}
	others, cleanup, err := e.otherUniverses(rt, o.OtherConfigSets)
	defer cleanup()
	if err != nil {
		return authoring.AddSolutionResult{}, err
	}
	spec := solutionSpec(o)
	spec.DisplayName, spec.Icon, spec.Description = displayName, e.Case.Universe.Icon, e.Case.Title
	spec.GUI, spec.Tags, spec.FirstGuide, spec.Others = e.Case.Universe.GUI, e.Case.Universe.Tags, e.Case.Universe.FirstGuide, others
	return authoring.New().AddSolution(rt.Universe(), o.Version, name, o.Marketplace, o.SecretKey, sd, spec)
}

// solutionSpec is what the snapshot's options give the solution: its config
// sets, vaults, docs, its « Quoi de neuf » and the change logs that letter
// covers - the case's own fields are set by the caller.
func solutionSpec(o SnapshotOptions) authoring.SolutionSpec {
	return authoring.SolutionSpec{ConfigSets: o.ConfigSets, Vaults: o.Vaults, Docs: o.Docs, WhatsNew: o.WhatsNew, LetterCovers: o.LetterCovers, LetterRequests: o.LetterRequests}
}

// otherUniverses are the case's other universes as the solution captures
// them, read from rt (the case or a checkpoint): each from its home, its
// display the case's (else its name), its config-sets those given for it,
// its seed its project folder and its own tasks. A config-set given for a
// universe the case does not declare is refused.
func (e *Engine) otherUniverses(rt Runtime, configSets map[string][]string) ([]authoring.OtherUniverse, func(), error) {
	var cleanups []func()
	cleanup := func() {
		for _, c := range cleanups {
			c()
		}
	}
	declared := map[string]bool{}
	var out []authoring.OtherUniverse
	for _, c := range e.Case.OtherUniverses {
		declared[c.Name] = true
		home := rt.OtherUniverse(c.Name)
		if _, err := os.Stat(home); err != nil {
			return nil, cleanup, fmt.Errorf("case %s has no universe %s at %s: install it again (--force)", e.Case.Name, c.Name, home)
		}
		display := c.DisplayName
		if display == "" {
			display = c.Name
		}
		o := authoring.OtherUniverse{Name: c.Name, Home: home, ConfigSets: configSets[c.Name],
			DisplayName: display, Icon: c.Icon, GUI: c.GUI, Tags: c.Tags}
		if c.Project.From != "" {
			o.Seed.Project = filepath.Join(e.Case.Dir, c.Project.From)
			if tasks := filepath.Join(application.EnvOf(home).HomeDir(), "tasks"); exists(tasks) {
				own, done, err := withoutSimUser(tasks)
				cleanups = append(cleanups, done)
				if err != nil {
					return nil, cleanup, err
				}
				o.Seed.Tasks = own
			}
		}
		out = append(out, o)
	}
	for n := range configSets {
		if !declared[n] {
			return nil, cleanup, fmt.Errorf("config-sets given for the universe %s, which the case %s does not declare (other_universes)", n, e.Case.Name)
		}
	}
	return out, cleanup, nil
}

// withoutSimUser answers the tasks folder to publish: a copy of tasks without
// the simulated user's task (empty when nothing else is left), removed by
// cleanup.
func withoutSimUser(tasks string) (string, func(), error) {
	none := func() {}
	entries, err := os.ReadDir(tasks)
	if err != nil {
		return "", none, err
	}
	stage, err := os.MkdirTemp("", "scenario-tasks-")
	if err != nil {
		return "", none, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	kept := 0
	for _, en := range entries {
		if en.Name() == SimUserTask+".md" {
			continue
		}
		src, dst := filepath.Join(tasks, en.Name()), filepath.Join(stage, en.Name())
		if en.IsDir() {
			err = copyTree(src, dst)
		} else {
			err = copyFile(src, dst)
		}
		if err != nil {
			cleanup()
			return "", none, err
		}
		kept++
	}
	if kept == 0 {
		cleanup()
		return "", none, nil
	}
	return stage, cleanup, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
