// SPDX-License-Identifier: Apache-2.0

package contentlint

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

func universe() *Universe {
	leanix := &mbapi.Manifest{Provider: "ktw-example",
		SlotTypes: []mbapi.SlotType{{Type: "leanix-service", Methods: []mbapi.SlotMethod{{Name: "Search"}, {Name: "RunReport"}}}},
		Slots:     []mbapi.Slot{{Name: "leanix", Type: "leanix-service"}},
		Contributions: []contribapi.Contribution{
			{Provider: "ktw-example", Type: "skill", ID: "leanix-tools"},
			{Provider: "ktw-example", Type: "skill", ID: "leanix-doc"},
			{Provider: "ktw-example", Type: "reference", ID: "leanix-tools/references/report-templates.md"},
		}}
	docs := &mbapi.Manifest{Provider: "ktw-docs",
		Contributions: []contribapi.Contribution{{Provider: "ktw-docs", Type: "reference", ID: "diagrams/mermaid.md"}}}
	compose := &mbapi.Manifest{Provider: "ktw-compose",
		Contributions: []contribapi.Contribution{{Provider: "ktw-compose", Type: "task", ID: "compose-artifact"}}}
	return NewUniverse([]*mbapi.Manifest{leanix, docs, compose})
}

// Every reference class the 2026-09-22 audit found in the corpus, one each,
// judged at both scopes. Templates and wildcards are prose, never findings;
// a skill's own `references/` file is its asset (served by the skill loader,
// not a catalog entry) and is never a finding in manifest mode.
func TestCheckRefs_JudgesEveryReferenceClass(t *testing.T) {
	body := `
See [the templates](references/report-templates.md), [a sibling skill](leanix-doc.md) and [gone](docs/nope.md).
Read ktw-docs:/reference/diagrams/mermaid.md and ktw-docs:/reference/diagrams/ (a folder).
Missing: ktw-docs:/reference/nothing.md, and ktw-acme:/notation/x (no such provider).
Call ktw-example:leanix/search, then ktw-example:leanix/run-report; not ktw-example:leanix-service/search
nor ktw-example:leanix/explode. Wildcards ktw-example:leanix/* and ktw-compose:tasks/compose-artifact are fine.
Templates: [x]({baseUrl}/Application/{UUID}) and [y](…) and [z](url) and https://example.com/ok.
Families: ktw-acme:/skill/doc-<type> and ktw-code:/reference/modules/verbs/<verb>.md are patterns, not addresses.
` + "Code is not prose: `application.UseService[SkillManager](ctx)` and `ktw-nope:/skill/example` in a span,\n" +
		"```go\nsvc := application.UseService[Thing](ctx) // ktw-nope:nope/method\n```\n" +
		"A folder link [the report templates](leanix-tools/references/) names what is published under it.\n" +
		"Not ours: an OData filter `members/any(m:m/email eq 'x')` and a Go embed pattern `all:embed/x` read as tool URIs.\n"
	refs := ExtractRefs(body)
	got := map[string]int{}
	for _, r := range refs {
		got[string(r.Kind)]++
	}
	if got["link"] != 4 || got["resource"] != 5 || got["tool"] != 7 {
		t.Fatalf("extracted %v (%+v), want 4 links, 5 resources (the span-quoted one counts), 7 tools", got, refs)
	}

	u := universe()
	self := CheckRefs("ktw-example", refs, u, ScopeSelf)
	uni := CheckRefs("ktw-example", refs, u, ScopeUniverse)
	want := map[string][]string{
		"self":     {"dead-link:docs/nope.md", "unknown-slot:ktw-example:leanix-service/search", "unknown-method:ktw-example:leanix/explode"},
		"universe": {"dead-link:docs/nope.md", "unknown-resource:ktw-docs:/reference/nothing.md", "unknown-provider:ktw-acme:/notation/x", "unknown-provider:ktw-nope:/skill/example", "unknown-slot:ktw-example:leanix-service/search", "unknown-method:ktw-example:leanix/explode", "unknown-provider:m:m/email", "unknown-provider:all:embed/x"},
	}
	// An unknown provider shaped like ours (`ktw-*`) is a typo — an error; any
	// other token is more likely not a URI at all — a warning, still reported.
	for _, f := range uni {
		if f.Rule != "unknown-provider" {
			continue
		}
		ours := strings.Contains(f.Message, `"ktw-`)
		if ours && f.Severity != verify.SeverityError || !ours && f.Severity != verify.SeverityWarn {
			t.Errorf("unknown-provider severity %s for %q", f.Severity, f.Message)
		}
	}
	for name, fs := range map[string][]verify.LintFinding{"self": self, "universe": uni} {
		if len(fs) != len(want[name]) {
			t.Errorf("%s scope: %d findings, want %d:\n  got  %v\n  want %v", name, len(fs), len(want[name]), fs, want[name])
			continue
		}
		for i, w := range want[name] {
			rule, text, _ := strings.Cut(w, ":")
			if fs[i].Rule != rule || !strings.Contains(fs[i].Message, text) {
				t.Errorf("%s scope finding %d = %s %q, want %s about %q", name, i, fs[i].Rule, fs[i].Message, rule, text)
			}
		}
	}
}

// LintManifest runs the kind's format lint, then the references, and reports
// an entry whose source cannot be opened instead of skipping it.
func TestLintManifest_FormatThenRefs_NeverSkips(t *testing.T) {
	skillOK := "---\nname: leanix-tools\ndescription: Query LeanIX.\n---\n\n# LeanIX tools\n\nCall ktw-example:leanix/search.\n"
	skillBad := "---\nname: broken\ndescription: Uses KTSP.\nowner: me\n---\n\nSee [x](docs/none.md). KTSP is the protocol.\n"
	m := &mbapi.Manifest{Provider: "ktw-example", Contributions: []contribapi.Contribution{
		{Provider: "ktw-example", Type: "skill", ID: "leanix-tools"},
		{Provider: "ktw-example", Type: "skill", ID: "broken"},
		{Provider: "ktw-example", Type: "skill", ID: "unreadable"},
		{Provider: "ktw-example", Type: "reference", ID: "not-linted"},
	}}
	open := func(uri, layer string) ([]byte, error) {
		switch uri {
		case "ktw-example:/skill/leanix-tools":
			return []byte(skillOK), nil
		case "ktw-example:/skill/broken":
			return []byte(skillBad), nil
		}
		return nil, errors.New("no bytes")
	}
	fs := LintManifest(m, open, universe(), ScopeSelf)
	byURI := map[string][]string{}
	for _, f := range fs {
		byURI[f.URI] = append(byURI[f.URI], f.Rule)
	}
	if len(byURI["ktw-example:/skill/leanix-tools"]) != 0 {
		t.Errorf("a clean skill has findings: %v", byURI["ktw-example:/skill/leanix-tools"])
	}
	got := strings.Join(byURI["ktw-example:/skill/broken"], ",")
	for _, want := range []string{"frontmatter-extra-key", "internal-jargon", "dead-link"} {
		if !strings.Contains(got, want) {
			t.Errorf("broken skill: findings %s lack %s", got, want)
		}
	}
	if got := byURI["ktw-example:/skill/unreadable"]; len(got) != 1 || got[0] != "source-unreadable" {
		t.Errorf("unreadable source must be a finding, got %v", got)
	}
	if _, linted := byURI["ktw-example:/reference/not-linted"]; linted {
		t.Error("a reference is not a skill or task and is not linted")
	}
	if len(Errors(fs)) == 0 {
		t.Error("Errors() must keep the blocking findings")
	}
}

// File mode resolves relative links on disk next to the file — a skill's own
// assets included — and judges URIs only when a universe is given.
func TestLintFile_ResolvesLinksOnDisk(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "references"), 0o755)
	os.WriteFile(filepath.Join(dir, "skills", "references", "scope.md"), []byte("# scope\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "skills", "sibling.md"), []byte("---\nname: sibling\ndescription: Sibling.\n---\n"), 0o644)
	skill := filepath.Join(dir, "skills", "main.md")
	os.WriteFile(skill, []byte("---\nname: main\ndescription: Main skill.\n---\n\nSee [scope](references/scope.md), [sibling](sibling.md), [gone](references/gone.md).\nCall ktw-example:leanix/search and ktw-example:leanix/explode.\n"), 0o644)

	res := LintFile(skill, "", nil)
	rules := map[string]int{}
	for _, f := range res.Findings {
		rules[f.Rule]++
	}
	if rules["dead-link"] != 1 || len(res.Findings) != 1 {
		t.Fatalf("without a universe: %v, want exactly one dead-link (references/gone.md)", res.Findings)
	}
	res = LintFile(skill, "", universe())
	rules = map[string]int{}
	for _, f := range res.Findings {
		rules[f.Rule]++
	}
	if rules["dead-link"] != 1 || rules["unknown-method"] != 1 || len(res.Findings) != 2 {
		t.Fatalf("with a universe: %v, want one dead-link and one unknown-method", res.Findings)
	}
	if kindOfPath("app/x/embed/tasks/foo.md") != "task" || kindOfPath("app/x/embed/skills/foo.md") != "skill" || kindOfPath("docs/foo.md") != "" {
		t.Error("kindOfPath: the folder names the kind")
	}
}

// A deck theme and a slide group are held to their kind's format check: the
// lint reads the slide-deck lint's row for each, by the kind's name or its
// pack folder.
func TestLintBytes_DeckKindsHaveTheirFormatCheck(t *testing.T) {
	rules := func(kind, src string) map[string]int {
		out := map[string]int{}
		for _, f := range LintBytes(kind, []byte(src), "ktw-example", nil, ScopeSelf).Findings {
			out[f.Rule]++
		}
		return out
	}
	if got := rules("deck-theme", "name: brand\ndescription: A brand.\n"); len(got) != 0 {
		t.Errorf("a sound theme: %v", got)
	}
	if got := rules("deck-theme", "name: Brand\ncolours: {}\n"); got["deck-theme-format"] != 1 {
		t.Errorf("a theme with an unknown key: %v, want deck-theme-format", got)
	}
	if got := rules("deck-theme", "name: Brand\n"); got["deck-theme-name"] != 1 {
		t.Errorf("a theme named not kebab: %v, want deck-theme-name", got)
	}
	if got := rules("slide-group", "name: our-team\n"); got["slide-group-slides"] == 0 {
		t.Errorf("a group with no slides: %v, want slide-group-slides", got)
	}
	if kindOfPath("app/x/embed/deck-themes/brand.yaml") != "deck-theme" || kindOfPath("app/x/embed/slide-groups/our-team.yaml") != "slide-group" ||
		kindOfPath("app/x/embed/deck-assets/brand/logo.png") != "" {
		t.Error("kindOfPath: the deck folders name their kind")
	}
}

// : an object-panel's filter.when (an expression its kind
// declares) is compiled against the object vocabulary - a name it does not
// know, as capabilities.publication.connector, is an error before any pane
// evaluates it; a called name stands for an expression function.
func TestLintManifest_PanelWhenNamesOnlyTheObjectVocabulary(t *testing.T) {
	panel := func(id, when string) contribapi.Contribution {
		data := []byte(`{"name":"` + id + `","filter":{"types":["publication"],"when":` + strconv.Quote(when) + `}}`)
		return contribapi.Contribution{Provider: "ktw-agents", Type: "object-panel", ID: id,
			Layers: map[string]contribapi.LayerRef{contribapi.LayerData: {Inline: data}}}
	}
	m := &mbapi.Manifest{Provider: "ktw-agents", Contributions: []contribapi.Contribution{
		panel("bad", "capabilities.publication.connector != ''"),
		panel("good", "source_sub_tabs() == false && object.publication.connector != '' && 'x' in current.capabilities"),
		panel("none", ""),
	}}
	fs := LintManifest(m, func(string, string) ([]byte, error) { return nil, errors.New("no bytes") }, universe(), ScopeSelf)
	var got []string
	for _, f := range fs {
		if f.Rule == "expression-unknown-name" {
			got = append(got, f.URI)
			if f.Severity != verify.SeverityError || !strings.Contains(f.Message, "filter.when") {
				t.Errorf("an unknown name is an error naming its field: %+v", f)
			}
		}
	}
	if len(got) != 1 || got[0] != "ktw-agents:/object-panel/bad" {
		t.Errorf("only the panel naming capabilities is a finding: %v (all: %v)", got, fs)
	}
}
