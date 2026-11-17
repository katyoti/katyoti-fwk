// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// prereqEngine is the engine that installs programs on this machine: the
// system runner, downloads fetched as the store fetches, placed under
// <GlobalHomeDir>/tools/<program>/.
func (s *service) prereqEngine() *prereqs.Engine {
	if s.prereqs != nil {
		return s.prereqs
	}
	return &prereqs.Engine{
		Runner:   prereqs.System{},
		Fetch:    store.Fetch,
		ToolsDir: filepath.Join(application.Env().GlobalHomeDir(), "tools"),
		Platform: runtime.GOOS,
		// Each step said as it starts (the command line's stderr when the
		// install runs in it), each bounded: a hang names its program.
		Progress:    os.Stderr,
		StepTimeout: prereqs.DefaultStepTimeout,
	}
}

// PlanPrereqs plans the programs of the named universes' sidecars. It first
// forgets the installed items whose files are gone and rewires the universes
// still pinned to them (forgetVanished): what is gone is reported not
// installed, never config-checked.
func (s *service) PlanPrereqs(ctx context.Context, in mktapi.PrereqsPlanInput) (mktapi.PrereqsPlan, error) {
	gone, err := forgetVanished(in.Universes)
	if err != nil {
		return mktapi.PrereqsPlan{}, err
	}
	needs, unchecked, err := s.prereqNeeds(ctx, in.Universes)
	if err != nil {
		return mktapi.PrereqsPlan{}, err
	}
	plan := s.prereqEngine().Plan(needs)
	plan.Unchecked = unchecked
	plan.NotInstalled, plan.Rewired = gone.notInstalled, gone.rewired
	return withEmptyPrograms(plan), nil
}

// vanishedReport is what forgetVanished did.
type vanishedReport struct {
	notInstalled, rewired []string
}

// forgetVanished removes from the installed-items registry every entry whose
// files are gone, then rewires every universe member pinned to a program that
// is gone to the installed item of its name; a member none
// replaces is kept and named not installed when it belongs to one of the
// universes asked about. Each move is logged.
func forgetVanished(universes []string) (vanishedReport, error) {
	var rep vanishedReport
	if path, err := orchcatalog.DefaultSidecarsPath(); err == nil {
		if _, err := orchcatalog.PruneVanishedSidecars(path); err != nil {
			return rep, err
		}
	}
	installed, err := available.List()
	if err != nil {
		return rep, err
	}
	moved, missing, err := wire.RepinVanished(installed)
	for _, m := range moved {
		slog.Info("Univers.yaml: a member pinned to a program that is gone is rewired to the installed version", "universe", m.Universe.Name, "member", m.Member, "from", m.From)
		rep.rewired = append(rep.rewired, m.Universe.Name+": "+m.Member)
	}
	if err != nil {
		return rep, err
	}
	// The universes asked about; none, or the default one, is every universe.
	all := len(universes) == 0
	asked := map[string]bool{}
	for _, u := range universes {
		home := universeTarget(u)
		if home == "" {
			all = true
		}
		asked[strings.ToLower(filepath.Clean(home))] = true
	}
	seen := map[string]bool{}
	for _, m := range missing {
		slog.Warn("Univers.yaml: a member's program is gone and nothing installed replaces it", "universe", m.Universe.Name, "member", m.Member, "program", m.Program)
		if (all || asked[strings.ToLower(filepath.Clean(m.Universe.Home))]) && !seen[m.Member] {
			seen[m.Member] = true
			rep.notInstalled = append(rep.notInstalled, m.Member)
		}
	}
	sort.Strings(rep.notInstalled)
	return rep, nil
}

// InstallPrereqs installs programs of the plan, then checks again.
func (s *service) InstallPrereqs(ctx context.Context, in mktapi.PrereqsInstallInput) (mktapi.PrereqsInstallOutput, error) {
	needs, unchecked, err := s.prereqNeeds(ctx, in.Universes)
	if err != nil {
		return mktapi.PrereqsInstallOutput{}, err
	}
	ctx, st := beginSteps(ctx, 0)
	eng := *s.prereqEngine()
	// Each step the engine says (a program as it starts) is a step of the
	// report too.
	if st.on {
		if eng.Progress != nil {
			eng.Progress = io.MultiWriter(eng.Progress, stepWriter{st})
		} else {
			eng.Progress = stepWriter{st}
		}
	}
	plan := eng.Plan(needs)
	plan.Unchecked = unchecked
	out, err := eng.Install(ctx, withEmptyPrograms(plan), in.Programs, in.DryRun, func(ctx context.Context) ([]prereqs.Need, []string) {
		n, u, _ := s.prereqNeeds(ctx, in.Universes)
		return n, u
	})
	if out.After != nil {
		a := withEmptyPrograms(*out.After)
		out.After = &a
	}
	if err == nil {
		st.end("The sidecars' checks ran again")
	}
	return out, err
}

// prereqNeeds reads, from each sidecar wired into the universes (installed
// from a marketplace, never a control-plane binary), its config check's
// programs. A sidecar wired into several universes is checked once: a
// program is the machine's, not a universe's. A sidecar whose check fails is
// named in unchecked, not fatal.
func (s *service) prereqNeeds(ctx context.Context, universes []string) ([]prereqs.Need, []string, error) {
	if len(universes) == 0 {
		universes = []string{""}
	}
	installed, err := available.List()
	if err != nil {
		return nil, nil, err
	}
	type job struct {
		home string
		ins  mktapi.Installed
	}
	var jobs []job
	seen := map[string]bool{}
	for _, u := range universes {
		home := universeTarget(u)
		wired := wire.WiredNames(home)
		for _, ins := range installed {
			if !wired[ins.Name] || ins.Source != "marketplace" || controlplane.IsInfra(ins.Name) || seen[ins.Name] {
				continue
			}
			seen[ins.Name] = true
			jobs = append(jobs, job{home, ins})
		}
	}
	reps := make([]*sidecarapi.ConfigCheckReport, len(jobs))
	errs := make([]error, len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rep, err := s.checkInstalled(ctx, j.home, j.ins)
			if err != nil {
				errs[i] = err
				return
			}
			reps[i] = &rep
		}()
	}
	wg.Wait()
	var needs []prereqs.Need
	var unchecked []string
	for i, j := range jobs {
		if reps[i] == nil {
			unchecked = append(unchecked, fmt.Sprintf("%s: %v", j.ins.Name, errs[i]))
			continue
		}
		needs = append(needs, prereqs.FromConfigCheck(j.ins.Name, sidecarDir(j.ins), *reps[i])...)
	}
	sort.Strings(unchecked)
	return needs, unchecked, nil
}

// solutionPrereqs plans the programs from the reports of a solution's members
// configured at install: what the Ready to start screen and `katyoti prereqs
// install` then install. Nothing is installed here.
func (s *service) solutionPrereqs(members []mktapi.Installed, reports []*sidecarapi.ConfigCheckReport) mktapi.PrereqsPlan {
	var needs []prereqs.Need
	var unchecked []string
	for i, ins := range members {
		if reports[i] == nil {
			unchecked = append(unchecked, ins.Name)
			continue
		}
		needs = append(needs, prereqs.FromConfigCheck(ins.Name, sidecarDir(ins), *reports[i])...)
	}
	plan := s.prereqEngine().Plan(needs)
	plan.Unchecked = unchecked
	return withEmptyPrograms(plan)
}

// sidecarDir is the folder a sidecar's script recipe resolves in: a content
// item's folder, a compiled sidecar's binary's folder.
func sidecarDir(ins mktapi.Installed) string {
	if ins.FolderPath != "" {
		return ins.FolderPath
	}
	if ins.BinaryPath != "" {
		return filepath.Dir(ins.BinaryPath)
	}
	return ""
}

// withEmptyPrograms keeps Programs a JSON array (the generated client
// iterates it).
func withEmptyPrograms(p mktapi.PrereqsPlan) mktapi.PrereqsPlan {
	if p.Programs == nil {
		p.Programs = []mktapi.PlannedProgram{}
	}
	return p
}
