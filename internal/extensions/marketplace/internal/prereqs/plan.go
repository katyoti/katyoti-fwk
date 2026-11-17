// SPDX-License-Identifier: Apache-2.0

// Package prereqs is the prereqs engine: from the programs every sidecar of a
// set of universes declares (its config check's deps, or its describe's tools)
// it plans what this machine needs — one row per program, deduplicated across
// sidecars at the highest version, required or optional, with the recipe
// chosen for this platform — and installs it: user-scope steps one by one,
// administrator steps together behind one prompt, then PATH read again and
// the sidecars checked again. A program whose installer (pip, bun) another
// program of the plan provides is installed after that provider, PATH read
// again between them.
//
// It never reads a sidecar's name to decide anything: the declarations are its
// only input (modules/katyoti/solutions.md §10).
package prereqs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// Need is one sidecar's declaration of one program, and what its check found.
type Need struct {
	// Sidecar declares the program.
	Sidecar string
	// Dir is the sidecar's folder: a script recipe's script resolves in it.
	Dir      string
	Name     string
	Required bool
	Version  string
	Commands []string
	// PythonPackage is the Python package the program is, checked as a
	// package (publish.CheckPythonPackage), never on PATH.
	PythonPackage string
	InstallSkill  string
	Recipes       []publish.InstallRecipe
	// Checked: the check ran; OK and Found say what it found. A need read
	// from a describe is not checked, and counts as missing.
	Checked bool
	OK      bool
	Found   string
}

// FromConfigCheck reads a sidecar's needs from its config check report.
func FromConfigCheck(sidecar, dir string, rep sidecarapi.ConfigCheckReport) []Need {
	out := make([]Need, 0, len(rep.Deps))
	for _, d := range rep.Deps {
		out = append(out, Need{
			Sidecar: sidecar, Dir: dir, Name: d.Name, Required: d.Required,
			Version: d.RequiredVersion, Commands: d.Commands, PythonPackage: d.PythonPackage,
			InstallSkill: d.InstallSkill, Recipes: d.Recipes, Checked: true, OK: d.OK, Found: d.FoundVersion,
		})
	}
	return out
}

// FromDescribe reads a sidecar's needs from its describe's tools (no check).
func FromDescribe(sidecar, dir string, tools []publish.ToolRequirement) []Need {
	out := make([]Need, 0, len(tools))
	for _, t := range tools {
		out = append(out, Need{
			Sidecar: sidecar, Dir: dir, Name: t.Name, Required: t.Required,
			Version: t.Version, Commands: t.Commands, InstallSkill: t.InstallSkill,
			Recipes: t.Recipes,
		})
	}
	return out
}

// Available says whether the installer a recipe kind uses is on this machine.
type Available func(kind string) bool

// NeedsAdmin says whether a recipe kind installs only with administrator
// rights. Every other kind installs in the person's own profile.
func NeedsAdmin(kind string) bool {
	return kind == publish.RecipeApt
}

// candidate is a recipe and the folder of the sidecar that declares it.
type candidate struct {
	recipe publish.InstallRecipe
	dir    string
	ver    string // the declaring sidecar's version of the program
}

// Plan builds the plan of a set of needs on one platform. avail says which
// installers are on this machine; nil takes every one as present.
func Plan(needs []Need, platform string, avail Available) mktapi.PrereqsPlan {
	if avail == nil {
		avail = func(string) bool { return true }
	}
	type acc struct {
		p     mktapi.PlannedProgram
		ok    bool
		cands []candidate
		seen  map[string]bool
	}
	byName := map[string]*acc{}
	var order []string
	sidecars := map[string]bool{}
	for _, n := range needs {
		sidecars[n.Sidecar] = true
		key := strings.ToLower(strings.TrimSpace(n.Name))
		if key == "" {
			continue
		}
		a := byName[key]
		if a == nil {
			a = &acc{p: mktapi.PlannedProgram{Name: n.Name}, ok: true, seen: map[string]bool{}}
			byName[key] = a
			order = append(order, key)
		}
		a.p.Required = a.p.Required || n.Required
		if !contains(a.p.NeededBy, n.Sidecar) {
			a.p.NeededBy = append(a.p.NeededBy, n.Sidecar)
		}
		for _, c := range n.Commands {
			if !contains(a.p.Commands, c) {
				a.p.Commands = append(a.p.Commands, c)
			}
		}
		if a.p.PythonPackage == "" {
			a.p.PythonPackage = n.PythonPackage
		}
		if CompareVersions(n.Version, a.p.Version) > 0 {
			a.p.Version = n.Version
		}
		if a.p.InstallSkill == "" {
			a.p.InstallSkill = n.InstallSkill
		}
		if !(n.Checked && n.OK) {
			a.ok = false
		}
		if n.Found != "" && (a.p.Found == "" || CompareVersions(n.Found, a.p.Found) < 0) {
			a.p.Found = n.Found
		}
		for _, r := range n.Recipes {
			if r.Platform != platform {
				continue
			}
			k := r.Kind + "|" + r.Package + "|" + r.URL + "|" + r.Script
			if r.Kind == publish.RecipeScript {
				k += "|" + n.Dir // each sidecar ships its own script
			}
			if a.seen[k] {
				continue
			}
			a.seen[k] = true
			a.cands = append(a.cands, candidate{recipe: r, dir: n.Dir, ver: n.Version})
		}
	}
	plan := mktapi.PrereqsPlan{Platform: platform}
	for s := range sidecars {
		plan.Sidecars = append(plan.Sidecars, s)
	}
	sort.Strings(plan.Sidecars)
	sort.Strings(order)
	type row struct {
		p      mktapi.PlannedProgram
		cands  []candidate
		chosen *candidate
		after  string
		why    []string
	}
	rows := make([]*row, 0, len(order))
	for _, key := range order {
		a := byName[key]
		r := &row{p: a.p}
		sort.Strings(r.p.NeededBy)
		if a.ok {
			r.p.State = mktapi.PrereqOK
			rows = append(rows, r)
			continue
		}
		// The recipes of the sidecar that asks for the highest version come
		// first: each sidecar's own order of preference is kept.
		sort.SliceStable(a.cands, func(i, j int) bool {
			return CompareVersions(a.cands[i].ver, a.cands[j].ver) > 0
		})
		r.cands = a.cands
		r.chosen, r.after, r.why = choose(a.cands, r.p.Version, avail, nil)
		rows = append(rows, r)
	}
	// A recipe whose installer is not on this machine still runs when another
	// program of the plan, installable itself, declares one of that
	// installer's commands (publish.RecipeInstallers): it is installed first.
	// Repeated until nothing changes, so a provider can have its own.
	provider := func(self *row) func(kind string) string {
		return func(kind string) string {
			for _, o := range rows {
				if o != self && o.chosen != nil && provides(o.p.Commands, kind) {
					return o.p.Name
				}
			}
			return ""
		}
	}
	for changed := true; changed; {
		changed = false
		for _, r := range rows {
			if r.p.State == mktapi.PrereqOK || r.chosen != nil {
				continue
			}
			if c, after, _ := choose(r.cands, r.p.Version, avail, provider(r)); c != nil {
				r.chosen, r.after = c, after
				changed = true
			}
		}
	}
	// An installer found here while the plan installs the program providing
	// it (the one its check accepts) still waits for that program.
	for _, r := range rows {
		if r.chosen != nil && r.after == "" {
			r.after = provider(r)(r.chosen.recipe.Kind)
		}
	}
	byProgram := map[string]*row{}
	for _, r := range rows {
		byProgram[r.p.Name] = r
		if r.p.State == mktapi.PrereqOK {
			continue
		}
		p := &r.p
		if r.chosen == nil {
			p.State = mktapi.PrereqManual
			p.Fallback = fallback(*p, platform, r.why)
			continue
		}
		rc := r.chosen.recipe
		p.State = mktapi.PrereqInstall
		p.Recipe = &rc
		p.After = r.after
		if rc.Kind == publish.RecipeScript && r.chosen.dir != "" {
			p.ScriptPath = filepath.Join(r.chosen.dir, filepath.FromSlash(rc.Script))
		}
		p.Scope = mktapi.ScopeUser
		if NeedsAdmin(rc.Kind) {
			p.Scope = mktapi.ScopeAdmin
		}
		p.Selected = p.Required
	}
	// A selected program selects the program providing its installer, even
	// an optional one.
	for changed := true; changed; {
		changed = false
		for _, r := range rows {
			if !r.p.Selected || r.p.After == "" {
				continue
			}
			if pr := byProgram[r.p.After]; pr != nil && !pr.p.Selected {
				pr.p.Selected = true
				changed = true
			}
		}
	}
	for _, r := range rows {
		if r.p.Selected && r.p.Scope == mktapi.ScopeAdmin {
			plan.Elevation = append(plan.Elevation, r.p.Name)
		}
		plan.Programs = append(plan.Programs, r.p)
	}
	return plan
}

// provides says whether a program's commands meet the installers of a kind.
func provides(commands []string, kind string) bool {
	for _, c := range commands {
		c = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c)), ".exe")
		for _, i := range publish.RecipeInstallers[kind] {
			if c == i {
				return true
			}
		}
	}
	return false
}

// choose takes the first candidate whose installer is here, or provided by
// a program of the plan (provider names it; nil: none is looked for), and
// that is not declared for a lower version than the one wanted; after names
// the providing program; why lists what was passed.
func choose(cands []candidate, want string, avail Available, provider func(kind string) string) (chosen *candidate, after string, why []string) {
	for i := range cands {
		c := cands[i]
		if c.recipe.MinVersion != "" && want != "" && CompareVersions(c.recipe.MinVersion, want) < 0 {
			why = append(why, fmt.Sprintf("%s installs %s, below %s", c.recipe.Kind, c.recipe.MinVersion, want))
			continue
		}
		if avail(c.recipe.Kind) {
			return &cands[i], "", why
		}
		if provider != nil {
			if p := provider(c.recipe.Kind); p != "" {
				return &cands[i], p, why
			}
		}
		why = append(why, c.recipe.Kind+" is not on this machine, nor provided by a program of this plan")
	}
	return nil, "", why
}

// fallback is the prose a person or an LLM follows when no recipe runs here.
func fallback(p mktapi.PlannedProgram, platform string, why []string) string {
	var b strings.Builder
	if len(why) == 0 {
		fmt.Fprintf(&b, "no install recipe for %s", platform)
	} else {
		fmt.Fprintf(&b, "no install recipe runs here (%s)", strings.Join(why, "; "))
	}
	if p.InstallSkill != "" {
		fmt.Fprintf(&b, ": follow the install skill %s", p.InstallSkill)
	} else {
		fmt.Fprintf(&b, ": install %s by hand", p.Name)
	}
	switch {
	case p.PythonPackage != "":
		fmt.Fprintf(&b, " so that the Python package %s is installed", p.PythonPackage)
	case len(p.Commands) > 0:
		fmt.Fprintf(&b, " so that %s is on PATH", strings.Join(p.Commands, " or "))
	}
	return b.String()
}

// CompareVersions compares two dotted versions number by number ("3.10" is
// above "3.9"); a leading "v" and anything after the numbers ("0.15.1
// (9dfd3a08)", "1.2.0-rc1") are ignored. An empty version is the lowest.
func CompareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case a == "" && b != "":
		return -1
	case a != "" && b == "":
		return 1
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out []int
	for _, f := range strings.Split(v, ".") {
		end := 0
		for end < len(f) && f[end] >= '0' && f[end] <= '9' {
			end++
		}
		if end == 0 {
			break
		}
		n, _ := strconv.Atoi(f[:end])
		out = append(out, n)
		if end < len(f) {
			break
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
