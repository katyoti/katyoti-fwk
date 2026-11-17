// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// ModuleDashboard is the module's status (ModuleStatus) as a page: the
// declared Dashboard panel (ktw-code's object-panels/module-dashboard.yaml)
// shows the answer as is, so the GUI holds no gocode view. The page reads, top
// down: the health, the tier reached against its target, the use-cases as a
// stacked bar, the audit findings and open decisions, the promote gates still
// blocking (or the all-clear) and the facets grouped by kind.
func (s *GocodeServiceImpl) ModuleDashboard(ctx stdctx.Context, in api.ModuleDashboardInput) (api.ModuleDashboardOutput, error) {
	st, err := s.ModuleStatus(ctx, api.ModuleStatusInput{ID: in.ID})
	if err != nil {
		return api.ModuleDashboardOutput{}, err
	}
	return api.ModuleDashboardOutput{Def: render.Def{Kind: "page", Title: "Dashboard", DSLTree: dashboardPage(st)}}, nil
}

// dashboardPage draws a status; pure, so the page is tested apart from CWM.
func dashboardPage(st api.ModuleStatusOutput) render.Node {
	m := st.Module
	if m.ID == "" {
		return render.Card(render.Text("No module status for this object.").With("size", "sm").With("color", "dimmed"))
	}
	kind := string(m.Kind)
	if kind == "" {
		kind = "—"
	}
	head := render.HBox(
		render.VBox(render.Heading("Module status", 5), render.Dimmed("gocode — kind: "+kind)).With("gap", 2),
	).With("justify", "space-between").With("align", "flex-start").With("wrap", false)
	if h := string(m.Health); h != "" {
		tone, icon := healthLook(h)
		head["children"] = append(head["children"].([]render.Node),
			render.Badge(h, tone).With("variant", "filled").With("size", "lg").With("icon", icon))
	}
	body := []render.Node{head, tierSection(m), ucSection(st.UCTotals), auditSection(st), gatesSection(st.BlockingGates)}
	if len(m.Facets) > 0 {
		body = append(body, render.Divider().With("variant", "dashed"), facetsSection(m.Facets))
	}
	return render.Card(body...)
}

func healthLook(h string) (tone, icon string) {
	switch h {
	case "healthy":
		return "green", "circle-check"
	case "degraded":
		return "yellow", "alert-circle"
	case "broken":
		return "red", "circle-x"
	}
	return "gray", "alert-circle"
}

// sub is a compact section of the dashboard card: an icon and a label row.
func sub(title, icon string, children ...render.Node) render.Node {
	return render.Section(title, icon, children...).With("size", "sm")
}

func tierSection(m api.Module) render.Node {
	reached := 0.0
	if m.TierTarget > 0 {
		reached = min(100, float64(m.TierCurrent)/float64(m.TierTarget)*100)
	}
	aside := []render.Node{
		render.Badge(fmt.Sprint(m.TierCurrent), "blue").With("size", "md"),
		render.Dimmed("→"),
		render.Badge(fmt.Sprintf("target %d", m.TierTarget), "blue").With("size", "md").With("variant", "outline"),
	}
	if st := string(m.TierState); st != "" {
		aside = append(aside, render.Badge(st, tierStateTone(st)).With("variant", "dot"))
	}
	bar := render.Gauge("", reached, 100).With("color", "blue").With("size", "sm").With("valueText", "")
	return sub("Tier", "trending-up", bar).Aside(aside...)
}

func tierStateTone(state string) string {
	switch state {
	case "specifying", "planning":
		return "cyan"
	case "executing":
		return "blue"
	case "verifying":
		return "indigo"
	case "auditing":
		return "violet"
	case "gated":
		return "orange"
	case "promoted":
		return "green"
	}
	return "gray"
}

func ucSection(t api.TestRunTotals) render.Node {
	if t.UCTotal == 0 {
		return sub("Use-cases", "shield-check").Empty("No UCs authored yet.", "")
	}
	// Greens first, then yellow, then reds — the order the bar is read in.
	bar := render.Stacked("",
		render.Segment{Value: float64(t.Pass), Color: "green", Label: "pass"},
		render.Segment{Value: float64(t.FailDeferred), Color: "yellow", Label: "fail-deferred"},
		render.Segment{Value: float64(t.NoTest), Color: "orange", Label: "no-test"},
		render.Segment{Value: float64(t.Skipped), Color: "blue", Label: "skipped"},
		render.Segment{Value: float64(t.Fail), Color: "red", Label: "fail"},
	).With("size", "lg").With("valueText", "").With("max", t.UCTotal)
	return sub("Use-cases", "shield-check", bar).Aside(render.Dimmed(fmt.Sprintf("%d total", t.UCTotal)))
}

// counter is a badge that lights up (filled, in its tone) when its count is
// not zero, and stays a gray light badge otherwise.
func counter(n int, label, tone string) render.Node {
	if n == 0 {
		return render.Badge(fmt.Sprintf("%d %s", n, label), "gray")
	}
	return render.Badge(fmt.Sprintf("%d %s", n, label), tone).With("variant", "filled")
}

func auditSection(st api.ModuleStatusOutput) render.Node {
	ods := "open ODs"
	if st.OpenODCount == 1 {
		ods = "open OD"
	}
	return sub("Audit + open decisions", "alert-circle",
		render.HBox(counter(st.AuditCritOpen, "critical", "red"), counter(st.AuditHighOpen, "high", "orange"), counter(st.OpenODCount, ods, "cyan")),
	)
}

func gatesSection(gates []string) render.Node {
	var rows []render.Node
	for _, g := range gates {
		rows = append(rows, render.HBox(render.Dimmed("•"), render.Text(g).With("size", "xs")).
			With("gap", 6).With("wrap", false).With("align", "flex-start"))
	}
	s := sub("Promote gates", "clock", rows...).Empty("All gates clear — promote should succeed.", "green")
	if len(gates) > 0 {
		s = s.With("badge", fmt.Sprintf("%d blocking", len(gates))).With("badgeColor", "orange").With("gap", 2)
	}
	return s
}

var facetLabels = map[string]string{
	"code": "Code packages", "docs": "Docs", "ucs": "Use cases", "tests": "Tests",
	"app": "App folder", "charter": "Charter", "manifest": "Manifest",
}

var facetOrder = []string{"code", "app", "ucs", "tests", "docs", "charter", "manifest"}

// facetsSection lists the module's typed paths, one labelled chip row per
// facet kind present, known kinds first in their order.
func facetsSection(facets []api.Facet) render.Node {
	byKind := map[string][]api.Facet{}
	var kinds []string
	for _, f := range facets {
		k := string(f.Kind)
		if _, seen := byKind[k]; !seen {
			kinds = append(kinds, k)
		}
		byKind[k] = append(byKind[k], f)
	}
	ordered := []string{}
	for _, k := range facetOrder {
		if _, ok := byKind[k]; ok {
			ordered = append(ordered, k)
		}
	}
	for _, k := range kinds {
		if _, known := facetLabels[k]; !known {
			ordered = append(ordered, k)
		}
	}
	var groups []render.Node
	for _, k := range ordered {
		label := facetLabels[k]
		if label == "" {
			label = k
		}
		var chips []render.Node
		for _, f := range byKind[k] {
			chip := render.Badge(f.Path, "gray").With("size", "xs").With("variant", "default").With("mono", true)
			if f.Role != "" {
				chip = chip.With("tooltip", f.Role)
			}
			chips = append(chips, chip)
		}
		groups = append(groups, render.VBox(render.Dimmed(label), render.HBox(chips...).With("gap", 4)).With("gap", 4))
	}
	return render.VBox(groups...).With("gap", 6)
}
