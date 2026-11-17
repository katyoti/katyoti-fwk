// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// Step is one program's install step, as a runner receives it.
type Step struct {
	Program string
	Recipe  publish.InstallRecipe
	// Script is a script recipe's script, resolved in its sidecar's folder.
	Script string
	// Version is the version the sidecars are validated with (none: the
	// latest): a winget step installs exactly it, so the check finds it.
	Version string
}

// Runner runs install steps on this machine. The engine owns the plan, the
// download and its sha256 check, the grouping and the order; a runner only
// runs. Tests give a fake one: nothing real is ever installed by a test.
type Runner interface {
	// Available says whether the installer of a recipe kind is on this machine.
	Available(kind string) bool
	// Install runs one step in the person's own profile, without a prompt.
	Install(ctx context.Context, s Step) error
	// InstallElevated runs every administrator step behind ONE prompt.
	InstallElevated(ctx context.Context, steps []Step) error
	// AddToPath puts a folder on the person's PATH (a downloaded program's).
	AddToPath(dir string) error
	// RefreshPath reads PATH again, so what the steps installed is found by
	// the checks that follow.
	RefreshPath() error
}

// Fetch reads a download's bytes.
type Fetch func(ctx context.Context, url string) ([]byte, error)

// Recheck runs the sidecars' checks again and returns their needs, and the
// sidecars it could not check.
type Recheck func(ctx context.Context) ([]Need, []string)

// Engine installs a plan.
type Engine struct {
	Runner Runner
	Fetch  Fetch
	// ToolsDir is where downloads are placed, a folder per program.
	ToolsDir string
	// Platform is the runtime.GOOS planned for.
	Platform string
	// Progress, when set, is told each step as it starts (the program, how it
	// installs), so a step that hangs is named while it hangs.
	Progress io.Writer
	// StepTimeout bounds one step - a user-scope program, or the administrator
	// steps behind their one prompt (0: unbounded). A step that does not end
	// in time is stopped and fails naming its program(s).
	StepTimeout time.Duration
}

// DefaultStepTimeout is how long one program's install may take before it is
// stopped: long enough for a large download over a slow line, short enough
// that a step waiting on a prompt nobody sees fails while it can be named.
const DefaultStepTimeout = 15 * time.Minute

// say tells Progress what starts.
func (e *Engine) say(format string, a ...any) {
	if e.Progress != nil {
		fmt.Fprintf(e.Progress, format+"\n", a...)
	}
}

// bounded runs one step under StepTimeout; a step stopped by it fails saying
// what was waited on and for how long.
func (e *Engine) bounded(ctx context.Context, what string, run func(context.Context) error) error {
	if e.StepTimeout <= 0 {
		return run(ctx)
	}
	sctx, cancel := context.WithTimeout(ctx, e.StepTimeout)
	defer cancel()
	err := run(sctx)
	if errors.Is(sctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return &explained{msg: fmt.Sprintf("%s: no end after %s, stopped (a prompt or a download that never answers)", what, e.StepTimeout), cause: err}
	}
	return err
}

// Plan plans needs on the engine's platform with its runner's installers.
func (e *Engine) Plan(needs []Need) mktapi.PrereqsPlan {
	return Plan(needs, e.Platform, e.Runner.Available)
}

// Install runs the steps of plan for the programs named (none: the selected
// ones) and the programs providing their installers, then reads PATH again and
// plans again from recheck.
//
// The steps run in waves: a program after the one providing its installer
// (After), PATH read again between waves. In a wave the user-scope steps run
// one by one; the administrator steps of every wave run together, behind one
// prompt, in the wave after their providers, and their dependents wait for
// them. A program already present, or with no recipe here, is skipped with
// why; one whose provider failed is not run. A dry run returns the steps as
// planned, in the order they would run.
func (e *Engine) Install(ctx context.Context, plan mktapi.PrereqsPlan, names []string, dryRun bool, recheck Recheck) (out mktapi.PrereqsInstallOutput, err error) {
	out = mktapi.PrereqsInstallOutput{Plan: plan}
	defer func() {
		last := out.Plan
		if out.After != nil {
			last = *out.After
		}
		out.Missing = MissingRequired(last)
	}()
	targets, err := pick(plan, names)
	if err != nil {
		return out, err
	}
	// A required program no recipe installs here is not selected (nothing to
	// run), yet it is still missing: the default install says so, with its
	// install skill, rather than report nothing to install.
	if len(names) == 0 {
		for _, p := range plan.Programs {
			if p.Required && p.State == mktapi.PrereqManual {
				targets = append(targets, p)
			}
		}
	}
	var run []mktapi.PlannedProgram
	for _, p := range targets {
		switch {
		case p.State == mktapi.PrereqOK:
			out.Steps = append(out.Steps, mktapi.PrereqStep{Program: p.Name, Status: mktapi.StepSkipped, Detail: "already present"})
		case p.Recipe == nil:
			out.Steps = append(out.Steps, mktapi.PrereqStep{Program: p.Name, Status: mktapi.StepSkipped, Detail: p.Fallback})
		default:
			run = append(run, p)
		}
	}
	waves := order(run)
	if dryRun {
		for _, w := range waves {
			for _, p := range w {
				out.Steps = append(out.Steps, mktapi.PrereqStep{Program: p.Name, Kind: p.Recipe.Kind, Scope: p.Scope, Status: mktapi.StepPlanned, Detail: describe(p)})
			}
		}
		return out, nil
	}
	ran := false
	failed := map[string]bool{}
	for i, w := range waves {
		var admin []mktapi.PlannedProgram
		ranHere := false
		for _, p := range w {
			if p.After != "" && failed[strings.ToLower(p.After)] {
				failed[strings.ToLower(p.Name)] = true
				out.Steps = append(out.Steps, mktapi.PrereqStep{Program: p.Name, Kind: p.Recipe.Kind, Scope: p.Scope, Status: mktapi.StepFailed,
					Detail: p.After + ", which provides " + p.Recipe.Kind + ", did not install"})
				continue
			}
			if p.Scope == mktapi.ScopeAdmin {
				admin = append(admin, p)
				continue
			}
			st := mktapi.PrereqStep{Program: p.Name, Kind: p.Recipe.Kind, Scope: p.Scope, Status: mktapi.StepInstalled, Detail: describe(p)}
			e.say("installing %s (%s)...", p.Name, describe(p))
			if err := e.bounded(ctx, p.Name, func(ctx context.Context) error { return e.runUser(ctx, p) }); err != nil {
				st.Status = mktapi.StepFailed
				st.Detail, st.Report = Explain(err)
				failed[strings.ToLower(p.Name)] = true
			} else {
				ranHere = true
			}
			out.Steps = append(out.Steps, st)
		}
		if len(admin) > 0 {
			steps := make([]Step, 0, len(admin))
			for _, p := range admin {
				steps = append(steps, step(p))
			}
			out.ElevationPrompts++
			names := make([]string, 0, len(admin))
			for _, p := range admin {
				names = append(names, p.Name)
			}
			e.say("installing as administrator, behind one prompt: %s...", strings.Join(names, ", "))
			err := e.bounded(ctx, "the administrator steps ("+strings.Join(names, ", ")+")", func(ctx context.Context) error {
				return e.Runner.InstallElevated(ctx, steps)
			})
			for _, p := range admin {
				st := mktapi.PrereqStep{Program: p.Name, Kind: p.Recipe.Kind, Scope: p.Scope, Status: mktapi.StepInstalled, Detail: describe(p)}
				if err != nil {
					st.Status = mktapi.StepFailed
					st.Detail, st.Report = Explain(err)
					failed[strings.ToLower(p.Name)] = true
				}
				out.Steps = append(out.Steps, st)
			}
			ranHere = ranHere || err == nil
		}
		ran = ran || ranHere
		// A later wave finds what this one installed only once PATH is read again.
		if ranHere && i < len(waves)-1 {
			if err := e.Runner.RefreshPath(); err != nil {
				return out, fmt.Errorf("read PATH again: %w", err)
			}
		}
	}
	if !ran {
		return out, nil
	}
	if err := e.Runner.RefreshPath(); err != nil {
		return out, fmt.Errorf("read PATH again: %w", err)
	}
	out.PathRefreshed = true
	if recheck != nil {
		needs, unchecked := recheck(ctx)
		after := e.Plan(needs)
		after.Unchecked = unchecked
		out.After = &after
	}
	return out, nil
}

// MissingRequired names the required programs a plan does not find present.
func MissingRequired(plan mktapi.PrereqsPlan) []string {
	var out []string
	for _, p := range plan.Programs {
		if p.Required && p.State != mktapi.PrereqOK {
			out = append(out, p.Name)
		}
	}
	return out
}

// order puts programs in waves: a program after the one of the set providing
// its installer, every administrator program in one wave (one prompt) — the
// wave after the deepest of their providers — and a program depending on one
// of them after it. Within a wave the order picked is kept.
func order(programs []mktapi.PlannedProgram) [][]mktapi.PlannedProgram {
	idx := map[string]int{}
	for i, p := range programs {
		idx[strings.ToLower(p.Name)] = i
	}
	depth := make([]int, len(programs))
	for changed, rounds := true, 0; changed && rounds <= len(programs)+1; rounds++ {
		changed = false
		admin := 0
		for i, p := range programs {
			if j, ok := idx[strings.ToLower(p.After)]; ok && p.After != "" && depth[i] < depth[j]+1 {
				depth[i] = depth[j] + 1
				changed = true
			}
			if p.Scope == mktapi.ScopeAdmin && depth[i] > admin {
				admin = depth[i]
			}
		}
		for i, p := range programs {
			if p.Scope == mktapi.ScopeAdmin && depth[i] < admin {
				depth[i] = admin
				changed = true
			}
		}
	}
	var waves [][]mktapi.PlannedProgram
	for i, p := range programs {
		for len(waves) <= depth[i] {
			waves = append(waves, nil)
		}
		waves[depth[i]] = append(waves[depth[i]], p)
	}
	out := waves[:0]
	for _, w := range waves {
		if len(w) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// pick resolves the programs named in a plan (none names the selected ones),
// with the programs providing their installers.
func pick(plan mktapi.PrereqsPlan, names []string) ([]mktapi.PlannedProgram, error) {
	find := func(n string) (mktapi.PlannedProgram, bool) {
		for _, p := range plan.Programs {
			if strings.EqualFold(p.Name, n) {
				return p, true
			}
		}
		return mktapi.PlannedProgram{}, false
	}
	var out []mktapi.PlannedProgram
	have := map[string]bool{}
	add := func(p mktapi.PlannedProgram) {
		if !have[strings.ToLower(p.Name)] {
			have[strings.ToLower(p.Name)] = true
			out = append(out, p)
		}
	}
	if len(names) == 0 {
		for _, p := range plan.Programs {
			if p.Selected {
				add(p)
			}
		}
	} else {
		var unknown []string
		for _, n := range names {
			if p, ok := find(n); ok {
				add(p)
			} else {
				unknown = append(unknown, n)
			}
		}
		if len(unknown) > 0 {
			return nil, fmt.Errorf("no sidecar of these universes needs %s", strings.Join(unknown, ", "))
		}
	}
	// The providers of the programs picked, before them.
	for i := 0; i < len(out); i++ {
		if out[i].After == "" || have[strings.ToLower(out[i].After)] {
			continue
		}
		if p, ok := find(out[i].After); ok {
			add(p)
		}
	}
	return out, nil
}

func step(p mktapi.PlannedProgram) Step {
	return Step{Program: p.Name, Recipe: *p.Recipe, Script: p.ScriptPath, Version: p.Version}
}

// describe says what a step installs.
func describe(p mktapi.PlannedProgram) string {
	r := p.Recipe
	switch r.Kind {
	case publish.RecipeDownload:
		return "download " + r.URL
	case publish.RecipeScript:
		if p.ScriptPath != "" {
			return "script " + p.ScriptPath
		}
		return "script " + r.Script
	}
	return r.Kind + " " + r.Package
}

// runUser runs one user-scope step: a download is fetched, checked against
// its sha256 and placed by the engine; any other kind goes to the runner.
func (e *Engine) runUser(ctx context.Context, p mktapi.PlannedProgram) error {
	if p.Recipe.Kind != publish.RecipeDownload {
		if p.Recipe.Kind == publish.RecipeScript && p.ScriptPath == "" {
			return fmt.Errorf("script %s: the sidecar's folder is not known", p.Recipe.Script)
		}
		return e.Runner.Install(ctx, step(p))
	}
	if e.Fetch == nil || e.ToolsDir == "" {
		return errors.New("download: no fetcher or tools folder")
	}
	data, err := e.Fetch(ctx, p.Recipe.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", p.Recipe.URL, err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, p.Recipe.SHA256) {
		return fmt.Errorf("download %s refused: sha256 %s, the recipe declares %s (nothing was written)", p.Recipe.URL, got, strings.ToLower(p.Recipe.SHA256))
	}
	dir := filepath.Join(e.ToolsDir, safeName(p.Name))
	if err := place(data, p.Recipe.URL, dir); err != nil {
		return fmt.Errorf("place %s: %w", p.Recipe.URL, err)
	}
	bin := commandDir(dir, p.Commands)
	return e.Runner.AddToPath(bin)
}

// place writes a download into dir: a zip or a tar.gz unpacked, any other
// file written as it is (executable).
func place(data []byte, url, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := path.Base(strings.SplitN(url, "?", 2)[0])
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, ".zip"), strings.HasSuffix(lower, ".nupkg"):
		// A NuGet package is a zip (Python's full build for Windows is one).
		return unzip(data, dir)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return untgz(data, dir)
	}
	return os.WriteFile(filepath.Join(dir, safeName(base)), data, 0o755)
}

func unzip(data []byte, dir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		dst, err := inside(dir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(dst, rc, f.Mode()|0o600)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untgz(data []byte, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		dst, err := inside(dir, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(dst, tr, os.FileMode(h.Mode)|0o600); err != nil {
				return err
			}
		}
	}
}

// inside resolves an archive entry in dir, refusing one that climbs out.
func inside(dir, name string) (string, error) {
	clean := path.Clean("/" + strings.ReplaceAll(name, `\`, "/"))
	dst := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	if rel, err := filepath.Rel(dir, dst); err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("archive entry %q climbs out", name)
	}
	return dst, nil
}

func writeFile(dst string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// commandDir is the folder under dir holding one of the commands (an archive
// often nests its binary), else dir.
func commandDir(dir string, commands []string) string {
	// The shallowest match: a package may carry copies of the command deeper
	// in it (Python's venv launcher stub under Lib/venv/scripts/nt/).
	found, depth := "", -1
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := strings.TrimSuffix(strings.ToLower(d.Name()), ".exe")
		for _, c := range commands {
			if name == strings.TrimSuffix(strings.ToLower(c), ".exe") {
				if n := strings.Count(filepath.ToSlash(p), "/"); depth < 0 || n < depth {
					found, depth = filepath.Dir(p), n
				}
			}
		}
		return nil
	})
	if found == "" {
		return dir
	}
	return found
}

// safeName keeps a name usable as one path element.
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, s)
	if s == "" || s == "." || s == ".." {
		return "program"
	}
	return s
}
