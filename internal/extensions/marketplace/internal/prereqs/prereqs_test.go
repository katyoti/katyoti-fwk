// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

func winget(pkg string) publish.InstallRecipe {
	return publish.InstallRecipe{Platform: "windows", Kind: publish.RecipeWinget, Package: pkg}
}
func scoop(pkg string) publish.InstallRecipe {
	return publish.InstallRecipe{Platform: "windows", Kind: publish.RecipeScoop, Package: pkg}
}
func brew(platform, pkg string) publish.InstallRecipe {
	return publish.InstallRecipe{Platform: platform, Kind: publish.RecipeBrew, Package: pkg}
}
func apt(pkg string) publish.InstallRecipe {
	return publish.InstallRecipe{Platform: "linux", Kind: publish.RecipeApt, Package: pkg}
}

func program(t *testing.T, plan mktapi.PrereqsPlan, name string) mktapi.PlannedProgram {
	t.Helper()
	for _, p := range plan.Programs {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no %s in the plan: %+v", name, plan.Programs)
	return mktapi.PlannedProgram{}
}

func all(string) bool { return true }

// One program declared by two sidecars is one row, needed by both.
func TestPlan_DeduplicatesAcrossSidecars(t *testing.T) {
	needs := []Need{
		{Sidecar: "docs", Name: "pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{winget("JohnMacFarlane.Pandoc")}},
		{Sidecar: "kb", Name: "Pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{winget("JohnMacFarlane.Pandoc")}},
		{Sidecar: "kb", Name: "typst", Required: true, Checked: true, OK: true},
	}
	plan := Plan(needs, "windows", all)
	if len(plan.Programs) != 2 {
		t.Fatalf("programs = %d, want 2 (pandoc once, typst): %+v", len(plan.Programs), plan.Programs)
	}
	p := program(t, plan, "pandoc")
	if strings.Join(p.NeededBy, ",") != "docs,kb" {
		t.Errorf("pandoc needed by %v, want docs and kb", p.NeededBy)
	}
	if p.State != mktapi.PrereqInstall || !p.Selected {
		t.Errorf("pandoc: state %s selected %v, want install, selected", p.State, p.Selected)
	}
	if ty := program(t, plan, "typst"); ty.State != mktapi.PrereqOK || ty.Selected {
		t.Errorf("typst is present: state %s selected %v", ty.State, ty.Selected)
	}
	if strings.Join(plan.Sidecars, ",") != "docs,kb" {
		t.Errorf("sidecars %v", plan.Sidecars)
	}
}

// The highest declared version is the plan's, and a recipe declared for a
// lower version gives way to one that reaches it (3.10 is above 3.9).
func TestPlan_HighestVersionWins(t *testing.T) {
	low := winget("Pandoc.Old")
	low.MinVersion = "3.9"
	high := scoop("pandoc")
	high.MinVersion = "3.10"
	needs := []Need{
		{Sidecar: "a", Name: "pandoc", Required: true, Version: "3.9", Recipes: []publish.InstallRecipe{low}},
		{Sidecar: "b", Name: "pandoc", Required: true, Version: "3.10", Recipes: []publish.InstallRecipe{high}},
	}
	p := program(t, Plan(needs, "windows", all), "pandoc")
	if p.Version != "3.10" {
		t.Errorf("version = %q, want 3.10", p.Version)
	}
	if p.Recipe == nil || p.Recipe.Kind != publish.RecipeScoop {
		t.Errorf("recipe = %+v, want the scoop one declared for 3.10", p.Recipe)
	}
}

// Required by one sidecar is required; optional everywhere is offered, never
// preselected.
func TestPlan_RequiredVersusOptional(t *testing.T) {
	needs := []Need{
		{Sidecar: "a", Name: "ffmpeg", Required: false, Recipes: []publish.InstallRecipe{winget("Gyan.FFmpeg")}},
		{Sidecar: "b", Name: "ffmpeg", Required: true, Recipes: []publish.InstallRecipe{winget("Gyan.FFmpeg")}},
		{Sidecar: "a", Name: "docling", Required: false, Recipes: []publish.InstallRecipe{{Platform: "windows", Kind: publish.RecipePip, Package: "docling"}}},
	}
	plan := Plan(needs, "windows", all)
	if f := program(t, plan, "ffmpeg"); !f.Required || !f.Selected {
		t.Errorf("ffmpeg required by b: required %v selected %v", f.Required, f.Selected)
	}
	d := program(t, plan, "docling")
	if d.Required || d.Selected {
		t.Errorf("docling is optional: required %v selected %v, want offered, not preselected", d.Required, d.Selected)
	}
	if d.State != mktapi.PrereqInstall || d.Recipe == nil {
		t.Errorf("docling is offered with its recipe: state %s recipe %+v", d.State, d.Recipe)
	}
}

// Each platform gets its own recipe, in the declared order of preference,
// passing an installer that is not on the machine; apt is administrator.
func TestPlan_RecipePerPlatform(t *testing.T) {
	need := Need{Sidecar: "a", Name: "pandoc", Required: true, Recipes: []publish.InstallRecipe{
		winget("JohnMacFarlane.Pandoc"), scoop("pandoc"), brew("darwin", "pandoc"), apt("pandoc"), brew("linux", "pandoc"),
	}}
	cases := []struct {
		platform string
		avail    Available
		kind     string
		scope    string
	}{
		{"windows", all, publish.RecipeWinget, mktapi.ScopeUser},
		{"windows", func(k string) bool { return k != publish.RecipeWinget }, publish.RecipeScoop, mktapi.ScopeUser},
		{"darwin", all, publish.RecipeBrew, mktapi.ScopeUser},
		{"linux", all, publish.RecipeApt, mktapi.ScopeAdmin},
		{"linux", func(k string) bool { return k != publish.RecipeApt }, publish.RecipeBrew, mktapi.ScopeUser},
	}
	for _, c := range cases {
		plan := Plan([]Need{need}, c.platform, c.avail)
		p := program(t, plan, "pandoc")
		if p.Recipe == nil || p.Recipe.Kind != c.kind || p.Recipe.Platform != c.platform {
			t.Errorf("%s: recipe %+v, want %s", c.platform, p.Recipe, c.kind)
			continue
		}
		if p.Scope != c.scope {
			t.Errorf("%s %s: scope %s, want %s", c.platform, c.kind, p.Scope, c.scope)
		}
		if want := c.scope == mktapi.ScopeAdmin; want != (len(plan.Elevation) == 1) {
			t.Errorf("%s %s: elevation %v", c.platform, c.kind, plan.Elevation)
		}
	}
}

// No recipe for this platform: reported, with the prose skill to follow.
func TestPlan_NoRecipeHereFallsBackToTheSkill(t *testing.T) {
	needs := []Need{{Sidecar: "a", Name: "pandoc", Required: true, Commands: []string{"pandoc"},
		InstallSkill: "install-pandoc", Recipes: []publish.InstallRecipe{winget("JohnMacFarlane.Pandoc")}}}
	p := program(t, Plan(needs, "darwin", all), "pandoc")
	if p.State != mktapi.PrereqManual || p.Recipe != nil || p.Selected {
		t.Fatalf("state %s recipe %+v selected %v, want manual, no recipe, not selected", p.State, p.Recipe, p.Selected)
	}
	for _, want := range []string{"no install recipe for darwin", "install-pandoc", "pandoc is on PATH"} {
		if !strings.Contains(p.Fallback, want) {
			t.Errorf("fallback %q lacks %q", p.Fallback, want)
		}
	}
	// A recipe whose installer is absent falls back the same way, saying why.
	p = program(t, Plan(needs, "windows", func(string) bool { return false }), "pandoc")
	if p.State != mktapi.PrereqManual || !strings.Contains(p.Fallback, "winget is not on this machine") {
		t.Errorf("no installer: state %s fallback %q", p.State, p.Fallback)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"3.10", "3.9", 1}, {"0.15.1 (9dfd3a08)", "0.15.1", 0}, {"v1.2", "1.2.0", 0}, {"", "0.1", -1}, {"1.2.0-rc1", "1.3", -1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// fakeRunner installs nothing: it records the calls, and a program it
// "installed" is found by the checks only once PATH was read again.
type fakeRunner struct {
	installed map[string]bool
	userCalls []string
	elevated  [][]string
	added     []string
	refreshed bool
	// events is every call in order: install:<p>, elevated:<p,q>, refresh.
	events []string
	// avail says which installers are here; nil: every one.
	avail Available
}

func newFake() *fakeRunner { return &fakeRunner{installed: map[string]bool{}} }

func (f *fakeRunner) Available(kind string) bool { return f.avail == nil || f.avail(kind) }
func (f *fakeRunner) Install(_ context.Context, s Step) error {
	f.userCalls = append(f.userCalls, s.Program)
	f.installed[s.Program] = true
	f.events = append(f.events, "install:"+s.Program)
	return nil
}
func (f *fakeRunner) InstallElevated(_ context.Context, steps []Step) error {
	var names []string
	for _, s := range steps {
		names = append(names, s.Program)
		f.installed[s.Program] = true
	}
	f.elevated = append(f.elevated, names)
	f.events = append(f.events, "elevated:"+strings.Join(names, ","))
	return nil
}
func (f *fakeRunner) AddToPath(dir string) error { f.added = append(f.added, dir); return nil }
func (f *fakeRunner) RefreshPath() error {
	f.refreshed = true
	f.events = append(f.events, "refresh")
	return nil
}

// recheck is the sidecars' checks run again: a program the runner installed is
// found only after PATH was read again.
func (f *fakeRunner) recheck(needs []Need) Recheck {
	return func(context.Context) ([]Need, []string) {
		out := make([]Need, len(needs))
		for i, n := range needs {
			n.Checked = true
			n.OK = n.OK || (f.refreshed && f.installed[n.Name])
			out[i] = n
		}
		return out, nil
	}
}

// Every administrator step runs behind ONE prompt; the user steps run on
// their own; an optional program is not installed unless named.
func TestInstall_GroupsTheElevation(t *testing.T) {
	needs := []Need{
		{Sidecar: "a", Name: "pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{apt("pandoc")}},
		{Sidecar: "b", Name: "graphviz", Required: true, Checked: true, Recipes: []publish.InstallRecipe{apt("graphviz")}},
		{Sidecar: "b", Name: "typst", Required: true, Checked: true, Recipes: []publish.InstallRecipe{brew("linux", "typst")}},
		{Sidecar: "b", Name: "ffmpeg", Required: false, Checked: true, Recipes: []publish.InstallRecipe{apt("ffmpeg")}},
	}
	f := newFake()
	e := &Engine{Runner: f, Platform: "linux"}
	plan := e.Plan(needs)
	if strings.Join(plan.Elevation, ",") != "graphviz,pandoc" {
		t.Errorf("plan elevation %v, want graphviz and pandoc (ffmpeg is optional)", plan.Elevation)
	}
	out, err := e.Install(context.Background(), plan, nil, false, f.recheck(needs))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.elevated) != 1 || out.ElevationPrompts != 1 {
		t.Fatalf("elevation prompts: runner %d, output %d, want 1", len(f.elevated), out.ElevationPrompts)
	}
	if strings.Join(f.elevated[0], ",") != "graphviz,pandoc" {
		t.Errorf("the one prompt covers %v, want graphviz and pandoc", f.elevated[0])
	}
	if strings.Join(f.userCalls, ",") != "typst" {
		t.Errorf("user steps %v, want typst", f.userCalls)
	}
	if f.installed["ffmpeg"] {
		t.Error("ffmpeg is optional: installed without being named")
	}
	// Naming it installs it.
	f2 := newFake()
	e2 := &Engine{Runner: f2, Platform: "linux"}
	if _, err := e2.Install(context.Background(), e2.Plan(needs), []string{"ffmpeg"}, false, nil); err != nil {
		t.Fatal(err)
	}
	if !f2.installed["ffmpeg"] || len(f2.elevated) != 1 || f2.installed["pandoc"] {
		t.Errorf("named ffmpeg: installed %v, prompts %d", f2.installed, len(f2.elevated))
	}
}

// After the steps PATH is read again, then the checks run again: the program
// that was missing is OK.
func TestInstall_RefreshesPathThenChecksAgain(t *testing.T) {
	needs := []Need{{Sidecar: "a", Name: "pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{winget("JohnMacFarlane.Pandoc")}}}
	f := newFake()
	e := &Engine{Runner: f, Platform: "windows"}
	plan := e.Plan(needs)
	if program(t, plan, "pandoc").State != mktapi.PrereqInstall {
		t.Fatal("pandoc should be missing before")
	}
	out, err := e.Install(context.Background(), plan, nil, false, f.recheck(needs))
	if err != nil {
		t.Fatal(err)
	}
	if !out.PathRefreshed || !f.refreshed {
		t.Error("PATH was not read again after the install")
	}
	if out.After == nil {
		t.Fatal("no check after the install")
	}
	if p := program(t, *out.After, "pandoc"); p.State != mktapi.PrereqOK {
		t.Errorf("after: pandoc %s, want ok", p.State)
	}
	if out.ElevationPrompts != 0 {
		t.Errorf("a user-scope install asked for administrator rights %d time(s)", out.ElevationPrompts)
	}
}

// A dry run runs nothing.
func TestInstall_DryRunRunsNothing(t *testing.T) {
	needs := []Need{{Sidecar: "a", Name: "pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{apt("pandoc")}}}
	f := newFake()
	e := &Engine{Runner: f, Platform: "linux"}
	out, err := e.Install(context.Background(), e.Plan(needs), nil, true, f.recheck(needs))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.userCalls)+len(f.elevated) != 0 || f.refreshed || out.ElevationPrompts != 0 {
		t.Errorf("a dry run ran: %+v", f)
	}
	if len(out.Steps) != 1 || out.Steps[0].Status != mktapi.StepPlanned {
		t.Errorf("steps %+v", out.Steps)
	}
}

func downloadNeeds(sum string) []Need {
	return []Need{{Sidecar: "a", Name: "tool", Required: true, Checked: true, Commands: []string{"tool"}, Recipes: []publish.InstallRecipe{{
		Platform: "windows", Kind: publish.RecipeDownload, URL: "https://example.test/tool.exe", SHA256: sum,
	}}}}
}

// A download whose sha256 does not match the recipe is refused: nothing is
// written, nothing is put on PATH.
func TestInstall_RefusesASHA256Mismatch(t *testing.T) {
	body := []byte("the program")
	wrong := strings.Repeat("0", 64)
	tools := t.TempDir()
	f := newFake()
	e := &Engine{Runner: f, Platform: "windows", ToolsDir: tools,
		Fetch: func(context.Context, string) ([]byte, error) { return body, nil }}
	out, err := e.Install(context.Background(), e.Plan(downloadNeeds(wrong)), nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 1 || out.Steps[0].Status != mktapi.StepFailed || !strings.Contains(out.Steps[0].Detail, "refused") {
		t.Fatalf("steps %+v, want the download refused", out.Steps)
	}
	if entries, _ := os.ReadDir(tools); len(entries) != 0 {
		t.Errorf("a refused download wrote %d entr(ies)", len(entries))
	}
	if len(f.added) != 0 || f.refreshed {
		t.Errorf("a refused download touched PATH: added %v refreshed %v", f.added, f.refreshed)
	}

	// The matching digest places the file and puts its folder on PATH.
	h := sha256.Sum256(body)
	f2 := newFake()
	e2 := &Engine{Runner: f2, Platform: "windows", ToolsDir: tools,
		Fetch: func(context.Context, string) ([]byte, error) { return body, nil }}
	out, err = e2.Install(context.Background(), e2.Plan(downloadNeeds(hex.EncodeToString(h[:]))), nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Steps[0].Status != mktapi.StepInstalled {
		t.Fatalf("steps %+v", out.Steps)
	}
	got, err := os.ReadFile(filepath.Join(tools, "tool", "tool.exe"))
	if err != nil || string(got) != string(body) {
		t.Errorf("placed %q, %v", got, err)
	}
	if len(f2.added) != 1 || f2.added[0] != filepath.Join(tools, "tool") || !f2.refreshed {
		t.Errorf("added %v refreshed %v", f2.added, f2.refreshed)
	}
}

func pip(platform, pkg string) publish.InstallRecipe {
	return publish.InstallRecipe{Platform: platform, Kind: publish.RecipePip, Package: pkg}
}

// pythonNeeds: python (required or not) providing pip by its commands, and
// pymupdf, required, installed only by pip.
func pythonNeeds(pythonRequired bool) []Need {
	return []Need{
		{Sidecar: "kb", Name: "python", Required: pythonRequired, Checked: true, Commands: []string{"python", "python3", "py"},
			Recipes: []publish.InstallRecipe{winget("Python.Python.3.12"), apt("python3-pip")}},
		{Sidecar: "kb", Name: "pymupdf", Required: true, Checked: true,
			Recipes: []publish.InstallRecipe{pip("windows", "pymupdf"), pip("linux", "pymupdf")}},
	}
}

func noPip(kind string) bool { return kind != publish.RecipePip }

// pip is not on this machine, but python, in the same plan, provides it:
// pymupdf is installed after python, not left manual.
func TestPlan_AnInstallerProvidedByThePlanIsInstalledFirst(t *testing.T) {
	plan := Plan(pythonNeeds(true), "windows", noPip)
	p := program(t, plan, "pymupdf")
	if p.State != mktapi.PrereqInstall || p.After != "python" || !p.Selected || p.Recipe == nil || p.Recipe.Kind != publish.RecipePip {
		t.Fatalf("pymupdf: state %s after %q selected %v recipe %+v, want install after python, selected, pip", p.State, p.After, p.Selected, p.Recipe)
	}
	if py := program(t, plan, "python"); py.State != mktapi.PrereqInstall || py.After != "" {
		t.Errorf("python: state %s after %q, want install, first", py.State, py.After)
	}
	// pip found here while the plan installs python (the one its check
	// accepts): pymupdf still waits for python.
	if p := program(t, Plan(pythonNeeds(true), "windows", all), "pymupdf"); p.After != "python" {
		t.Errorf("pip here, python planned: pymupdf after %q, want python", p.After)
	}
	// python present: nothing to wait for.
	needs := pythonNeeds(true)
	needs[0].OK = true
	if p := program(t, Plan(needs, "windows", all), "pymupdf"); p.State != mktapi.PrereqInstall || p.After != "" {
		t.Errorf("python present: pymupdf %s after %q, want install, no after", p.State, p.After)
	}
}

// A provider not in the plan provides nothing: the program stays manual.
func TestPlan_AnInstallerNoProgramProvidesStaysManual(t *testing.T) {
	plan := Plan(pythonNeeds(true)[1:], "windows", noPip)
	p := program(t, plan, "pymupdf")
	if p.State != mktapi.PrereqManual || p.After != "" || p.Selected {
		t.Fatalf("pymupdf: state %s after %q selected %v, want manual", p.State, p.After, p.Selected)
	}
	if !strings.Contains(p.Fallback, "pip is not on this machine") {
		t.Errorf("fallback %q does not say pip is missing", p.Fallback)
	}
}

// An optional provider of a selected program is selected with it.
func TestPlan_ASelectedProgramSelectsItsProvider(t *testing.T) {
	plan := Plan(pythonNeeds(false), "windows", noPip)
	if py := program(t, plan, "python"); !py.Selected {
		t.Errorf("python is optional but provides pip to the required pymupdf: selected %v", py.Selected)
	}
}

// The install runs python, reads PATH again, then pymupdf, in one run; naming
// pymupdf alone brings python.
func TestInstall_ProvidersFirstThenPathThenDependents(t *testing.T) {
	needs := pythonNeeds(true)
	f := newFake()
	f.avail = noPip
	e := &Engine{Runner: f, Platform: "windows"}
	out, err := e.Install(context.Background(), e.Plan(needs), nil, false, f.recheck(needs))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.events, " "); got != "install:python refresh install:pymupdf refresh" {
		t.Errorf("events %q, want python, PATH read again, pymupdf, PATH read again", got)
	}
	if out.After == nil || program(t, *out.After, "pymupdf").State != mktapi.PrereqOK {
		t.Errorf("after: %+v, want pymupdf ok", out.After)
	}
	f2 := newFake()
	f2.avail = noPip
	e2 := &Engine{Runner: f2, Platform: "windows"}
	if _, err := e2.Install(context.Background(), e2.Plan(needs), []string{"pymupdf"}, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f2.userCalls, ","); got != "python,pymupdf" {
		t.Errorf("named pymupdf: user steps %q, want python then pymupdf", got)
	}
}

// On linux python is an administrator step: every administrator step still
// runs behind ONE prompt, before the pip step that needs it.
func TestInstall_AnAdministratorProviderKeepsOnePrompt(t *testing.T) {
	needs := append(pythonNeeds(true),
		Need{Sidecar: "docs", Name: "graphviz", Required: true, Checked: true, Recipes: []publish.InstallRecipe{apt("graphviz")}})
	f := newFake()
	f.avail = noPip
	e := &Engine{Runner: f, Platform: "linux"}
	out, err := e.Install(context.Background(), e.Plan(needs), nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.ElevationPrompts != 1 || len(f.elevated) != 1 {
		t.Fatalf("elevation prompts %d (runner %d), want 1", out.ElevationPrompts, len(f.elevated))
	}
	if got := strings.Join(f.events, " "); got != "elevated:graphviz,python refresh install:pymupdf refresh" {
		t.Errorf("events %q, want the one prompt, PATH read again, pymupdf", got)
	}
}

// A required program no recipe installs here (its installer absent) is not
// "nothing to install": the default install lists it skipped with its install
// skill and names it missing; once every required program is present, nothing
// is missing.
func TestInstall_ARequiredProgramNoRecipeInstallsStaysMissing(t *testing.T) {
	needs := []Need{
		{Sidecar: "a", Name: "pandoc", Required: true, Checked: true, InstallSkill: "ktw-docs:/skill/install-pandoc",
			Commands: []string{"pandoc"}, Recipes: []publish.InstallRecipe{winget("JohnMacFarlane.Pandoc")}},
		{Sidecar: "a", Name: "drawio", Checked: true, Recipes: []publish.InstallRecipe{winget("JGraph.Draw")}},
	}
	f := newFake()
	f.avail = func(string) bool { return false } // a clean Windows: no winget
	e := &Engine{Runner: f, Platform: "windows"}
	plan := e.Plan(needs)
	out, err := e.Install(context.Background(), plan, nil, false, f.recheck(needs))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 1 || out.Steps[0].Program != "pandoc" || out.Steps[0].Status != mktapi.StepSkipped ||
		!strings.Contains(out.Steps[0].Detail, "ktw-docs:/skill/install-pandoc") {
		t.Errorf("steps %+v, want pandoc skipped with its install skill (drawio, optional, not listed)", out.Steps)
	}
	if len(out.Missing) != 1 || out.Missing[0] != "pandoc" {
		t.Errorf("missing %v, want [pandoc]", out.Missing)
	}
	if len(f.userCalls)+len(f.elevated) != 0 {
		t.Errorf("nothing can run here, yet %v %v ran", f.userCalls, f.elevated)
	}

	needs[0].OK = true
	out, err = e.Install(context.Background(), e.Plan(needs), nil, false, f.recheck(needs))
	if err != nil || len(out.Missing) != 0 || len(out.Steps) != 0 {
		t.Errorf("every required program present: steps %+v missing %v err %v", out.Steps, out.Missing, err)
	}
}

// A winget step installs from the winget repository the recipe's id names:
// --source winget, so an unreachable Microsoft Store source (a Windows
// Sandbox, a locked-down PC) does not stop it.
func TestCommandLine_WingetNamesItsSource(t *testing.T) {
	argv, err := commandLine(Step{Program: "pandoc", Recipe: winget("JohnMacFarlane.Pandoc")})
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(argv, " ")
	for _, w := range []string{"winget install --id JohnMacFarlane.Pandoc --exact", "--source winget", "--scope user", "--accept-source-agreements"} {
		if !strings.Contains(line, w) {
			t.Errorf("the winget step should carry %q: %s", w, line)
		}
	}
}

// A winget step installs the version the sidecars are validated with - the
// check accepts only it (pandoc 3.11, where winget's latest is 3.12.1) - and
// the latest when none is declared.
func TestCommandLine_WingetInstallsTheValidatedVersion(t *testing.T) {
	argv, err := commandLine(Step{Program: "pandoc", Recipe: winget("JohnMacFarlane.Pandoc"), Version: "3.11"})
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.Join(argv, " "); !strings.Contains(line, "--exact --source winget --version 3.11 --scope user") {
		t.Errorf("the validated version should be asked: %s", line)
	}
	argv, _ = commandLine(Step{Program: "rclone", Recipe: winget("Rclone.Rclone")})
	if line := strings.Join(argv, " "); strings.Contains(line, "--version") {
		t.Errorf("no declared version: the latest, no --version: %s", line)
	}
	if s := step(mktapi.PlannedProgram{Name: "typst", Version: "0.15.1", Recipe: &publish.InstallRecipe{Kind: publish.RecipeWinget, Package: "Typst.Typst"}}); s.Version != "0.15.1" {
		t.Errorf("the planned version should reach the step: %+v", s)
	}
}

// A winget step that fails on a dependency only an administrator can install
// (typst's Visual C++ runtime, machine-wide) says what to do: the person, who
// is not an administrator, asks their help desk - in any language winget
// answers. Any other failure stays as it was.
func TestInstall_AMachineOnlyDependencyAsksTheHelpDesk(t *testing.T) {
	s := Step{Program: "typst", Recipe: winget("Typst.Typst"), Version: "0.15.1"}
	for lang, out := range map[string]string{
		"fr": "winget install --id Typst.Typst: exit status 0x8a150104: Trouvé Typst [Typst.Typst] Version 0.15.1\nAucun programme d’installation approprié n’a été trouvé pour le manifeste : Microsoft.VCRedist.2015+.x64 version 14.51.36247.0\nErreur lors du traitement des dépendances de package. Sortie...",
		"en": "winget install --id Typst.Typst: exit status 0x8a150104: Found Typst [Typst.Typst] Version 0.15.1\nNo applicable installer found for the manifest Microsoft.VCRedist.2015+.x64 version 14.51.36247.0\nError processing package dependencies.",
	} {
		msg := explainWinget(s, errors.New(out)).Error()
		for _, w := range []string{"typst needs the Microsoft Visual C++ 2015-2022 Redistributable (x64) (Microsoft.VCRedist.2015+.x64)", "only an administrator", "ask your help desk", "run the install again"} {
			if !strings.Contains(msg, w) {
				t.Errorf("%s: the message should say %q:\n%s", lang, w, msg)
			}
		}
	}
	other := errors.New("winget install --id Typst.Typst: exit status 0x8a150014: network unreachable")
	if got := explainWinget(s, other); got != other {
		t.Errorf("another failure should stay as it was: %v", got)
	}
}

// A winget recipe's custom arguments reach the installer (winget --custom).
func TestCommandLine_WingetPassesTheRecipesCustomArguments(t *testing.T) {
	r := winget("Python.Python.3.12")
	r.Custom = "InstallLauncherAllUsers=0 PrependPath=1"
	argv, err := commandLine(Step{Program: "python", Recipe: r})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(argv, "\x00"), "--custom\x00InstallLauncherAllUsers=0 PrependPath=1") {
		t.Errorf("argv %q does not pass the custom arguments", argv)
	}
}

// A failed step says in one line what went wrong and carries its full report:
// the command, its exit code as winget names it with what it means, all it
// printed and where winget keeps its logs (: Chrome's machine-only
// package asked for the user alone).
func TestExplain_AFailedCommandCarriesItsFullReport(t *testing.T) {
	ce := &CommandError{Command: "winget install --id Google.Chrome --exact --source winget --scope user", ExitCode: int(uint32(0x8A150010)),
		Output: "Found Google Chrome [Google.Chrome] Version 155.0\r\n  -  \r\nNo applicable installer found; see logs for more details.\r\n", Cause: errors.New("exit status 0x8a150010")}
	short, r := Explain(ce)
	if r == nil {
		t.Fatal("a command's failure should carry its report")
	}
	if r.Command != ce.Command || r.Code != "0x8A150010" || !strings.Contains(r.Meaning, "for the user only") ||
		!strings.Contains(r.Output, "No applicable installer found") || r.ExitCode != ce.ExitCode {
		t.Errorf("the report: %+v", r)
	}
	if !strings.Contains(short, "for the user only") || !strings.HasSuffix(short, "(0x8A150010)") || strings.Contains(short, "\n") {
		t.Errorf("the short message should be winget's meaning and its code, one line: %q", short)
	}

	// Explained in the person's terms, the failure keeps its report.
	dep := &CommandError{Command: "winget install --id Typst.Typst", ExitCode: int(uint32(0x8A150104)), Cause: errors.New("exit status"),
		Output: "No applicable installer found for the manifest Microsoft.VCRedist.2015+.x64 version 14.51\n"}
	short, r = Explain(explainWinget(Step{Program: "typst", Recipe: winget("Typst.Typst")}, dep))
	if r == nil || r.Command != dep.Command || !strings.Contains(short, "ask your help desk") {
		t.Errorf("an explained failure: %q, %+v", short, r)
	}

	// A failure that ran no command has no report.
	if short, r := Explain(errors.New("bun, which provides bun, did not install")); r != nil || short == "" {
		t.Errorf("no command, no report: %q %+v", short, r)
	}
}

// pip --user puts a program's commands in Python's user scripts folder, which
// no PATH holds by default: a pip step puts that folder on PATH so the check
// finds what was installed (: docling installed, still « À
// installer »). The folder is the one Python itself names.
func TestPipScriptsDir_IsPythonsUserScriptsFolder(t *testing.T) {
	pip := pipCommand()
	if len(pip) < 3 {
		t.Skip("no python here")
	}
	dir := pipScriptsDir(context.Background(), pip)
	if dir == "" || !filepath.IsAbs(dir) {
		t.Fatalf("the user scripts folder of %v: %q", pip, dir)
	}
	want := "bin"
	if runtime.GOOS == "windows" {
		want = "Scripts"
	}
	if filepath.Base(dir) != want {
		t.Errorf("%q is not a user scripts folder (…/%s)", dir, want)
	}
	if pipScriptsDir(context.Background(), []string{"pip"}) != "" {
		t.Error("pip alone names no Python: no folder")
	}
}
