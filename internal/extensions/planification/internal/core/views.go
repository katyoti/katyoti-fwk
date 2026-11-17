// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Views implements api.ViewService: it reads planification's objects through
// the release and planning services and writes Markdown pages. It knows no target.
type Views struct{}

// NewViews returns the view service.
func NewViews() *Views { return &Views{} }

var _ api.ViewService = (*Views)(nil)

// labels are the words a page is written with, per language.
var labels = map[string]map[string]string{
	"en": {
		"changelog": "Change log", "releases": "Releases shipped", "outside": "Delivered outside a release",
		"modules": "Modules", "plan": "Plan", "sprint": "Sprint", "waits": "Waits on",
		"holds": "Holds", "critical": "Critical path", "duration": "Remaining duration", "runnable": "Can start now",
		"goals": "Goals", "budget": "Budget", "produces": "Produces", "unplanned": "Not in a sprint",
		"none": "None.", "deliverables": "Deliverables", "stream": "Stream", "latest": "Latest release",
		"state": "State", "progress": "Done", "network": "Network",
		"gates": "Gates", "ready_to_release": "Ready to release", "waiting": "Waiting on a gate", "forced": "Forced through a gate",
		"confirmed_by": "confirmed by", "awaits_person": "waits for a person's confirmation", "validation": "validation", "answered": "answered",
		"release": "Release", "date": "Date", "module": "Module",
		"nothing": "Nothing shipped yet.", "unit": "estimate units", "scope": "Scope", "module_releases": "Module releases shipping in it", "releases_index": "Releases", "open_releases": "releases planned or frozen",
		"work": "Work", "td_running": "Pulled", "td_ready": "Ready", "td_ready_order": "Ready, in the order they are taken", "td_waiting": "Waiting", "td_failed": "Failed or blocked",
		"td_done": "Done (archived)", "td_todo": "To-do", "td_step": "Step", "td_request": "Request", "td_who": "Who", "td_priority": "Priority", "td_all": "every module", "td_requests": "Requests", "td_stands": "Where it stands", "td_ended": "To-dos ended",
		"mu_title": "Module update", "mu_hint": "A request that changes a module ends with it up to date: the want it serves, an acceptance check that proves the change, the change log",
		"mu_serves": "Serves", "mu_serves_none": "Serves no want yet", "mu_changelog": "Change log", "mu_changelog_none": "No change-log entry recorded",
		"mu_invisible": "Not in the change log", "mu_checks": "Acceptance checks changed", "mu_checks_none": "No acceptance check recorded",
		"mu_missing": "Missing", "mu_exempt": "Exempt", "mu_ok": "Up to date", "mu_waived": "Waived", "mu_waive": "Waive the module update",
		"mu_waive_hint": "A person only: the request ends without its module brought up to date — say why", "mu_waive_note": "Why its module needs no update",
	},
	"fr": {
		"changelog": "Journal des changements", "releases": "Versions livrées", "outside": "Livré hors version",
		"modules": "Modules", "plan": "Plan", "sprint": "Sprint", "waits": "Attend",
		"holds": "Contient", "critical": "Chemin critique", "duration": "Durée restante", "runnable": "Peut démarrer",
		"goals": "Objectifs", "budget": "Budget", "produces": "Produit", "unplanned": "Hors sprint",
		"none": "Aucun élément.", "deliverables": "Livrables", "stream": "Flux", "latest": "Dernière version",
		"state": "État", "progress": "Terminé", "network": "Réseau",
		"gates": "Portes", "ready_to_release": "Prête à livrer", "waiting": "En attente d'une porte", "forced": "Passages forcés",
		"confirmed_by": "confirmé par", "awaits_person": "attend la confirmation d'une personne", "validation": "validation", "answered": "répondue",
		"release": "Version", "date": "Date", "module": "Module",
		"nothing": "Rien de livré pour l'instant.", "unit": "unités d'estimation", "scope": "Périmètre", "module_releases": "Versions de modules livrées avec elle", "releases_index": "Versions", "open_releases": "versions planifiées ou gelées",
		"work": "Travail", "td_running": "Prises", "td_ready": "Prêtes", "td_ready_order": "Prêtes, dans l'ordre où elles sont prises", "td_waiting": "En attente", "td_failed": "En échec ou bloquées",
		"td_done": "Faites (archivées)", "td_todo": "À faire", "td_step": "Étape", "td_request": "Demande", "td_who": "Qui", "td_priority": "Priorité", "td_all": "tous les modules", "td_requests": "Demandes", "td_stands": "Où elle en est", "td_ended": "À faire terminées",
		"mu_title": "Mise à jour du module", "mu_hint": "Une demande qui change un module se termine avec lui à jour : le besoin qu'elle sert, un critère d'acceptation qui prouve le changement, le journal des changements",
		"mu_serves": "Sert", "mu_serves_none": "Ne sert encore aucun besoin", "mu_changelog": "Journal des changements", "mu_changelog_none": "Aucune entrée du journal des changements enregistrée",
		"mu_invisible": "Hors du journal des changements", "mu_checks": "Critères d'acceptation modifiés", "mu_checks_none": "Aucun critère d'acceptation enregistré",
		"mu_missing": "Manque", "mu_exempt": "Exemptée", "mu_ok": "À jour", "mu_waived": "Dispensée", "mu_waive": "Dispenser de la mise à jour du module",
		"mu_waive_hint": "Une personne seulement : la demande se termine sans que son module soit mis à jour ; dites pourquoi", "mu_waive_note": "Pourquoi son module n'a pas besoin d'être mis à jour",
	},
}

func label(lang, key string) string {
	if l, ok := labels[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return labels["en"][key]
}

func page(key, parent, title, body string) render.Document {
	m := map[string]any{api.MetaKey: key}
	if parent != "" {
		m[api.MetaParent] = parent
	}
	return render.Document{MediaType: "text/markdown", Title: title, Content: body, Meta: m}
}

func check(done bool) string {
	if done {
		return "✅"
	}
	return "⬜"
}

func money(v float64) string {
	if v == 0 {
		return "—"
	}
	return fmt.Sprintf("$%g", v)
}

// doneDates is, per object, the day it last reached `done` (its own lifecycle
// or a capability's — a request moves through its plan), from the
// journal's transitioned records (an object stores only its state): one read
// for a whole page. No journal, no dates.
func doneDates(ctx context.Context) map[string]string {
	out := map[string]string{}
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return out
	}
	got, err := journal.Query(ctx, journalapi.QueryInput{Actions: []string{journalapi.ActionTransitioned}, Limit: doneDatesLimit})
	if err != nil {
		return out
	}
	for _, r := range got.Records { // newest first: the first met is the latest
		if r.ToState == api.TargetDone && out[r.ID] == "" { // its own lifecycle or its plan's
			out[r.ID] = r.At.UTC().Format("2006-01-02")
		}
	}
	return out
}

// doneDatesLimit bounds the transitioned records a page reads.
const doneDatesLimit = 20000

// moduleScope answers whether a module is in a change log's scope: the scope's
// module or one of its children (a parent module groups them); an empty scope
// takes every module.
func moduleScope(ctx context.Context, cwm cwmapi.CWMService, root string) func(string) bool {
	cache := map[string]bool{}
	return func(module string) bool {
		if root == "" {
			return true
		}
		if v, ok := cache[module]; ok {
			return v
		}
		v, _ := withinModule(ctx, cwm, module, root)
		cache[module] = v
		return v
	}
}

// ChangeLog renders the change log page.
func (v *Views) ChangeLog(ctx context.Context, in api.ChangeLogInput) (api.DocumentsOutput, error) {
	body, err := v.changeLog(ctx, in)
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	return api.DocumentsOutput{Documents: []render.Document{page("changelog", "", label(in.Lang, "changelog"), body)}}, nil
}

func (v *Views) changeLog(ctx context.Context, in api.ChangeLogInput) (string, error) {
	lang := in.Lang
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	inScope := moduleScope(ctx, cwm, in.Module)
	var b strings.Builder
	rel := NewReleases()
	shipped, err := rel.List(ctx, api.ListInput{State: api.ReleaseReleased})
	if err != nil {
		return "", err
	}
	sort.SliceStable(shipped.Releases, func(i, j int) bool {
		a, c := shipped.Releases[i], shipped.Releases[j]
		if a.Date != c.Date {
			return a.Date > c.Date
		}
		return a.ID > c.ID
	})
	fmt.Fprintf(&b, "## %s\n\n", label(lang, "releases"))
	n := 0
	for _, r := range shipped.Releases {
		if (r.Project && in.Module != "") || !r.Project && !inScope(r.Module) {
			continue
		}
		n++
		fmt.Fprintf(&b, "### %s — %s", r.Version, r.Name)
		if r.Date != "" {
			fmt.Fprintf(&b, " (%s)", r.Date)
		}
		fmt.Fprintf(&b, "\n\n%s: `%s`\n\n", label(lang, "module"), r.Module)
		s, err := rel.Scope(ctx, api.GetInput{ID: r.ID})
		if err != nil {
			return "", err
		}
		for _, t := range s.Targets {
			fmt.Fprintf(&b, "- %s %s", check(t.Done), t.Title)
			if t.Via != "" {
				fmt.Fprintf(&b, " _(%s)_", t.Via)
			}
			b.WriteString("\n")
			o, _, err := loadAny(ctx, cwm, t.ID)
			if err != nil {
				return "", err
			}
			if hasRequest(o) {
				for _, d := range fieldsOf(o).Deliverables {
					fmt.Fprintf(&b, "  - %s %s\n", check(d.Verified), d.Title)
				}
			}
		}
		b.WriteString("\n")
	}
	if n == 0 {
		fmt.Fprintf(&b, "%s\n\n", label(lang, "nothing"))
	}

	// Done requests aimed at no release, newest first.
	all, err := listRequests(ctx, cwm, "")
	if err != nil {
		return "", err
	}
	type item struct{ date, line string }
	var outside []item
	doneOn := doneDates(ctx)
	for _, o := range all {
		module, _ := cwmapi.ModuleOfObjectID(o.ID)
		if stateOf(o) != api.StateDone || edgeTo(o, api.EdgeTargets) != "" || !inScope(module) {
			continue
		}
		outside = append(outside, item{doneOn[o.ID], fmt.Sprintf("%s — %s (`%s`)", doneOn[o.ID], o.Name, module)})
	}
	sort.SliceStable(outside, func(i, j int) bool { return outside[i].date > outside[j].date })
	fmt.Fprintf(&b, "## %s\n\n", label(lang, "outside"))
	if len(outside) == 0 {
		fmt.Fprintf(&b, "%s\n\n", label(lang, "none"))
	}
	for _, it := range outside {
		fmt.Fprintf(&b, "- %s\n", it.line)
	}
	if len(outside) > 0 {
		b.WriteString("\n")
	}

	return b.String(), nil
}

// ReleaseBoard renders a release's page.
func (v *Views) ReleaseBoard(ctx context.Context, in api.ViewInput) (api.DocumentsOutput, error) {
	body, title, err := v.releaseBoard(ctx, in.ID, in.Lang, "")
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	return api.DocumentsOutput{Documents: []render.Document{page("release/"+in.ID, "", title, body)}}, nil
}

// releaseBoard writes a release's board — its scope, the module releases that
// ship in it, its plan in figures; prefix names the stream in its title.
func (v *Views) releaseBoard(ctx context.Context, id, lang, prefix string) (string, string, error) {
	s, err := NewReleases().Scope(ctx, api.GetInput{ID: id})
	if err != nil {
		return "", "", err
	}
	r := s.Release
	title := label(lang, "release") + " " + r.Module + " " + r.Version
	if r.Project {
		title = label(lang, "release") + " " + r.Version
	}
	if r.Name != "" && r.Name != r.Version {
		title += " — " + r.Name
	}
	if prefix != "" {
		title = prefix + " — " + title
	}
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s | %s | %s |\n|---|---|---|---|\n| `%s` | %s | %s | %d/%d |\n\n",
		label(lang, "module"), label(lang, "state"), label(lang, "date"), label(lang, "progress"),
		r.Module, r.State, dash(r.Date), s.Done, s.Total)
	fmt.Fprintf(&b, "## %s\n\n", label(lang, "scope"))
	if len(s.Targets) == 0 {
		fmt.Fprintf(&b, "%s\n\n", label(lang, "none"))
	}
	for _, t := range s.Targets {
		fmt.Fprintf(&b, "- %s %s", check(t.Done), dash(t.Title))
		if t.Via != "" {
			fmt.Fprintf(&b, " _(%s)_", t.Via)
		}
		b.WriteString("\n")
	}
	if len(s.Targets) > 0 {
		b.WriteString("\n")
	}
	if len(s.Modules) > 0 {
		fmt.Fprintf(&b, "## %s\n\n", label(lang, "module_releases"))
		for _, m := range s.Modules {
			fmt.Fprintf(&b, "- `%s` %s — %s\n", m.Module, m.Version, m.State)
		}
		b.WriteString("\n")
	}
	net, err := NewPlanning().Network(ctx, api.GetInput{ID: id})
	if err != nil {
		return "", "", err
	}
	ended := 0
	for _, n := range net.Nodes {
		if n.Ended {
			ended++
		}
	}
	fmt.Fprintf(&b, "## %s\n\n| %s | %s | %s |\n|---|---|---|\n| %s | %d/%d | %g |\n",
		label(lang, "plan"), label(lang, "budget"), label(lang, "progress"), label(lang, "duration"),
		money(r.BudgetUSD), ended, len(net.Nodes), net.Duration)
	return b.String(), title, nil
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// ReleasePlan renders a release's plan: its network, its sprints (steps), the
// critical path, what can start now.
func (v *Views) ReleasePlan(ctx context.Context, in api.ViewInput) (api.DocumentsOutput, error) {
	body, title, err := v.releasePlan(ctx, in.ID, in.Lang, "")
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	return api.DocumentsOutput{Documents: []render.Document{page("plan/"+in.ID, "", title, body)}}, nil
}

func (v *Views) releasePlan(ctx context.Context, id, lang, prefix string) (string, string, error) {
	planning := NewPlanning()
	got, err := planning.GetPlan(ctx, api.GetInput{ID: id})
	if err != nil {
		return "", "", err
	}
	plan := got.Plan
	net, err := planning.Network(ctx, api.GetInput{ID: id})
	if err != nil {
		return "", "", err
	}
	rel, err := NewReleases().Get(ctx, api.GetInput{ID: id})
	if err != nil {
		return "", "", err
	}
	r := rel.Release
	title := r.Module + " " + r.Version + " — " + label(lang, "plan")
	if prefix != "" {
		title = prefix + " — " + title
	}
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s | %s | %s |\n|---|---|---|---|\n| %s | %s | %s | %s |\n\n",
		label(lang, "release"), label(lang, "state"), label(lang, "budget"), label(lang, "module"),
		r.Version, r.State, money(r.BudgetUSD), "`"+r.Module+"`")
	nodes := map[string]api.NetNode{}
	for _, n := range net.Nodes {
		nodes[n.ID] = n
	}
	gates, err := gatesSection(ctx, lang, id)
	if err != nil {
		return "", "", err
	}
	b.WriteString(gates)
	// The network, drawn: done nodes green, the critical path in red.
	fmt.Fprintf(&b, "## %s\n\n```mermaid\nflowchart LR\n", label(lang, "network"))
	alias := map[string]string{}
	for i, n := range net.Nodes {
		alias[n.ID] = fmt.Sprintf("n%d", i+1)
	}
	for _, n := range net.Nodes {
		shape := `["%s"]`
		if n.Type == api.StepType {
			shape = `{{"%s"}}`
		}
		fmt.Fprintf(&b, "  %s"+shape+"\n", alias[n.ID], strings.ReplaceAll(n.Title, `"`, "'"))
	}
	for _, n := range net.Nodes {
		for _, p := range n.After {
			fmt.Fprintf(&b, "  %s --> %s\n", alias[p], alias[n.ID])
		}
	}
	for _, n := range net.Nodes {
		switch {
		case n.Ended:
			fmt.Fprintf(&b, "  style %s fill:#d3f9d8\n", alias[n.ID])
		case n.Critical:
			fmt.Fprintf(&b, "  style %s stroke:#e03131,stroke-width:3px\n", alias[n.ID])
		}
	}
	b.WriteString("```\n\n")

	// Sprints: the steps, in the order they are reached.
	steps := append([]api.Step{}, plan.Steps...)
	sort.SliceStable(steps, func(i, j int) bool {
		a, c := nodes[steps[i].ID], nodes[steps[j].ID]
		if a.EarliestStart != c.EarliestStart {
			return a.EarliestStart < c.EarliestStart
		}
		return steps[i].ID < steps[j].ID
	})
	held := map[string]bool{}
	for i, s := range steps {
		fmt.Fprintf(&b, "## %s %d — %s (%s)\n\n", label(lang, "sprint"), i+1, s.Name, s.State)
		if s.Produces != "" {
			fmt.Fprintf(&b, "**%s:** %s\n\n", label(lang, "produces"), s.Produces)
		}
		fmt.Fprintf(&b, "%s:\n\n", label(lang, "holds"))
		for _, p := range s.After {
			held[p] = true
			n := nodes[p]
			fmt.Fprintf(&b, "- %s %s _(%s)_\n", check(n.Ended && n.State != api.StateDropped), dash(n.Title), n.State)
		}
		if len(s.After) == 0 {
			fmt.Fprintf(&b, "%s\n", label(lang, "none"))
		}
		b.WriteString("\n")
	}
	var loose []string
	for _, n := range net.Nodes {
		if n.Request && !held[n.ID] {
			loose = append(loose, fmt.Sprintf("- %s %s _(%s)_", check(n.Ended && n.State != api.StateDropped), n.Title, n.State))
		}
	}
	if len(loose) > 0 {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", label(lang, "unplanned"), strings.Join(loose, "\n"))
	}
	fmt.Fprintf(&b, "## %s\n\n", label(lang, "critical"))
	if len(net.CriticalPath) == 0 {
		fmt.Fprintf(&b, "%s\n\n", label(lang, "none"))
	} else {
		names := make([]string, len(net.CriticalPath))
		for i, id := range net.CriticalPath {
			names[i] = nodes[id].Title
		}
		fmt.Fprintf(&b, "%s — %s: %g %s\n\n", strings.Join(names, " → "), label(lang, "duration"), net.Duration, label(lang, "unit"))
	}
	fmt.Fprintf(&b, "## %s\n\n", label(lang, "runnable"))
	if len(net.Runnable) == 0 {
		fmt.Fprintf(&b, "%s\n", label(lang, "none"))
	}
	for _, id := range net.Runnable {
		fmt.Fprintf(&b, "- %s\n", nodes[id].Title)
	}
	return b.String(), title, nil
}

// Stream renders the whole page set: a root, the change log, the releases, a
// board and a plan per release — parents before children.
func (v *Views) Stream(ctx context.Context, in api.StreamInput) (api.DocumentsOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return api.DocumentsOutput{}, invalid("Stream: name required")
	}
	lang := in.Lang
	all, err := NewReleases().List(ctx, api.ListInput{})
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	log, err := v.changeLog(ctx, api.ChangeLogInput{Lang: lang})
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	var index strings.Builder
	fmt.Fprintf(&index, "| %s | %s | %s | %s |\n|---|---|---|---|\n", label(lang, "release"), label(lang, "module"), label(lang, "state"), label(lang, "date"))
	var children []render.Document
	open, shown := 0, 0
	for _, r := range all.Releases {
		if r.State == api.ReleaseDropped {
			continue
		}
		shown++
		if r.State != api.ReleaseReleased {
			open++
		}
		fmt.Fprintf(&index, "| %s %s | `%s` | %s | %s |\n", r.Version, dash(r.Name), r.Module, r.State, dash(r.Date))
		body, title, err := v.releaseBoard(ctx, r.ID, lang, name)
		if err != nil {
			return api.DocumentsOutput{}, err
		}
		children = append(children, page("release/"+r.ID, "releases", title, body))
		body, title, err = v.releasePlan(ctx, r.ID, lang, name)
		if err != nil {
			return api.DocumentsOutput{}, err
		}
		children = append(children, page("plan/"+r.ID, "release/"+r.ID, title, body))
	}
	if shown == 0 {
		index.Reset()
		fmt.Fprintf(&index, "%s\n", label(lang, "none"))
	}
	root := fmt.Sprintf("## %s\n\n%d %s\n", label(lang, "releases_index"), open, label(lang, "open_releases"))
	docs := []render.Document{
		page("stream", "", name+" — "+label(lang, "stream"), root),
		page("changelog", "stream", name+" — "+label(lang, "changelog"), log),
		page("releases", "stream", name+" — "+label(lang, "releases_index"), index.String()),
	}
	return api.DocumentsOutput{Documents: append(docs, children...)}, nil
}

// gatesSection is the release's gates: its ready-to-release checklist, what
// in it waits on a gate and what resolves that, and every move forced
// through a gate with the requirements it passed unmet.
func gatesSection(ctx context.Context, lang, id string) (string, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n**%s:**\n\n", label(lang, "gates"), label(lang, "ready_to_release"))
	gates, err := application.UseService[gatesapi.GatesService](ctx)
	if err != nil {
		return "", err
	}
	// The forced moves are the journal's (an object stores only its state);
	// without a journal none is listed.
	journal, _ := application.UseService[journalapi.JournalService](ctx)
	chk, err := gates.Check(ctx, gatesapi.CheckInput{ID: id, Gate: api.GateReadyToRelease})
	if err != nil {
		return "", err
	}
	for _, it := range chk.Checklist.Items {
		line := fmt.Sprintf("- %s %s", check(it.Status == gatesapi.Met), dash(it.Label))
		switch {
		case it.ConfirmedBy != nil:
			line += fmt.Sprintf(" — %s %s", label(lang, "confirmed_by"), it.ConfirmedBy.By.ID)
		case it.Kind == "person":
			line += " — " + label(lang, "awaits_person")
		case it.Reason != "":
			line += " — " + it.Reason
		}
		b.WriteString(line + "\n")
	}
	if len(chk.Checklist.Items) == 0 {
		fmt.Fprintf(&b, "%s\n", label(lang, "none"))
	}
	scope, err := NewReleases().Scope(ctx, api.GetInput{ID: id})
	if err != nil {
		return "", err
	}
	ids := []string{id}
	for _, t := range scope.Targets {
		ids = append(ids, t.ID)
	}
	var waiting, forced []string
	for _, oid := range ids {
		o, _, err := loadAny(ctx, cwm, oid)
		if err != nil {
			return "", err
		}
		if w, ok := gateWaitOf(o); ok {
			rs, err := resolutionsOf(ctx, cwm, o, w)
			if err != nil {
				return "", err
			}
			var res []string
			for _, r := range rs {
				switch {
				case r.Request != "":
					res = append(res, fmt.Sprintf("%s `%s` _(%s)_", r.Requirement, r.Request, r.State))
				case !r.open():
					res = append(res, fmt.Sprintf("%s — %s %s: %s", r.Requirement, label(lang, "validation"), label(lang, "answered"), r.verdict()))
				default:
					res = append(res, fmt.Sprintf("%s — %s `%s`", r.Requirement, label(lang, "validation"), r.Ask))
				}
			}
			waiting = append(waiting, fmt.Sprintf("- %s → %s, %s: %s", nameOf(o), w.To, w.Gate, dash(strings.Join(res, "; "))))
		}
		if journal == nil {
			continue
		}
		if moves, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: o.ID}); err == nil {
			for _, e := range moves.Entries {
				if e.Forced {
					line := fmt.Sprintf("- %s: %s → %s, %s %s", nameOf(o), e.From, e.To, e.By.ID, e.At)
					if unmet := forcedPast(e); len(unmet) > 0 {
						line += " — " + strings.Join(unmet, ", ")
					}
					if e.Note != "" {
						line += " — " + e.Note
					}
					forced = append(forced, line)
				}
			}
		}
	}
	if len(waiting) > 0 {
		fmt.Fprintf(&b, "\n**%s:**\n\n%s\n", label(lang, "waiting"), strings.Join(waiting, "\n"))
	}
	if len(forced) > 0 {
		fmt.Fprintf(&b, "\n**%s:**\n\n%s\n", label(lang, "forced"), strings.Join(forced, "\n"))
	}
	b.WriteString("\n")
	return b.String(), nil
}

// forcedPast names the requirements a forced move passed unmet: those of the
// gate() its guard called that failed.
func forcedPast(e cwmapi.LifecycleEntry) []string {
	cl, ok := gatesapi.FailingChecklist(e.Calls)
	if !ok {
		return nil
	}
	var names []string
	for _, it := range cl.Unmet() {
		names = append(names, it.Name)
	}
	return names
}
