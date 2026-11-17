// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/assistant"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The tasks What's Next opens a chat on: decompose a goal into features, and
// a feature into sub-features.
const (
	GenerateFeaturesTask = "generate-goal-features"
	BreakDownTask        = "break-down-feature"
)

// deterministic are the action kinds Act resolves by a CWM write; the others
// name creative work.
var deterministic = map[string]bool{"shape-us": true, "review-waste": true}

// NextPage is What's Next as the page that shows it: Overview's answer, and
// the requests waiting on intake, drawn with the page blocks.
func (s *Service) NextPage(ctx context.Context, in api.ObjectInput) (render.Def, error) {
	ov, err := s.Overview(ctx, in)
	if err != nil {
		return render.Def{}, err
	}
	requests := s.triage(ctx, in.ID)
	if in.Release != "" {
		cwm, err := cwmOf(ctx)
		if err != nil {
			return render.Def{}, err
		}
		snap, err := Gather(ctx, cwm, in.ID)
		if err != nil {
			return render.Def{}, err
		}
		_, kept, err := s.scoped(ctx, cwm, snap, in.Release)
		if err != nil {
			return render.Def{}, err
		}
		requests = keptRequests(requests, kept)
	}
	tree := NextTree(in.Language, in.ID, ov, requests)
	if choices := s.releaseChoices(ctx, in.ID); len(choices) > 0 {
		tree = render.VBox(releasePicker(in.Language, in.Release, choices), tree)
	}
	return render.Def{Kind: "page", DSLTree: tree}, nil
}

// releaseChoices are the releases and increments What's Next can be narrowed
// to on the object (a module's own; a project's, every module's); none when
// no lister is wired or planification does not answer.
func (s *Service) releaseChoices(ctx context.Context, id string) []ReleaseChoice {
	if s.listReleases == nil {
		return nil
	}
	module := id
	if cwm, err := cwmOf(ctx); err == nil {
		// A project - any type carrying the project level capability
		// - lists every module's releases.
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id}); err == nil {
			if t, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: got.Object.Type}); err == nil && t.ObjectType.Level() == cwmapi.CapProject {
				module = ""
			}
		}
	}
	choices, err := s.listReleases(ctx, module)
	if err != nil {
		return nil
	}
	return choices
}

// releaseForm narrows What's Next to a release or an increment.
type releaseForm struct {
	Release string `json:"release,omitempty"`
}

// releasePicker is a query form choosing the release What's Next is narrowed
// to; the page re-reads with it, empty for everything.
func releasePicker(lang, current string, choices []ReleaseChoice) render.Node {
	schema := render.InlineSchema[releaseForm]("ReleaseScope", map[string]string{"release": tr(lang, "release")})
	enum, labels := []any{""}, map[string]any{"": tr(lang, "allWork")}
	for _, c := range choices {
		enum = append(enum, c.ID)
		labels[c.ID] = c.Label
	}
	if root, ok := schema["types"].(map[string]any)["ReleaseScope"].(map[string]any); ok {
		if f, ok := root["fields"].(map[string]any)["release"].(map[string]any); ok {
			f["enum"], f["contentParams"] = enum, map[string]any{"options": labels}
		}
	}
	return render.Card(render.Form(schema, render.QueryWith(tr(lang, "narrow"), nil)).
		With("data", map[string]any{"release": current}).With("mode", "edit")).With("p", "sm")
}

// triage lists the module's requests waiting on intake: ready, and no intake
// step recorded since they last became ready (filed, filed again, answered).
// None when no lister is wired or planification does not answer.
func (s *Service) triage(ctx context.Context, moduleID string) []planapi.Request {
	if s.listRequests == nil {
		return nil
	}
	got, err := s.listRequests(ctx, planapi.ListInput{Module: moduleID, State: "ready"})
	if err != nil {
		return nil
	}
	var out []planapi.Request
	for _, r := range got.Requests {
		if r.Plan != nil && awaitsIntake(*r.Plan) {
			out = append(out, r)
		}
	}
	return out
}

// awaitsIntake: no intake step of the plan ended after its last move to ready.
func awaitsIntake(p planapi.PlanFields) bool {
	ready := ""
	for _, e := range p.Trail {
		if e.To == planapi.PlanReady && e.At > ready {
			ready = e.At
		}
	}
	for _, st := range p.Steps {
		if st.Task == "intake" && st.EndedAt != "" && st.EndedAt >= ready {
			return false
		}
	}
	return true
}

// TriageShown is how many requests to triage the page lists; the rest are
// counted.
const TriageShown = 5

// NextTree draws What's Next on the object id: the acceptance criteria's
// standing, the requests to triage, the goals (each opening a chat that
// generates its features), the features as cards (CoD, delivery, a gauge,
// their criteria as chips that open them, a chat that breaks them down), the
// criteria under no feature, the alignment findings and the ranked actions,
// each run on the act URI. Its texts are in lang (en or fr; empty = English).
func NextTree(lang, id string, ov api.OverviewOutput, requests []planapi.Request) render.Node {
	var blocks []render.Node
	blocks = append(blocks, standing(lang, ov.Overview))
	if len(requests) > 0 {
		blocks = append(blocks, requestsSection(lang, id, requests))
	}
	if len(ov.Goals) > 0 {
		blocks = append(blocks, goalsSection(lang, ov.Goals))
	}
	blocks = append(blocks, featuresSection(lang, ov.Stories))
	if len(ov.LooseCriteria) > 0 {
		blocks = append(blocks, section(tr(lang, "looseCriteria"), "circle-dashed", "gray", len(ov.LooseCriteria),
			render.HBox(chips(lang, ov.LooseCriteria)...)))
	}
	if len(ov.Findings) > 0 {
		blocks = append(blocks, findingsSection(lang, ov.Findings))
	}
	blocks = append(blocks, actionsSection(lang, id, ov))
	return render.VBox(blocks...).With("gap", "lg")
}

func section(title, icon, color string, count int, children ...render.Node) render.Node {
	return render.Section(title, icon, children...).With("size", "md").With("color", color).With("badge", count)
}

// row lines nodes up on one line, the first ones left and the last right.
func row(children ...render.Node) render.Node {
	return render.HBox(children...).With("justify", "space-between").With("wrap", false).With("align", "center")
}

func strong(text string) render.Node { return render.Text(text).With("size", "sm").With("fw", 500) }

// standing is the acceptance criteria of the scope as a stacked bar, by the
// status of their verdicts: passing, failing, without a test, could not run.
func standing(lang string, o assistant.Overview) render.Node {
	if o.CriteriaTotal == 0 {
		return render.Dimmed(tr(lang, "noCriteria"))
	}
	return render.Stacked(tr(lang, "criteria"),
		render.Segment{Value: float64(o.CriteriaPassing), Color: "green", Label: tr(lang, "segPass")},
		render.Segment{Value: float64(o.CriteriaFailing), Color: "red", Label: tr(lang, "segFailing")},
		render.Segment{Value: float64(o.CriteriaNoTest), Color: "gray", Label: tr(lang, "segNoTest")},
		render.Segment{Value: float64(o.CriteriaUnproven), Color: "orange", Label: tr(lang, "segUnproven")},
	).With("valueText", fmt.Sprintf(tr(lang, "passOf"), o.CriteriaPassing, o.CriteriaTotal)).With("max", o.CriteriaTotal)
}

func requestsSection(lang, moduleID string, rs []planapi.Request) render.Node {
	var rows []render.Node
	for i, r := range rs {
		if i == TriageShown {
			rows = append(rows, render.Dimmed(fmt.Sprintf(tr(lang, "andMore"), len(rs)-TriageShown)))
			break
		}
		line := []render.Node{
			render.Badge(r.ID, "gray").With("mono", true).With("size", "xs"),
			link(r.Title, render.NavigateTo(tr(lang, "openRequest"), r.ID, "")),
		}
		if r.Kind == "question" {
			line = append(line, render.Badge(tr(lang, "question"), "cyan").With("size", "xs"))
		}
		if r.Requester.Kind == "module" {
			line = append(line, render.Badge(fmt.Sprintf(tr(lang, "from"), r.Requester.ID), "gray").With("size", "xs").With("keepCase", true))
		}
		rows = append(rows, render.HBox(line...).With("wrap", false))
	}
	return section(tr(lang, "requestsToTriage"), "inbox", "violet", len(rs), rows...).
		Aside(link(tr(lang, "allRequests"), render.NavigateTo(tr(lang, "allRequests"), moduleID, "requests")))
}

func goalsSection(lang string, goals []api.GoalView) render.Node {
	var rows []render.Node
	for _, g := range goals {
		title := g.Title
		if title == "" {
			title = tr(lang, "untitledGoal")
		}
		tone := "gray"
		if g.ServingUS > 0 && g.DeliveredUS == g.ServingUS {
			tone = "green"
		}
		gen := render.Button(tr(lang, "generateFeatures"), render.ChatOn(tr(lang, "generateFeatures"), GenerateFeaturesTask, g.ID)).
			With("icon", "sparkles").With("color", "violet")
		rows = append(rows, row(
			render.HBox(link(title, render.NavigateTo(tr(lang, "openGoal"), g.ID, "")),
				render.Badge(fmt.Sprintf(tr(lang, "featuresOf"), g.DeliveredUS, g.ServingUS), tone).With("keepCase", true)).With("wrap", false),
			gen))
	}
	return section(tr(lang, "goals"), "target", "teal", len(goals), rows...)
}

func featuresSection(lang string, stories []api.StoryView) render.Node {
	var cards []render.Node
	for _, s := range stories {
		state := render.Badge(fmt.Sprintf("%d/%d", s.PassCount, s.TotalCount), "gray").With("variant", "outline").With("size", "xs")
		if s.Delivered {
			state = render.Badge(tr(lang, "delivered"), "green").With("variant", "filled").With("size", "xs").With("icon", "circle-check")
		}
		head := row(
			link(s.Title, render.NavigateTo(tr(lang, "openFeature"), s.ID, "")).With("fw", 600),
			render.HBox(
				render.Badge(fmt.Sprintf("CoD %d", s.CoD), "violet").With("size", "xs").With("keepCase", true),
				state,
				render.Button(tr(lang, "breakDown"), render.ChatOn(tr(lang, "breakDown"), BreakDownTask, s.ID)).
					With("icon", "hierarchy-2").With("color", "violet"),
			).With("wrap", false))
		body := []render.Node{head}
		if s.TotalCount > 0 {
			tone := "teal"
			if s.Delivered {
				tone = "green"
			}
			body = append(body, render.Gauge("", float64(s.PassCount), float64(s.TotalCount)).
				With("color", tone).With("size", "sm").With("valueText", ""))
		}
		if len(s.Criteria) > 0 {
			body = append(body, render.HBox(chips(lang, s.Criteria)...))
		}
		cards = append(cards, render.Card(body...).With("gap", "xs").With("p", "sm"))
	}
	return section(tr(lang, "features"), "list-details", "violet", len(stories), cards...).Empty(tr(lang, "noFeatures"), "")
}

// chips are acceptance criteria as badges colored and signed by their
// verdict's status, each opening its criterion.
func chips(lang string, acs []api.CriterionView) []render.Node {
	out := make([]render.Node, 0, len(acs))
	for _, c := range acs {
		tone, icon := statusVisual(c.Status)
		out = append(out, render.Badge(c.Title, tone).With("icon", icon).With("keepCase", true).
			With("tooltip", c.Title+" — "+c.Status).
			With("action", render.NavigateTo(tr(lang, "openCriterion"), c.ID, "")))
	}
	return out
}

// statusVisual is a criterion verdict status's color and sign.
func statusVisual(status string) (string, string) {
	switch {
	case assistant.Passes(status):
		return "green", "circle-check"
	case assistant.Failing(status):
		return "red", "circle-x"
	case status == "no-test":
		return "gray", "circle-dashed"
	}
	return "orange", "alert-triangle"
}

func findingColor(kind string) string {
	switch kind {
	case "GAP":
		return "orange"
	case "WASTE":
		return "red"
	case "ORPHAN":
		return "grape"
	}
	return "gray"
}

func findingsSection(lang string, fs []assistant.Finding) render.Node {
	var rows []render.Node
	for _, f := range fs {
		title := f.Title
		if title == "" {
			title = f.Subject
		}
		rows = append(rows, render.HBox(
			render.Badge(f.Kind, findingColor(f.Kind)),
			render.VBox(strong(title), render.Dimmed(f.Message)).With("gap", 0),
		).With("wrap", false).With("align", "flex-start"))
	}
	return section(tr(lang, "alignment"), "alert-triangle", "orange", len(fs), rows...)
}

func actionColor(score int) string {
	switch {
	case score >= 1000:
		return "red"
	case score >= 450:
		return "orange"
	case score >= 200:
		return "blue"
	}
	return "gray"
}

// actionsSection ranks the actions, each run on the act URI with {id, kind}:
// Act answers {ok, message}, shown by the host, which then re-reads.
func actionsSection(lang, id string, ov api.OverviewOutput) render.Node {
	var rows []render.Node
	for i, a := range ov.Actions {
		title := a.Title
		if title == "" {
			title = a.Kind
		}
		run := render.Button(tr(lang, "run"), render.RunURI(fmt.Sprintf(tr(lang, "runKind"), a.Kind), ov.Act, map[string]any{"id": id, "kind": a.Kind})).
			With("icon", "bolt").With("color", "gray").With("variant", "subtle")
		if deterministic[a.Kind] {
			run = run.With("color", "teal").With("variant", "light")
		}
		rows = append(rows, row(
			render.HBox(
				render.Badge(fmt.Sprintf("%d", i+1), actionColor(a.Score)).With("variant", "filled"),
				render.VBox(
					render.HBox(strong(title), render.Badge(a.Kind, "gray").With("variant", "outline").With("size", "xs")).With("wrap", false),
					render.Dimmed(a.Rationale),
				).With("gap", 2),
			).With("wrap", false).With("align", "flex-start"),
			run))
	}
	return section(tr(lang, "whatsNext"), "compass", "blue", len(ov.Actions), rows...).Empty(tr(lang, "allGreen"), "green")
}

// link is a small link — the views' body size.
func link(text string, a render.Action) render.Node { return render.Link(text, a).With("size", "sm") }
