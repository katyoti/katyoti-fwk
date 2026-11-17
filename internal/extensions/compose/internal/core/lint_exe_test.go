// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
)

// fakeExes stands in for compiled sidecars: a manifest and the sources each
// would answer through `sc resources manifest` / `sc resources read`.
type fakeExes struct {
	manifests map[string]*mbapi.Manifest
	sources   map[string]string // uri → source
}

func (f fakeExes) Manifest(_ context.Context, exe string) (*mbapi.Manifest, error) {
	if m, ok := f.manifests[exe]; ok {
		return m, nil
	}
	return nil, errors.New("no such exe: " + exe)
}

func (f fakeExes) Source(_ context.Context, _ string, uri string) ([]byte, error) {
	if s, ok := f.sources[uri]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("no source for " + uri)
}

func exes() fakeExes {
	skill := contribapi.Contribution{Provider: "ktw-a", Type: "skill", ID: "a-skill"}
	skill.SetData([]byte(`{"name":"a-skill"}`))
	skill.SetSource([]byte("x"), "text/markdown")
	other := contribapi.Contribution{Provider: "ktw-b", Type: "skill", ID: "b-skill"}
	other.SetData([]byte(`{"name":"b-skill"}`))
	other.SetSource([]byte("y"), "text/markdown")
	return fakeExes{
		manifests: map[string]*mbapi.Manifest{
			"bin/ktw-a.exe": {Provider: "ktw-a", Contributions: []contribapi.Contribution{skill},
				SlotTypes: []mbapi.SlotType{{Type: "a-service", Methods: []mbapi.SlotMethod{{Name: "Search"}}}}, Slots: []mbapi.Slot{{Name: "a", Type: "a-service"}}},
			"bin/ktw-b.exe": {Provider: "ktw-b", Contributions: []contribapi.Contribution{other}},
		},
		sources: map[string]string{
			// a-skill cites b's skill (cross-sidecar, exists), b's missing reference, and its own slot with a wrong method
			"ktw-a:/skill/a-skill": "---\nname: a-skill\ndescription: A.\n---\n\nRead ktw-b:/skill/b-skill and ktw-b:/reference/missing.md. Call ktw-a:a/search and ktw-a:a/explode.\n",
			"ktw-b:/skill/b-skill": "---\nname: b-skill\ndescription: B.\n---\n\nSee [gone](docs/nope.md).\n",
		},
	}
}

// lint-sidecar judges one executable against itself: own slots, own links;
// what it says about other providers is not its to judge.
func TestLintSidecar_JudgesTheExecutableAgainstItself(t *testing.T) {
	s := &Service{exeOverride: exes()}
	out, err := s.LintSidecar(context.Background(), api.LintSidecarInput{Exe: "bin/ktw-a.exe"})
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]int{}
	for _, f := range out.Findings {
		rules[f.Rule]++
	}
	if out.Status != "fail" || rules["unknown-method"] != 1 || rules["unknown-resource"] != 0 {
		t.Fatalf("self scope: status=%s findings=%v — want the wrong own method only, nothing about ktw-b", out.Status, out.Findings)
	}
	if len(out.Binaries) != 1 || out.Binaries[0] != "ktw-a.exe" {
		t.Errorf("binaries = %v", out.Binaries)
	}
}

// lint-universe judges every executable against all of them: the missing
// reference on ktw-b is now a finding, and so is ktw-b's own dead link.
func TestLintUniverse_JudgesEveryExecutableAgainstAll(t *testing.T) {
	s := &Service{exeOverride: exes()}
	out, err := lintExes(context.Background(), s.exeReader(), []string{"bin/ktw-a.exe", "bin/ktw-b.exe"}, 1 /* ScopeUniverse */)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]int{}
	for _, f := range out.Findings {
		rules[f.Rule]++
	}
	if out.Status != "fail" || rules["unknown-resource"] != 1 || rules["unknown-method"] != 1 || rules["dead-link"] != 1 {
		t.Fatalf("universe scope: %v", out.Findings)
	}
	if strings.Join(out.Binaries, ",") != "ktw-a.exe,ktw-b.exe" {
		t.Errorf("binaries = %v", out.Binaries)
	}
}

// A per-type verb with --uri reads the resource through the executable that
// publishes it; --file and --uri together, or --uri without --exe, are refused.
func TestLintSkill_ResourceModeReadsThroughTheExecutable(t *testing.T) {
	s := &Service{exeOverride: exes()}
	res, err := s.LintSkill(context.Background(), api.LintSkillInput{URI: "ktw-a:/skill/a-skill", Exe: "bin/ktw-a.exe"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range res.Findings {
		if f.Rule == "unknown-method" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resource mode must judge the skill against its sidecar's manifest, got %v", res.Findings)
	}
	if _, err := s.LintSkill(context.Background(), api.LintSkillInput{URI: "ktw-a:/skill/a-skill"}); err == nil {
		t.Error("--uri without --exe must be refused")
	}
	if _, err := s.LintTask(context.Background(), api.LintTaskInput{File: "x.md", URI: "ktw-a:/task/t"}); err == nil {
		t.Error("--file and --uri together must be refused")
	}
}

// A build that could not replace a locked executable leaves `ktw-x.exe~`
// beside it; the universe lint judged that stale copy once (2026-09-22) and
// reported a defect already fixed in the real binary.
func TestSidecarExecutables_IgnoresBuildLeftoversAndInfra(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"ktw-example.exe", "ktw-example.exe~", "ktw-docs.exe.old-1", "katyoti.exe", "katyoti", "ktw-gui.exe", "ktw-hub.exe", "README.md", "ktw-utils.exe"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := sidecarExecutables(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range got {
		names = append(names, filepath.Base(g))
	}
	want := []string{"katyoti", "katyoti.exe", "ktw-example.exe", "ktw-utils.exe"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", names, want)
	}
}

// lint-universe reports a kind two members declare (blind audit H4): the
// loser's URI carries the finding, the message names both, one declaration
// two members carry alike is one kind (no finding) — the pack-declared skill
// kind each binary publishes under its own provider, naming its own schema,
// included — differing bodies fail, and a member redeclaring a system kind
// fails whatever the bodies say, while the system set every binary carries
// under the SAME URI is one declaration, not a duplicate per binary.
func TestLintUniverse_ReportsAKindTwoMembersDeclare(t *testing.T) {
	decl := func(provider, kind, schema, validate string) contribapi.Contribution {
		c := contribapi.Contribution{Provider: provider, Type: contribapi.ResourceTypeContributionType, ID: kind}
		c.SetData([]byte(`{"name":"` + kind + `","schema":"` + schema + `","validate":"` + validate + `"}`))
		return c
	}
	sysWorkflow := decl("system", "workflow", "system:/data-type/workflow", "error")
	rd := fakeExes{manifests: map[string]*mbapi.Manifest{
		// a and b declare recipe identically (same schema, same severity); gadget differs in severity.
		// a and b both carry the skills pack: each declares skill under its own
		// provider, naming its own schema — the same declaration, one kind.
		"bin/ktw-a.exe": {Provider: "ktw-a", Contributions: []contribapi.Contribution{sysWorkflow, decl("ktw-a", "skill", "ktw-a:/data-type/skill", "warn"), decl("ktw-a", "recipe", "ktw-a:/data-type/recipe", "warn"), decl("ktw-a", "gadget", "ktw-a:/data-type/gadget", "warn")}},
		"bin/ktw-b.exe": {Provider: "ktw-b", Contributions: []contribapi.Contribution{sysWorkflow, decl("ktw-b", "skill", "ktw-b:/data-type/skill", "warn"), decl("ktw-b", "recipe", "ktw-a:/data-type/recipe", "warn"), decl("ktw-b", "gadget", "ktw-a:/data-type/gadget", "error")}},
		"bin/ktw-c.exe": {Provider: "ktw-c", Contributions: []contribapi.Contribution{sysWorkflow, decl("ktw-c", "workflow", "ktw-c:/data-type/workflow", "warn")}},
	}}
	out, err := lintExes(context.Background(), rd, []string{"bin/ktw-a.exe", "bin/ktw-b.exe", "bin/ktw-c.exe"}, contentlint.ScopeUniverse)
	if err != nil {
		t.Fatal(err)
	}
	byURI := map[string]string{}
	for _, f := range out.Findings {
		byURI[f.URI] = "[" + string(f.Severity) + "/" + f.Rule + "] " + f.Message
	}
	want := map[string][]string{
		"ktw-b:/contribution-type/gadget":   {"error/" + contribapi.KindRuleDuplicate, "different", "ktw-a:/contribution-type/gadget"},
		"ktw-c:/contribution-type/workflow": {"error/" + contribapi.KindRuleRedeclaresSystem, "system:/contribution-type/workflow"},
	}
	for uri, frags := range want {
		got, ok := byURI[uri]
		if !ok {
			t.Errorf("no finding at %s; findings: %v", uri, byURI)
			continue
		}
		for _, frag := range frags {
			if !strings.Contains(got, frag) {
				t.Errorf("%s: finding %q lacks %q", uri, got, frag)
			}
		}
	}
	if f, dup := byURI["ktw-b:/contribution-type/recipe"]; dup {
		t.Errorf("one declaration carried by two members is one kind, reported %q", f)
	}
	if f, dup := byURI["ktw-b:/contribution-type/skill"]; dup {
		t.Errorf("the skills pack's kind carried by two binaries is one kind, reported %q", f)
	}
	if _, dup := byURI["system:/contribution-type/workflow"]; dup {
		t.Errorf("the system set carried by every binary under one URI was reported as a duplicate")
	}
	if len(byURI) != len(want) || out.Status != "fail" {
		t.Errorf("want %d findings and status fail, got %d and %q: %v", len(want), len(byURI), out.Status, byURI)
	}
	// Self scope is one binary against itself: no cross-member kind finding.
	self, err := lintExes(context.Background(), rd, []string{"bin/ktw-a.exe"}, contentlint.ScopeSelf)
	if err != nil || len(self.Findings) != 0 {
		t.Errorf("lint-sidecar must not judge other members' kinds: err=%v findings=%v", err, self.Findings)
	}
}

// lint-universe reports what CWM would find reading the members'
// declarations: a gate entry one member contributes on another member's type
// is narrowed to its own objects — a finding naming the grant to add.
func TestLintUniverse_ReportsAGateOutOfItsProvidersScope(t *testing.T) {
	entry := func(provider, kind, id, body string) contribapi.Contribution {
		c := contribapi.Contribution{Provider: provider, Type: kind, ID: id}
		c.SetData([]byte(body))
		return c
	}
	rd := fakeExes{manifests: map[string]*mbapi.Manifest{
		"bin/ktw-a.exe": {Provider: "ktw-a", Contributions: []contribapi.Contribution{
			entry("ktw-a", "object-type", "doc", `{"name":"doc"}`)}},
		"bin/ktw-b.exe": {Provider: "ktw-b", Contributions: []contribapi.Contribution{
			entry("ktw-b", "hook", "spy", `{"name":"spy","type":"before_update","uri":"ktw-b:spy/check","filter":{"types":["doc"]}}`),
			entry("ktw-b", "hook", "log", `{"name":"log","type":"on_update","uri":"ktw-b:spy/log","filter":{"types":["doc"]}}`)}},
	}}
	out, err := lintExes(context.Background(), rd, []string{"bin/ktw-a.exe", "bin/ktw-b.exe"}, contentlint.ScopeUniverse)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range out.Findings {
		if strings.HasPrefix(f.Rule, "declaration-") {
			got = append(got, f.URI+" "+f.Rule)
			if !strings.Contains(f.Message, "grants: {gate:") {
				t.Errorf("the finding must name the grant to add: %s", f.Message)
			}
		}
	}
	if strings.Join(got, ",") != "ktw-b:/hook/spy declaration-scope" {
		t.Errorf("declaration findings = %v, want the before_update hook only (an after hook is not scoped)", got)
	}
}
