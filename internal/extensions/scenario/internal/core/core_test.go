// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A case loads from its folder and refuses what it could not run: a step with
// no kind or two, a stage id used twice, no member.
func TestACaseLoadsAndIsChecked(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold(filepath.Join(dir, "demo"), "demo"); err != nil {
		t.Fatal(err)
	}
	c, err := api.Load(filepath.Join(dir, "demo"))
	if err != nil || c.Name != "demo" || c.UniverseName() != "scn-demo" || len(c.Stages) != 1 {
		t.Fatalf("scaffolded case = %+v, %v", c, err)
	}
	// Its seed module is of a type ktw-methods ships (documentation): the
	// module type is retired.
	raw, err := os.ReadFile(filepath.Join(dir, "demo", api.CaseFile))
	if err != nil || strings.Contains(string(raw), "type: module") || !strings.Contains(string(raw), "type: documentation") {
		t.Errorf("the scaffolded seed module's type: %v %s", err, raw)
	}
	for name, yaml := range map[string]string{
		"two kinds":  "name: x\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - id: a\n    steps:\n      - {call: {uri: a:b/C}, run: {cmd: x}}\n",
		"no kind":    "name: x\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - id: a\n    steps:\n      - {note: nothing}\n",
		"same id":    "name: x\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - {id: a}\n  - {id: a}\n",
		"no members": "name: x\nuniverse: {}\n",
		"bad name":   "name: a b\nuniverse: {sidecars: [ktw-agents]}\n",
	} {
		d := filepath.Join(dir, "bad")
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, api.CaseFile), []byte(yaml), 0o644)
		if _, err := api.Load(d); err == nil {
			t.Errorf("%s: loaded, want refused", name)
		}
	}
}

// Step inputs read saved values and the case's own names; a whole template
// keeps the value's type.
func TestInputsReadSavedValues(t *testing.T) {
	e := &Engine{Case: &api.Case{Name: "demo"}, RT: Runtime{Dir: "/tmp/demo"}}
	st := State{Saved: map[string]any{"g1": map[string]any{"object": map[string]any{"id": "todo.G-001", "n": 3.0}}}}
	got := e.expand(map[string]any{
		"id":    "${saved.g1.object.id}",
		"n":     "${saved.g1.object.n}",
		"note":  "goal ${saved.g1.object.id} in ${case.universe}",
		"list":  []any{"${case.name}"},
		"stays": "${saved.nope.x}",
	}, st).(map[string]any)
	if got["id"] != "todo.G-001" || got["n"] != 3.0 || got["note"] != "goal todo.G-001 in scn-demo" ||
		got["list"].([]any)[0] != "demo" || got["stays"] != "${saved.nope.x}" {
		t.Fatalf("expanded = %+v", got)
	}
}

// The simulated user's answer is read wherever its run put it.
func TestTheSimulatedAnswerIsRead(t *testing.T) {
	for name, tc := range map[string]struct {
		out      any
		decision string
		note     string
	}{
		"top":     {map[string]any{"decision": "rejected", "note": "use Bun"}, "rejected", "use Bun"},
		"nested":  {map[string]any{"result": map[string]any{"decision": "approved", "note": "ok"}}, "approved", "ok"},
		"json":    {`{"decision":"approved","note":"fine"}`, "approved", "fine"},
		"text":    {"Bun and TypeScript", "approved", "Bun and TypeScript"},
		"no word": {map[string]any{"note": "go"}, "approved", "go"},
	} {
		d, n := parseAnswer(tc.out)
		if d != tc.decision || n != tc.note {
			t.Errorf("%s: %q %q, want %q %q", name, d, n, tc.decision, tc.note)
		}
	}
}

// A checkpoint copies the case's whole changing state; restoring it puts back
// exactly that state (later changes gone). No universe is running here.
func TestACheckpointRestoresTheCaseState(t *testing.T) {
	root := t.TempDir()
	e := NewEngine(&api.Case{Name: "demo", Stages: []api.Stage{{ID: "s1"}, {ID: "s2"}}}, root, "", nil)
	write := func(rel, s string) {
		p := filepath.Join(e.RT.Dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("project/src/a.ts", "v1")
	write("data/sessions/x.jsonl", "one")
	_ = saveState(e.RT, State{Installed: true, Passed: []string{"s1"}})
	if err := e.Checkpoint(t.Context(), "s1"); err != nil {
		t.Fatal(err)
	}
	write("project/src/a.ts", "v2")
	write("project/src/b.ts", "new")
	_ = saveState(e.RT, State{Installed: true, Passed: []string{"s1", "s2"}})
	if err := e.Restore(t.Context(), e.checkpointBefore("s2")); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(e.RT.Project(), "src", "a.ts"))
	_, bErr := os.Stat(filepath.Join(e.RT.Project(), "src", "b.ts"))
	st, _ := loadState(e.RT)
	if string(a) != "v1" || bErr == nil || len(st.Passed) != 1 {
		t.Fatalf("after restore: a=%q, b present=%v, passed=%v; want v1, no b, [s1]", a, bErr == nil, st.Passed)
	}
	if got := e.checkpointBefore("s1"); got != CheckpointInstalled {
		t.Errorf("before the first stage = %q, want installed", got)
	}
}

// A copy entry puts one file, or a folder's selected files, into the
// universe's home; an optional source that is absent is skipped, a selector
// that keeps nothing is refused.
func TestCopyEntriesFillTheUniverse(t *testing.T) {
	root, caseDir := t.TempDir(), t.TempDir()
	for rel, s := range map[string]string{
		"config/TaskGroups.yaml": "g", "config/LLMProfiles.yaml": "p", "config/notes.md": "n",
		"config/sub/Extra.yaml": "x", "keys.env": "K=1",
	} {
		p := filepath.Join(caseDir, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(s), 0o644)
	}
	e := NewEngine(&api.Case{Name: "demo", Dir: caseDir}, root, "", nil)
	home := e.RT.UniverseHome()
	for _, tc := range []struct {
		cp   api.Copy
		want int
	}{
		{api.Copy{From: "config", To: "config", Select: []string{"*.yaml"}, Exclude: []string{"sub/*"}}, 2},
		{api.Copy{From: "keys.env", To: ".env"}, 1},
		{api.Copy{From: "config/notes.md", To: "docs/"}, 1},
		{api.Copy{From: "absent.env", To: ".env", Optional: true}, 0},
	} {
		if n, err := e.copyIn(tc.cp); err != nil || n != tc.want {
			t.Errorf("copy %+v = %d, %v; want %d", tc.cp, n, err, tc.want)
		}
	}
	for rel, want := range map[string]bool{
		"config/TaskGroups.yaml": true, "config/LLMProfiles.yaml": true, "config/notes.md": false,
		"config/sub/Extra.yaml": false, ".env": true, "docs/notes.md": true,
	} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(rel))); (err == nil) != want {
			t.Errorf("%s present = %v, want %v", rel, err == nil, want)
		}
	}
	for _, cp := range []api.Copy{
		{From: "absent.env", To: ".env"},
		{From: "config", To: "config", Select: []string{"*.json"}},
		{From: "keys.env", To: ".env", Select: []string{"*"}},
	} {
		if _, err := e.copyIn(cp); err == nil {
			t.Errorf("copy %+v: done, want refused", cp)
		}
	}
	for _, to := range []string{"../outside", "/abs", "config/../../x"} {
		d := t.TempDir()
		raw := "name: x\nuniverse:\n  sidecars: [ktw-agents]\n  copy:\n    - {from: a, to: \"" + to + "\"}\n"
		_ = os.WriteFile(filepath.Join(d, api.CaseFile), []byte(raw), 0o644)
		if _, err := api.Load(d); err == nil {
			t.Errorf("to %q: loaded, want refused", to)
		}
	}
}

// An installed case's binaries come from one folder, recorded beside them: a
// refresh takes the same ones, never whatever a default names.
func TestTheBinariesRememberTheirSource(t *testing.T) {
	src := t.TempDir()
	for _, b := range []string{hubBinary, "ktw-agents"} {
		_ = os.WriteFile(filepath.Join(src, exe(b)), []byte(b), 0o755)
	}
	e := NewEngine(&api.Case{Name: "demo", Universe: api.CaseUniverse{Sidecars: []string{"ktw-agents"}}}, t.TempDir(), src, nil)
	if got := BinSource(e.RT); got != "" {
		t.Fatalf("before install: source %q, want none", got)
	}
	if err := os.MkdirAll(e.RT.Bin(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.copyBinaries(); err != nil {
		t.Fatal(err)
	}
	if got := BinSource(e.RT); got != src {
		t.Fatalf("after install: source %q, want %q", got, src)
	}
}

// A listing is walked by a dot path (maps, then list indexes), its elements
// kept when each field of where contains its value; an each step's element is
// read as ${item.<path>}; find and each are checked down to their steps.
func TestListingsFindAndEach(t *testing.T) {
	v := map[string]any{"releases": []any{
		map[string]any{"id": "todo.R-001", "name": "1.0 — Core list"},
		map[string]any{"id": "todo.R-002", "name": "2.0 — Filters"},
	}}
	if got := walk(v, "releases.1.id"); got != "todo.R-002" {
		t.Fatalf("walk = %v, want todo.R-002", got)
	}
	if walk(v, "releases.9.id") != nil || walk(v, "nope.x") != nil {
		t.Error("walk found what is not there")
	}
	id := func(s string) string { return s }
	list := walk(v, "releases").([]any)
	if !matches(list[0], map[string]string{"name": "1.0"}, id) || matches(list[1], map[string]string{"name": "1.0"}, id) {
		t.Error("where 1.0 matched the wrong release")
	}
	if !matches(list[1], map[string]string{"name": "FILTERS"}, id) {
		t.Error("where is case-insensitive")
	}

	e := &Engine{Case: &api.Case{Name: "demo"}}
	st := State{Saved: map[string]any{eachItem: list[0]}}
	if got := e.expand(map[string]any{"on": "${item.id}"}, st).(map[string]any)["on"]; got != "todo.R-001" {
		t.Fatalf("${item.id} = %v", got)
	}

	for name, yaml := range map[string]string{
		"find saves nothing": "name: x\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - id: a\n    steps:\n      - {find: {uri: a:b/C, items: l}}\n",
		"each runs nothing":  "name: x\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - id: a\n    steps:\n      - {each: {uri: a:b/C, items: l}}\n",
		"bad inner step":     "name: x\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - id: a\n    steps:\n      - each: {uri: a:b/C, items: l, steps: [{note: none}]}\n",
	} {
		d := t.TempDir()
		_ = os.WriteFile(filepath.Join(d, api.CaseFile), []byte(yaml), 0o644)
		if _, err := api.Load(d); err == nil {
			t.Errorf("%s: loaded, want refused", name)
		}
	}
}

// The GUI launcher starts the case's universe as the scenario runs it, then
// opens ktw-gui on that universe; it names the case's own folders.
func TestTheLauncherOpensTheCaseInTheGUI(t *testing.T) {
	root, cases := t.TempDir(), t.TempDir()
	e := NewEngine(&api.Case{Name: "demo"}, root, "", nil)
	path, err := e.WriteLauncher(cases)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != e.RT.Dir || filepath.Base(path) != LauncherName() {
		t.Fatalf("launcher at %s, want %s in %s", path, LauncherName(), e.RT.Dir)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	sb := e.Sandbox()
	for _, want := range []string{"scenario start demo", "scenario stop demo", "--universe scn-demo", e.RT.Bin(), e.RT.Universe(), e.RT.Data(), cases, sb.Root(), sb.Cache()} {
		if !strings.Contains(s, want) {
			t.Errorf("launcher lacks %q:\n%s", want, s)
		}
	}
	gui, start, at := strings.Index(s, "--universe scn-demo"), strings.Index(s, "scenario start"), strings.Index(s, sb.Root())
	if start > gui {
		t.Error("the GUI opens before the universe is started")
	}
	// The sandbox's world is set for the GUI only: `scenario start` enters the
	// sandbox itself, from the installation it records as its host.
	if at < start || at > gui {
		t.Error("the sandbox's global root is not set between starting the universe and opening the GUI")
	}
	if runtime.GOOS == "windows" && !strings.Contains(s, `set "KTW_DATA="`) {
		t.Error("the GUI may inherit a KTW_DATA, which would override every universe's data folder")
	}
}

func TestOnlyAHubFromTheCaseBinIsTheCasesOwn(t *testing.T) {
	e := NewEngine(&api.Case{Name: "demo"}, t.TempDir(), "", nil)
	if !e.isOwnHub(filepath.Join(e.RT.Bin(), "ktw-hub.exe")) {
		t.Error("the case's own hub is taken for a foreign one")
	}
	if e.isOwnHub(filepath.Join(filepath.Dir(e.RT.Bin()), "other", "ktw-hub.exe")) {
		t.Error("a hub from outside the case (another installation's control plane) is taken for the case's own")
	}
	if !e.isOwnHub(filepath.Join(e.Sandbox().ControlPlane(), "ktw-hub.exe")) {
		t.Error("the hub a GUI spawns from the case's sandbox is taken for a foreign one")
	}
	if !e.isOwnHub("") {
		t.Error("a hub whose path cannot be told is replaced on a guess")
	}
}

// A user file's front matter scripts the person's answers: read at load, each
// needing a decision, a rejection its note; the persona text below it is kept
// for the agent path.
func TestTheUserFileScriptsAnswers(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold(filepath.Join(dir, "demo"), "demo"); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(dir, "demo", "user.md")
	_ = os.WriteFile(user, []byte("---\nanswers:\n  - {when: colour, decision: approved, note: blue}\n  - {decision: rejected, note: not yet}\n---\n# The person\n"), 0o644)
	c, err := api.Load(filepath.Join(dir, "demo"))
	if err != nil || len(c.Answers) != 2 || c.Answers[0].When != "colour" || c.Answers[1].Decision != "rejected" {
		t.Fatalf("answers = %+v, %v", c.Answers, err)
	}
	_ = os.WriteFile(user, []byte("---\nanswers:\n  - {when: colour, note: blue}\n---\n"), 0o644)
	if _, err := api.Load(filepath.Join(dir, "demo")); err == nil {
		t.Error("an answer without a decision loaded, want refused")
	}
	_ = os.WriteFile(user, []byte("---\nanswers:\n  - {when: review, decision: rejected}\n---\n"), 0o644)
	if _, err := api.Load(filepath.Join(dir, "demo")); err == nil {
		t.Error("a rejection without a note loaded, want refused: the inbox refuses it")
	}
	_ = os.WriteFile(user, []byte("# Only a persona\n"), 0o644)
	if c, err := api.Load(filepath.Join(dir, "demo")); err != nil || len(c.Answers) != 0 {
		t.Errorf("a persona without answers = %+v, %v; want none", c.Answers, err)
	}
}

// A question takes the first scripted answer not given yet whose `when` it
// contains (any case; an empty `when` takes any question); none left is -1.
func TestAScriptedAnswerIsGivenOnce(t *testing.T) {
	answers := []api.UserAnswer{{When: "Colour", Decision: "approved"}, {Decision: "rejected"}, {When: "size", Decision: "approved"}}
	for name, tc := range map[string]struct {
		used     []int
		question string
		want     int
	}{
		"matches":         {nil, "Which colour?", 0},
		"used goes next":  {[]int{0}, "Which colour?", 1},
		"any after when":  {nil, "Ship it?", 1},
		"all used":        {[]int{0, 1}, "Ship it?", -1},
		"specific later":  {[]int{1}, "What size?", 2},
		"none left of it": {[]int{0, 1, 2}, "What size?", -1},
	} {
		if got := scriptedAnswer(answers, tc.used, tc.question); got != tc.want {
			t.Errorf("%s: %d, want %d", name, got, tc.want)
		}
	}
}

// A stage's budget counts the work it made run — a person's, the planner's, an
// agent's, a workflow's — never the universe's own runs (the planner machine,
// started again at each restart; what it keeps alive).
func TestTheBudgetCountsWorkRuns(t *testing.T) {
	o := func(kind string) *taskapi.RunOrigin { return &taskapi.RunOrigin{Kind: kind} }
	got := workRuns([]*taskapi.RunOrigin{
		o(taskapi.OriginPerson), o(taskapi.OriginPlanner), o(taskapi.OriginAgent), o(taskapi.OriginWorkflow), nil,
		o(taskapi.OriginSystem), o(taskapi.OriginKeepAlive),
	})
	if got != 5 {
		t.Fatalf("work runs = %d, want 5 (system and keep-alive left out)", got)
	}
}

// The leak check sees a write into the watched files of an installation —
// its registry, a universe's config, a hub lockfile, its cache — and nothing
// of a universe's working files.
func TestTheLeakCheckSeesAWriteOutsideTheSandbox(t *testing.T) {
	base := t.TempDir()
	host := application.MachineDirs{Root: filepath.Join(base, "root"), Data: filepath.Join(base, "data"), Cache: filepath.Join(base, "cache")}
	write := func(rel, s string) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("root/universes.yaml", "universes: []")
	write("root/universes/admin/.katyoti/config/Univers.yaml", "name: admin")
	write("data/universes/dev/hub/hub.lock", "{}")
	write("cache/sidecars/x.json", "{}")
	g := WatchLeaks(host)
	write("root/universes/admin/tasks/run.log", "a working file") // not watched
	if err := g.Check(); err != nil {
		t.Fatalf("no watched file changed, the check says: %v", err)
	}
	for _, rel := range []string{"root/universes.yaml", "root/universes/scn-x/.katyoti/config/Univers.yaml", "data/universes/dev/hub/hub.lock", "cache/sidecars/y.json"} {
		g := WatchLeaks(host)
		write(rel, "written by the case, longer than before")
		err := g.Check()
		if err == nil || !strings.Contains(err.Error(), filepath.Join(base, filepath.FromSlash(rel))) {
			t.Errorf("a write into %s: %v, want a leak naming it", rel, err)
		}
	}
}

// Reinstalling a case clears its previous install but keeps its sandbox's
// machine, where the command itself runs.
func TestAReinstallKeepsTheSandbox(t *testing.T) {
	e := NewEngine(&api.Case{Name: "demo"}, t.TempDir(), "", nil)
	for _, rel := range []string{"machine/root/universes.yaml", "machine/data/tool/logs/x.log", "project/a.ts", "state.json", "checkpoints/installed/state.json"} {
		p := filepath.Join(e.RT.Dir, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}
	if err := e.clearRuntime(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(e.RT.Dir)
	if len(entries) != 1 || entries[0].Name() != "machine" || !fileExists(filepath.Join(e.RT.Dir, "machine", "data", "tool", "logs", "x.log")) {
		t.Fatalf("after clearing: %v, want the machine folder alone, whole", entries)
	}
}

// A case whose runtime folder holds only its sandbox's machine folder (made
// by the command running its first install) was never installed; anything
// beside it is a previous install.
func TestOnlyTheMachineFolderIsNotAPreviousInstall(t *testing.T) {
	e := NewEngine(&api.Case{Name: "demo"}, t.TempDir(), "", nil)
	if e.installedBefore() {
		t.Fatal("a missing runtime folder reads as installed")
	}
	_ = os.MkdirAll(filepath.Join(e.RT.Dir, "machine", "root", "universes", "admin"), 0o755)
	if e.installedBefore() {
		t.Fatal("a runtime folder with only its machine folder reads as installed: a first install is refused")
	}
	_ = os.WriteFile(filepath.Join(e.RT.Dir, "state.json"), []byte("{}"), 0o644)
	if !e.installedBefore() {
		t.Fatal("a runtime folder with a state.json reads as never installed: a second install goes on without --force")
	}
}

// The control plane is copied once: an unchanged binary is not copied again,
// a changed one is.
func TestAnUnchangedBinaryIsNotCopiedAgain(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.exe"), filepath.Join(dir, "cp", "dst.exe")
	_ = os.WriteFile(src, []byte("v1"), 0o755)
	if err := copyIfChanged(src, dst); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(dst, []byte("v9"), 0o755) // same size: only a copy that ran would put v1 back
	info, _ := os.Stat(src)
	_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	if err := copyIfChanged(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "v9" {
		t.Fatalf("an unchanged binary was copied again (%q)", b)
	}
	later := info.ModTime().Add(time.Minute)
	_ = os.Chtimes(src, later, later)
	if err := copyIfChanged(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "v1" {
		t.Fatalf("a changed binary was not copied (%q)", b)
	}
}

// A call check holds when the answer contains `contains` and, with
// `excludes`, does not contain it: a member that must be absent.
func TestACallCheckCanExclude(t *testing.T) {
	answer := `{"names":["ktw-agents","ktw-acme"]}`
	for _, tc := range []struct {
		c    api.CallCheck
		want bool
	}{
		{api.CallCheck{Contains: "ktw-acme"}, true},
		{api.CallCheck{Contains: "ktw-acme", Excludes: "ktw-demo"}, true},
		{api.CallCheck{Excludes: "ktw-demo"}, true},
		{api.CallCheck{Contains: "ktw-agents", Excludes: "ktw-acme"}, false},
		{api.CallCheck{Contains: "ktw-demo"}, false},
	} {
		if got := tc.c.Holds(answer); got != tc.want {
			t.Errorf("%+v holds = %v, want %v", tc.c, got, tc.want)
		}
	}
}

// `scenario gui <case> --open <address>`: the launcher starts ktw-gui on the
// address, in the case's universe when the address names none; an address
// the GUI could not open is refused before anything is written
func TestTheLauncherOpensTheGUIOnAnAddress(t *testing.T) {
	root, cases := t.TempDir(), t.TempDir()
	e := NewEngine(&api.Case{Name: "demo"}, root, "", nil)
	path, err := e.WriteLauncherOpen(cases, "object/m.F-001/links")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if s := string(b); !strings.Contains(s, "--universe scn-demo --open ") || !strings.Contains(s, "scn-demo/object/m.F-001/links") {
		t.Errorf("the GUI should start on the address in the case's universe:\n%s", s)
	}
	if _, err := e.WriteLauncherOpen(cases, "object"); err == nil || !strings.Contains(err.Error(), "--open") {
		t.Errorf("an address that opens nothing: %v", err)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "--open \"scn-demo/object\"") || strings.Contains(string(b), "--open 'scn-demo/object'") {
		t.Error("a refused address was written")
	}
}

// : the installation's own activity during a verb is not the
// case's - a cache file rewritten with the same content is no leak, a hub lock
// held by a process outside the case's folders (a person's hub) and a lock
// removed (a hub stopping) are notes; a cache file changed, and a lock held
// from the case's own folders, are leaks.
func TestTheLeakCheckDoesNotBlameTheInstallationsOwnActivity(t *testing.T) {
	base := t.TempDir()
	host := application.MachineDirs{Root: filepath.Join(base, "root"), Data: filepath.Join(base, "data"), Cache: filepath.Join(base, "cache")}
	write := func(rel, s string) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lock := func(universe string) {
		dir := filepath.Join(host.Data, "universes", universe)
		if err := hubclient.WriteLockFile(dir, hubclient.LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:1"}); err != nil {
			t.Fatal(err)
		}
	}
	write("cache/katyoti/sidecars/ktw-docs@1/manifest.json", `{"name":"ktw-docs"}`)
	write("root/universes.yaml", "universes: []")
	self, _ := os.Executable()
	elsewhere := filepath.Join(base, "case-bin")

	// A rescan: the same content written again, later.
	g := WatchLeaks(host, elsewhere)
	time.Sleep(20 * time.Millisecond)
	write("cache/katyoti/sidecars/ktw-docs@1/manifest.json", `{"name":"ktw-docs"}`)
	if err := g.Check(); err != nil {
		t.Fatalf("a cache file rewritten with the same content: %v", err)
	}

	// A person's hub starting (its process is not the case's), then stopping.
	g = WatchLeaks(host, elsewhere)
	lock("dev")
	if err := g.Check(); err != nil {
		t.Fatalf("a hub lock held outside the case's folders: %v", err)
	}
	if len(g.Notes) != 1 || !strings.Contains(g.Notes[0], "not the case's") {
		t.Fatalf("notes = %v, want the hub lock said", g.Notes)
	}
	g = WatchLeaks(host, elsewhere)
	_ = os.Remove(hubclient.LockFilePath(filepath.Join(host.Data, "universes", "dev")))
	if err := g.Check(); err != nil || len(g.Notes) != 1 {
		t.Fatalf("a hub stopping: %v, notes %v", err, g.Notes)
	}

	// The case's own hub on a universe of the installation, and a cache file
	// changed: leaks.
	g = WatchLeaks(host, filepath.Dir(self))
	lock("admin")
	write("cache/katyoti/sidecars/ktw-docs@1/manifest.json", `{"name":"ktw-docs","changed":true}`)
	err := g.Check()
	for _, want := range []string{filepath.Join("admin", "hub", "hub.lock"), "manifest.json"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("the case's own writes: %v, want a leak naming %s", err, want)
		}
	}
}

// : stages run on an installed universe reach its hub through
// the installation's data folder and name it - not the case's scn- universe -
// when it is not running; a stage answering the Inbox gives the universe the
// simulated user's task, as a scenario universe has it, and the user file is
// the persona.
func TestStagesRunOnAnInstalledUniverse(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "universes", "archifactory")
	u := InstalledUniverse{Name: "archifactory", Home: home, Data: filepath.Join(dir, "data", "universes", "archifactory"),
		Project: filepath.Join(dir, "Documents", "Katyoti", "archifactory"), Work: filepath.Join(dir, "results", "case-study")}
	stages := []api.Stage{{ID: "s5", Steps: []api.Step{{AnswerInbox: &api.AnswerInboxStep{Max: 1}}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := RunStagesOn(ctx, "study", stages, "I am the architect.", u, nil)
	if err == nil || !strings.Contains(err.Error(), "stage s5") || !strings.Contains(err.Error(), "archifactory is not running") {
		t.Fatalf("no hub on the installed universe: %v, want stage s5 saying archifactory is not running", err)
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(home).HomeDir(), "tasks", SimUserTask+".md")); err != nil {
		t.Errorf("the simulated user's task is not in the installed universe: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(u.Work, "user.md")); err != nil || string(b) != "I am the architect." {
		t.Errorf("the persona: %q %v", b, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(u.Work, "runs")); len(entries) != 1 {
		t.Errorf("the stage's report: %v, want one in the work folder", entries)
	}
}

// : a case's universe gets the tags, GUI and first guide its
// case declares, as its solution's install writes them; a case declaring none
// leaves Univers.yaml as it is.
func TestACaseUniverseCarriesTheCasesTags(t *testing.T) {
	home := t.TempDir()
	if err := orchcatalog.SaveManifest(home, &orchcatalog.Manifest{Version: 1, Name: "scn-study", DisplayName: "Scenario study"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml"))
	if err := setUniverseDisplay(home, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")); string(after) != string(before) {
		t.Fatal("a case declaring nothing changed Univers.yaml")
	}
	gui := &cenv.UniverseGUI{}
	if err := setUniverseDisplay(home, []string{"archifactory"}, gui, "ktw-acme:/guide-recipe/usage"); err != nil {
		t.Fatal(err)
	}
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tags) != 1 || m.Tags[0] != "archifactory" || m.GUI == nil || m.FirstGuide != "ktw-acme:/guide-recipe/usage" || m.Name != "scn-study" {
		t.Fatalf("the universe after install: %+v", m)
	}
}

// : a case declaring no_welcome creates its universe with the
// welcome screens off; a case declaring nothing keeps them, as before.
func TestACaseUniverseCanStartWithoutWelcomeScreens(t *testing.T) {
	c := api.Case{Name: "study", Universe: api.CaseUniverse{NoWelcome: true}}
	in := caseUniverseInput(c, "scn-study", "grape", "/home/scn-study")
	if !in.NoWelcome || in.Name != "scn-study" || in.DisplayName != "Scenario study" || in.HomePath != "/home/scn-study" {
		t.Fatalf("the case universe's creation: %+v", in)
	}
	if caseUniverseInput(api.Case{Name: "plain"}, "scn-plain", "grape", "/h").NoWelcome {
		t.Fatal("a case declaring nothing turned the welcome screens off")
	}
}
