// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// fakeActions answers a check with a canned review result and starts a task
// by naming a run; it records every call.
type fakeActions struct {
	mu      sync.Mutex
	calls   []actionsapi.RunInput
	result  api.ReviewResult
	answers map[string]any // an action URI's own answer, in place of result
}

func (f *fakeActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (f *fakeActions) Run(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	if strings.Contains(in.URI, ":tasks/") {
		return actionsapi.RunOutput{URI: in.URI, TaskID: "task-1"}, nil
	}
	var b []byte
	if a, ok := f.answers[in.URI]; ok {
		b, _ = json.Marshal(a)
	} else {
		b, _ = json.Marshal(f.result)
	}
	return actionsapi.RunOutput{URI: in.URI, Value: b}, nil
}

func (f *fakeActions) extension() application.Extension {
	return publish.PublishSimpleService[actionsapi.ActionService](
		func(context.Context) (actionsapi.ActionService, error) { return f, nil })
}

func findingIDs(t *testing.T, w world, typ string) map[string]cwmapi.Object {
	t.Helper()
	out, err := w.cwm.ObjectList(w.ctx, cwmapi.ObjectListInput{Parent: "m", Type: api.FindingType})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]cwmapi.Object{}
	for _, o := range out.Objects {
		full, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: o.ID})
		if err != nil {
			t.Fatal(err)
		}
		got[o.ID] = full.Object
	}
	return got
}

func stateOf(t *testing.T, w world, id string) string {
	t.Helper()
	got, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := got.Object.Lifecycle(); ok {
		return l.State
	}
	return ""
}

// Filing a review type's result twice: the second filing finds a finding seen
// before by its fingerprint (the same object, seen-in moved), files the new
// one and closes the one no longer seen; a manual review waits in-review for a
// person, an automatic one is closed; the third filing folds the first.
func TestFilingAResultFollowsItsFindings(t *testing.T) {
	w := setup(t)
	first := api.ReviewResult{Rating: "needs-attention", Summary: "two problems", Report: "# Security\n\nTwo problems.",
		Findings: []api.ResultFinding{
			{Severity: "high", Statement: "Open shutdown", Location: "server.go:10", Key: "unauthenticated-endpoint"},
			{Severity: "medium", Statement: "World-readable output", Location: "out.go:3", Key: "file-permissions"},
		}}
	one, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Trigger: api.TriggerManual, RequestedBy: "alice", Result: first})
	if err != nil {
		t.Fatal(err)
	}
	if len(one.New) != 2 || len(one.Persisting)+len(one.Resolved) != 0 {
		t.Fatalf("first filing: %+v", one)
	}
	if got := stateOf(t, w, one.Review); got != "in-review" {
		t.Errorf("a manual review waits for a person: %s", got)
	}
	rv, _ := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: one.Review})
	if rv.Object.Parent != "m" || !strings.Contains(rv.Object.Body, "Two problems.") || !strings.HasPrefix(strings.TrimPrefix(rv.Object.ID, "m."), "RV-") {
		t.Errorf("the review under the object with its report: %+v", rv.Object)
	}

	second := api.ReviewResult{Rating: "needs-attention", Summary: "one left, one new", Report: "r2",
		Findings: []api.ResultFinding{
			{Severity: "critical", Statement: "Open shutdown, still", Location: "server.go:10", Key: "Unauthenticated-Endpoint"},
			{Severity: "low", Statement: "Verbose logs", Location: "log.go:1", Key: "logging"},
		}}
	two, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Trigger: api.TriggerAutomatic, RequestedBy: "m.RQ-001", Result: second})
	if err != nil {
		t.Fatal(err)
	}
	if len(two.New) != 1 || len(two.Persisting) != 1 || len(two.Resolved) != 1 {
		t.Fatalf("second filing: %+v", two)
	}
	if got := stateOf(t, w, two.Review); got != "closed" {
		t.Errorf("an automatic review is closed: %s", got)
	}
	fs := findingIDs(t, w, "security")
	if len(fs) != 3 {
		t.Fatalf("three findings under the module, no copy: %v", fs)
	}
	kept := fs[two.Persisting[0]]
	if kept.EdgeTo(api.EdgeFoundIn) != one.Review || kept.EdgeTo(api.EdgeSeenIn) != two.Review || kept.Fields["severity"] != "critical" {
		t.Errorf("the finding seen again: %+v", kept)
	}
	if stateOf(t, w, two.Resolved[0]) != api.FindingClosed {
		t.Errorf("the finding not seen again is closed")
	}

	// What the fold will change, its files resolved while they all exist: the
	// first review (removed), the findings that point at it (detached) and
	// the module (its history).
	var pointing []string
	for id, f := range findingIDs(t, w, "security") {
		for _, e := range f.Edges {
			if e.To == one.Review {
				pointing = append(pointing, id)
				break
			}
		}
	}
	sort.Strings(pointing)
	if len(pointing) != 2 {
		t.Fatalf("two findings point at the first review: %v", pointing)
	}
	want := map[string]string{one.Review: relPath(t, w, one.Review), "m": relPath(t, w, "m")}
	for _, id := range pointing {
		want[id] = relPath(t, w, id)
	}

	three, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Result: api.ReviewResult{Rating: "good", Report: "r3"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(three.Supersede.Folded, ",") != one.Review || len(three.Resolved) != 2 {
		t.Errorf("the third filing folds the first and resolves what is left: %+v", three)
	}
	// A commit by path of what the filing reports carries the fold whole:
	// the removed review, the findings whose links to it were cut, the
	// history.
	if strings.Join(three.Supersede.Removed, ",") != one.Review || strings.Join(three.Supersede.Detached, ",") != strings.Join(pointing, ",") ||
		three.Supersede.History != "m" {
		t.Errorf("the fold names what it removed, detached and wrote the history of: %+v", three.Supersede)
	}
	want[three.Review] = relPath(t, w, three.Review)
	files := map[string]bool{}
	for _, f := range three.Files {
		files[f] = true
	}
	for id, p := range want {
		if !files[p] {
			t.Errorf("the filing's files lack %s (%s): %v", p, id, three.Files)
		}
	}
	for _, p := range three.Supersede.Files {
		if !files[p] {
			t.Errorf("the filing's files lack the fold's %s: %v", p, three.Files)
		}
	}
}

// relPath is the object's file relative to the project, as a filing reports it.
func relPath(t *testing.T, w world, id string) string {
	t.Helper()
	got, err := w.cwm.ObjectResolve(w.ctx, cwmapi.ObjectResolveInput{Ref: id})
	if err != nil {
		t.Fatalf("resolve %s: %v", id, err)
	}
	rel, err := filepath.Rel(got.ProjectRoot, got.Path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

// Start runs what is chosen: a check type is called and filed at once, an
// agent type starts the review-run task with the type's skill; a run over the
// cost threshold is only estimated until confirmed; the Reviews panel offers
// the types by dimension and the presets.
func TestStartRunsTheChosenReviewTypes(t *testing.T) {
	fake := &fakeActions{result: api.ReviewResult{Rating: "good", Report: "all builds",
		Findings: []api.ResultFinding{{Severity: "low", Statement: "a vet warning", Location: "a.go:1", Key: "vet"}}}}
	w := setupWith(t, fstest.MapFS{
		"review-types/checks.yaml":   {Data: []byte("name: checks\ntitle: Build and tests\ndimension: quality\napplies: {types: [module]}\nrunner: {check: 'ktw-test:fixture/check'}\npresets: [code]\n")},
		"review-types/security.yaml": {Data: []byte("name: security\ntitle: Security\ndimension: security\napplies: {types: [module]}\nrunner: {skill: 'ktw-code:/skill/code-review', input: {guide: security}}\nmax_cost_usd: 2\npresets: [code]\n")},
		"review-types/threats.yaml":  {Data: []byte("name: threats\ntitle: Threat model\ndimension: security\napplies: {types: [module]}\nrunner: {skill: 'ktw-code:/skill/code-review'}\nmax_cost_usd: 4\n")},
	}, fake.extension())

	out, err := w.svc.Start(w.ctx, api.StartInput{Object: "m", Preset: "code", RequestedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if out.NeedsConfirm || len(out.Started) != 2 || out.EstimateUSD != 2 {
		t.Fatalf("the code preset: %+v", out)
	}
	byType := map[string]api.StartedType{}
	for _, s := range out.Started {
		byType[s.Type] = s
	}
	if byType["checks"].Review == "" || byType["checks"].Error != "" {
		t.Errorf("the check is filed at once: %+v", byType["checks"])
	}
	if c := byType["checks"]; c.Review != "" && !slices.Contains(c.Files, relPath(t, w, c.Review)) {
		t.Errorf("the check names the files its filing wrote, its review's among them: %+v", c)
	}
	if byType["security"].TaskID != "task-1" {
		t.Errorf("the agent type starts its run: %+v", byType["security"])
	}
	var task actionsapi.RunInput
	for _, c := range fake.calls {
		if strings.HasSuffix(c.URI, ":tasks/review-run") {
			task = c
		}
	}
	var in map[string]any
	_ = json.Unmarshal(task.Input, &in)
	if in["skill"] != "ktw-code:/skill/code-review" || in["type"] != "security" || in["object"] != "m" || in["run"] != out.Run {
		t.Errorf("review-run's input: %s", task.Input)
	}

	over, err := w.svc.Start(w.ctx, api.StartInput{Object: "m", Dimension: "security"})
	if err != nil || !over.NeedsConfirm || over.EstimateUSD != 6 || len(over.Started) != 0 {
		t.Fatalf("over the threshold, only estimated: %+v %v", over, err)
	}
	if over, _ = w.svc.Start(w.ctx, api.StartInput{Object: "m", Dimension: "security", Confirm: true}); len(over.Started) != 2 {
		t.Errorf("confirmed, it starts: %+v", over)
	}
	if _, err := w.svc.Start(w.ctx, api.StartInput{Object: "m", Types: []string{"nope"}}); err == nil {
		t.Error("a choice no type matches is refused")
	}

	list, err := w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	page := asJSON(t, list)
	for _, want := range []string{`"title":"Start a review"`, `"uri":"ktw-test:reviews/start"`, `"preset":"code"`, `"dimension":"security"`,
		`"types":["threats"]`, `"confirm":"This review may cost up to $6.00. Start it?"`} {
		if !strings.Contains(page, want) {
			t.Errorf("Reviews page lacks %s:\n%s", want, page)
		}
	}
}

// A review compared with the previous one of its type: its rating change, its
// findings new, persisting and resolved; the Reviews panel's scorecard shows
// each type's timeline — the reviews folded into the history and those kept.
func TestAReviewComparesWithThePreviousAndTheTimelineShowsProgress(t *testing.T) {
	w := setupWith(t, fstest.MapFS{
		"review-types/security.yaml": {Data: []byte("name: security\ntitle: Security\ndimension: security\nrunner: {skill: x}\n")},
	})
	score := func(n int) *int { return &n }
	file := func(rating string, sc int, fs ...api.ResultFinding) api.FileOutput {
		out, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Trigger: api.TriggerAutomatic,
			Result: api.ReviewResult{Rating: rating, Score: score(sc), Report: "r", Findings: fs}})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := api.ResultFinding{Severity: "high", Statement: "A", Location: "a.go", Key: "a"}
	b := api.ResultFinding{Severity: "medium", Statement: "B", Location: "b.go", Key: "b"}
	c := api.ResultFinding{Severity: "low", Statement: "C", Location: "c.go", Key: "c"}
	first := file("critical", 30, a, b)
	second := file("needs-attention", 55, a, c)

	one, err := w.svc.Review(w.ctx, api.ObjectInput{ID: second.Review})
	if err != nil {
		t.Fatal(err)
	}
	page := asJSON(t, one)
	for _, want := range []string{`"title":"Since the previous review"`, `"text":"1 new"`, `"text":"1 persisting"`, `"text":"1 resolved"`,
		`score 30 → 55 (+25)`, `"navigate":{"object":"` + first.Review + `"`, `"navigate":{"object":"` + second.Resolved[0] + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the comparison lacks %s:\n%s", want, page)
		}
	}
	first1, _ := w.svc.Review(w.ctx, api.ObjectInput{ID: first.Review})
	if page := asJSON(t, first1); !strings.Contains(page, "The first review of its type.") {
		t.Errorf("the first review has no previous one:\n%s", page)
	}

	file("good", 90)
	list, err := w.svc.ReviewsOf(w.ctx, api.ObjectInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	page = asJSON(t, list)
	for _, want := range []string{`"title":"Scorecard"`, `"text":"security"`, ` · 30 · 0"`, ` · 55 · 0"`, ` · 90 · 0"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the scorecard lacks %s:\n%s", want, page)
		}
	}
}

// An automatic run follows the change: only the automatic types whose kinds
// hold the request's kind and whose path globs match a path it touched; none
// matching, nothing starts and nothing is refused.
func TestAnAutomaticRunFollowsTheRequestsChange(t *testing.T) {
	fake := &fakeActions{result: api.ReviewResult{Rating: "good", Report: "ok"}}
	w := setupWith(t, fstest.MapFS{
		"review-types/go.yaml":   {Data: []byte("name: go\ntitle: Go checks\ndimension: quality\napplies: {types: [module]}\ntrigger: {mode: automatic, kinds: [build, fix], paths: ['*.go']}\nrunner: {check: 'ktw-test:fixture/check'}\n")},
		"review-types/docs.yaml": {Data: []byte("name: docs\ntitle: Docs checks\ndimension: documentation\napplies: {types: [module]}\ntrigger: {mode: automatic, kinds: [build], paths: ['*.md']}\nrunner: {check: 'ktw-test:fixture/check'}\n")},
	}, fake.extension())
	started := func(in api.StartInput) []string {
		t.Helper()
		in.Object, in.Trigger = "m", api.TriggerAutomatic
		out, err := w.svc.Start(w.ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		var types []string
		for _, s := range out.Started {
			types = append(types, s.Type)
		}
		return types
	}
	if got := started(api.StartInput{Kind: "fix", Paths: []string{"internal/a/b.go"}}); !slices.Equal(got, []string{"go"}) {
		t.Errorf("a fix touching Go: %v; want go only", got)
	}
	if got := started(api.StartInput{Kind: "build", Paths: []string{"modules/m/concepts.md"}}); !slices.Equal(got, []string{"docs"}) {
		t.Errorf("a build touching a page: %v; want docs only", got)
	}
	if got := started(api.StartInput{Kind: "analyse", Paths: []string{"a.go"}}); len(got) != 0 {
		t.Errorf("an analysis: %v; want nothing started", got)
	}
}

// An automatic review of a change hands the check the paths the change
// touched, so the check scopes to them.
func TestAChangesReviewHandsItsPathsToTheCheck(t *testing.T) {
	fake := &fakeActions{result: api.ReviewResult{Rating: "good", Report: "ok"}}
	w := setupWith(t, fstest.MapFS{
		"review-types/t-checks.yaml": {Data: []byte("name: t-checks\ntitle: Checks\ndimension: quality\napplies: {types: [module]}\ntrigger: {mode: automatic, kinds: [fix]}\nrunner: {check: 'ktw-test:fixture/check'}\n")},
	}, fake.extension())
	if _, err := w.svc.Start(w.ctx, api.StartInput{Object: "m", Types: []string{"t-checks"}, Trigger: api.TriggerAutomatic,
		Kind: "fix", Paths: []string{"internal/m/a.go"}}); err != nil {
		t.Fatal(err)
	}
	var in map[string]any
	for _, c := range fake.calls {
		if c.URI == "ktw-test:fixture/check" {
			_ = json.Unmarshal(c.Input, &in)
		}
	}
	if paths, _ := in["paths"].([]any); len(paths) != 1 || paths[0] != "internal/m/a.go" {
		t.Errorf("the check's input: %v", in)
	}
}

// An automatic review of a change touching Go files of two modules is filed
// on each owning module - its owners answered by the type's owners action -
// with that module's paths; the module the run was started on and owning
// none of them is not reviewed.
func TestAnAutomaticRunReviewsEachModuleOwningTheChange(t *testing.T) {
	fake := &fakeActions{result: api.ReviewResult{Rating: "good", Report: "ok"}, answers: map[string]any{
		"ktw-test:facts/change-owners": api.ChangeOwnersOutput{Owners: []api.ChangeOwner{
			{Object: "a", Paths: []string{"internal/a/x.go"}}, {Object: "b", Paths: []string{"internal/b/y.go"}}}},
	}}
	w := setupWith(t, fstest.MapFS{
		"review-types/go-checks.yaml": {Data: []byte("name: go-checks\ntitle: Go checks\ndimension: quality\napplies: {types: [module]}\ntrigger: {mode: automatic, kinds: [fix], owners: 'ktw-test:facts/change-owners'}\nrunner: {check: 'ktw-test:fixture/check'}\n")},
	}, fake.extension())
	cwmapp.Module(t, w.ctx, w.cwm, "a")
	cwmapp.Module(t, w.ctx, w.cwm, "b")

	out, err := w.svc.Start(w.ctx, api.StartInput{Object: "m", Trigger: api.TriggerAutomatic, Kind: "fix",
		Paths: []string{"internal/a/x.go", "internal/b/y.go", "modules/m/concepts.md"}})
	if err != nil {
		t.Fatal(err)
	}
	reviewed := map[string]string{}
	for _, st := range out.Started {
		if st.Type != "go-checks" || st.Review == "" || st.Error != "" {
			t.Errorf("each owner's go-checks is filed: %+v", st)
		}
		reviewed[st.Object] = st.Review
	}
	if len(reviewed) != 2 || reviewed["a"] == "" || reviewed["b"] == "" {
		t.Fatalf("a go-checks review on each owning module, none on m: %+v", out.Started)
	}
	for mod, id := range reviewed {
		if !strings.HasPrefix(relPath(t, w, id), "modules/"+mod+"/") {
			t.Errorf("the review of %s is filed in %s: %s", mod, mod, relPath(t, w, id))
		}
	}
	checked := map[string]string{}
	for _, c := range fake.calls {
		if c.URI == "ktw-test:fixture/check" {
			var in struct {
				Object string   `json:"object"`
				Paths  []string `json:"paths"`
			}
			_ = json.Unmarshal(c.Input, &in)
			checked[in.Object] = strings.Join(in.Paths, ",")
		}
	}
	if checked["a"] != "internal/a/x.go" || checked["b"] != "internal/b/y.go" || len(checked) != 2 {
		t.Errorf("each module's check is given its own paths: %v", checked)
	}
}

// A run that checked only part of the subject (a change's packages) leaves
// open the findings it did not see; a run that checked nothing files no
// review at all - the previous review and its open findings stand.
func TestAPartialOrEmptyRunNeverClosesWhatItDidNotCheck(t *testing.T) {
	w := setup(t)
	full := api.ReviewResult{Rating: "needs-attention", Summary: "two", Report: "r1", Findings: []api.ResultFinding{
		{Severity: "high", Statement: "2 test(s) failing", Location: "correctness", Key: "failing"},
		{Severity: "medium", Statement: "Dead code", Location: "dead-code", Key: "dead"},
	}}
	one, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Trigger: api.TriggerAutomatic, Result: full})
	if err != nil || len(one.New) != 2 {
		t.Fatalf("first filing: %+v %v", one, err)
	}

	partial := api.ReviewResult{Rating: "good", Summary: "the change's package is clean", Report: "r2", Coverage: api.CoveragePartial,
		Findings: []api.ResultFinding{{Severity: "medium", Statement: "Dead code", Location: "dead-code", Key: "dead"}}}
	two, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Trigger: api.TriggerAutomatic, Result: partial})
	if err != nil {
		t.Fatal(err)
	}
	if len(two.Resolved) != 0 || len(two.Persisting) != 1 {
		t.Fatalf("a partial run closes nothing it did not see: %+v", two)
	}
	for id := range findingIDs(t, w, "security") {
		if st := stateOf(t, w, id); st == api.FindingClosed || st == api.FindingAddressed {
			t.Errorf("%s was closed by a partial run: %s", id, st)
		}
	}

	none := api.ReviewResult{Rating: "good", Summary: "The change touched no Go file of the module", Report: "r3", Coverage: api.CoverageNone}
	three, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "security", Trigger: api.TriggerAutomatic, Result: none})
	if err != nil {
		t.Fatal(err)
	}
	if three.Review != "" || three.Skipped == "" || len(three.Files) != 0 {
		t.Fatalf("a run that checked nothing files nothing: %+v", three)
	}
	if _, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: two.Review}); err != nil {
		t.Errorf("the previous review stands: %v", err)
	}
	for id := range findingIDs(t, w, "security") {
		if st := stateOf(t, w, id); st == api.FindingClosed || st == api.FindingAddressed {
			t.Errorf("%s was closed by a run that checked nothing: %s", id, st)
		}
	}

	// Through Start: a check answering coverage none is reported skipped.
	fake := &fakeActions{result: none}
	ws := setupWith(t, fstest.MapFS{
		"review-types/checks.yaml": {Data: []byte("name: checks\ntitle: Build and tests\ndimension: quality\napplies: {types: [module]}\nrunner: {check: 'ktw-test:fixture/check'}\npresets: [code]\n")},
	}, fake.extension())
	out, err := ws.svc.Start(ws.ctx, api.StartInput{Object: "m", Preset: "code"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Started) != 1 || out.Started[0].Review != "" || out.Started[0].Skipped == "" || out.Started[0].Error != "" {
		t.Fatalf("a check that checked nothing is reported skipped, no review: %+v", out.Started)
	}
}
