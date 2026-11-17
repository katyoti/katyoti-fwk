// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// DELIVERY'S « DELIVERED » TAB (view 5 « Delivered
// increments » of the mock-up
// modules/research-delivery-overview/delivery-views-v2-mockup.html) - beside
// Open increments, its badge
// the count of the increments done. The tab is a section (DeliveredPage): the
// increments done grouped by the release they shipped in, newest release
// first (its date, then its id), « No release » last; each group a fold - its
// release, its last candidate and its count -, the newest open; each
// increment the planification card: its id, its
// title, its number of requests, the day its delivery was approved and what
// it delivered in two lines at most (its description), the newest approved
// first.

// The Delivered tab's source and element ids, read by its GUI test.
const (
	deliveredPageURI    = "ktw-agents:increments/delivered-page"
	deliveredTabView    = "delivery-delivered"
	deliveredGroupsView = "delivered-groups"
	deliveredFoldPrefix = "delivered-"
	deliveredNoRelease  = "none"
)

// deliveredWords are the tab's words, per language.
var deliveredWords = map[string]map[string]string{
	"en": {
		"tab": "Delivered", "requests": "%d requests", "requests_one": "%d request",
		"approved": "approved %s", "no_release": "No release",
		"empty": "No increment is delivered yet.", "loading": "The increments delivered load…",
	},
	"fr": {
		"tab": "Livrés", "requests": "%d demandes", "requests_one": "%d demande",
		"approved": "approuvé le %s", "no_release": "Sans version",
		"empty": "Aucun incrément n'est encore livré.", "loading": "Les incréments livrés se chargent…",
	},
}

// deliveredWord is a word of the tab in lang, English when lang has none.
func deliveredWord(lang, key string) string {
	if l, ok := deliveredWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return deliveredWords["en"][key]
}

// deliveredRequests is « n requests » in lang: the singular for one - and for
// none in French.
func deliveredRequests(lang string, n int) string {
	key := "requests"
	if n == 1 || (n == 0 && lang == "fr") {
		key = "requests_one"
	}
	return fmt.Sprintf(deliveredWord(lang, key), n)
}

// deliveredCount is the tab's badge: the increments done.
func deliveredCount(ctx context.Context) int {
	list, err := NewIncrements().List(ctx, api.ListIncrementsInput{State: api.IncrementDone})
	if err != nil {
		return 0
	}
	return len(list.Increments)
}

// deliveredTab is the « Delivered » tab: the section reading DeliveredPage.
func deliveredTab(lang string) node {
	section := loadedSection(deliveredGroupsView, deliveredPageURI, node{"language": lang},
		deliveredWord(lang, "loading"), render.PlaceholderRows, 4)
	return node{"kind": "vbox", "gap": "sm", "data-view": deliveredTabView, "children": []node{section}}
}

// deliveredLine is one increment delivered, read once.
type deliveredLine struct {
	ID, Title, Description string
	Requests               int
	// Approved is when its delivery was approved (RFC 3339); empty when no
	// approval is recorded (closed by its verb).
	Approved string
}

// deliveredGroup is the increments delivered in one release.
type deliveredGroup struct {
	ID, Label, Date string
	Lines           []deliveredLine
}

// DeliveredPage answers Delivery's Delivered tab: the
// increments done grouped by release, newest first, each group folding.
func (i *Increments) DeliveredPage(ctx context.Context, in api.DeliveredIncrementsInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	lang := in.Language
	groups, err := deliveredGroups(ctx, cwm, lang)
	if err != nil {
		return render.Def{}, err
	}
	var kids []node
	if len(groups) == 0 {
		kids = append(kids, render.Dimmed(deliveredWord(lang, "empty")))
	} else {
		folds := make([]render.Fold, 0, len(groups))
		for _, g := range groups {
			folds = append(folds, render.Fold{ID: deliveredFoldPrefix + g.ID, Label: g.Label,
				Badge: len(g.Lines), Content: deliveredRows(lang, g)})
		}
		acc := render.Accordion(folds, deliveredFoldPrefix+groups[0].ID)
		acc["data-view"] = deliveredGroupsView
		acc["data-count"] = len(groups)
		kids = append(kids, node(acc))
	}
	page := node{"kind": "vbox", "gap": "xs", "data-view": "delivered-page", "children": kids}
	return render.Def{Kind: "page", Title: deliveredWord(lang, "tab"), DSLTree: page}, nil
}

// deliveredGroups are the increments done, by the release they shipped in:
// the newest release first (its date, then its id), « No release » last; in a
// group the newest approved first, then by id.
func deliveredGroups(ctx context.Context, cwm cwmapi.CWMService, lang string) ([]deliveredGroup, error) {
	list, err := NewIncrements().List(ctx, api.ListIncrementsInput{State: api.IncrementDone})
	if err != nil {
		return nil, err
	}
	byRelease := map[string]*deliveredGroup{}
	var order []*deliveredGroup
	for _, inc := range list.Increments {
		o, err := loadIncrement(ctx, cwm, inc.ID)
		if err != nil {
			continue
		}
		rel := inc.ShipsIn
		if rel == "" {
			rel = deliveredNoRelease
		}
		g, ok := byRelease[rel]
		if !ok {
			g = &deliveredGroup{ID: rel, Label: deliveredWord(lang, "no_release")}
			if rel != deliveredNoRelease {
				g.Label, g.Date = deliveredReleaseLabel(ctx, cwm, rel)
			}
			byRelease[rel] = g
			order = append(order, g)
		}
		g.Lines = append(g.Lines, deliveredLine{ID: inc.ID, Title: inc.Title, Description: firstLine(inc.Description),
			Requests: deliveredRequestCount(ctx, cwm, o), Approved: approvedAt(o)})
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := order[a], order[b]
		if (x.ID == deliveredNoRelease) != (y.ID == deliveredNoRelease) {
			return y.ID == deliveredNoRelease
		}
		if x.Date != y.Date {
			return x.Date > y.Date
		}
		return idNumber(x.ID) > idNumber(y.ID)
	})
	out := make([]deliveredGroup, 0, len(order))
	for _, g := range order {
		sort.SliceStable(g.Lines, func(a, b int) bool {
			x, y := g.Lines[a], g.Lines[b]
			if x.Approved != y.Approved {
				return x.Approved > y.Approved
			}
			return idNumber(x.ID) > idNumber(y.ID)
		})
		out = append(out, *g)
	}
	return out, nil
}

// deliveredReleaseLabel names a release as the mock-up's groups do - its
// stream and version (« KaTyoti 1.005 »), its last candidate after
// (« · rc6 ») - and gives its date, the groups' order.
func deliveredReleaseLabel(ctx context.Context, cwm cwmapi.CWMService, id string) (label, date string) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return id, ""
	}
	f := releaseFields(o)
	label = api.ReleaseLabel(f.Version, titleOf(o))
	if sm := edgeTo(o, api.EdgeInStream); sm != "" && f.Version != "" {
		if name := titleOfID(ctx, cwm, sm); name != "" && name != sm {
			label = name + " " + f.Version
		}
	}
	if n := len(f.Candidates); n > 0 && f.Candidates[n-1].Name != "" {
		label += " · " + f.Candidates[n-1].Name
	}
	return label, f.Date
}

// deliveredRequestCount is how many requests an increment counts - those its
// pane counts (incrementRequestObjects), a dropped or split one left out.
func deliveredRequestCount(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) int {
	scope, err := scopeOf(ctx, cwm, o)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range incrementRequestObjects(ctx, cwm, o, scope) {
		if !uncounted(planState(r)) {
			n++
		}
	}
	return n
}

// approvedAt is when an increment's delivery was approved: the last approval
// of its delivery that succeeded, else its plan's last move to done.
func approvedAt(o cwmapi.Object) string {
	p, ok := planOf(o)
	if !ok {
		return ""
	}
	for n := len(p.Steps) - 1; n >= 0; n-- {
		s := p.Steps[n]
		if strings.HasSuffix(s.Step, "/approve-delivery") && s.Outcome == schedapi.StatusSuccess && s.EndedAt != "" {
			return s.EndedAt
		}
	}
	for n := len(p.Trail) - 1; n >= 0; n-- {
		if p.Trail[n].To == api.PlanDone {
			return p.Trail[n].At
		}
	}
	return ""
}

// approvedDay is an approval's day as the mock-up writes it (22/10), in the
// local time.
func approvedDay(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return ""
	}
	return t.Local().Format("02/01")
}

// deliveredRows are a group's increments, each the planification card: its id
// opening it, its title, its number of requests, its approval's day and what
// it delivered.
func deliveredRows(lang string, g deliveredGroup) node {
	cards := make([]card, 0, len(g.Lines))
	for _, l := range g.Lines {
		approved := "—"
		if day := approvedDay(l.Approved); day != "" {
			approved = fmt.Sprintf(deliveredWord(lang, "approved"), day)
		}
		cards = append(cards, card{ID: l.ID, Title: l.Title, OpenLabel: runningWord(lang, "open") + " " + l.ID, Tone: "green",
			Badges: []node{
				{"kind": "text", "content": deliveredRequests(lang, l.Requests), "size": "xs", "mono": true, "color": "dimmed", "data-requests": l.Requests},
				{"kind": "text", "content": approved, "size": "xs", "mono": true, "color": "dimmed", "data-approved": l.ID},
			},
			// What it delivered: its description, dimmed, two lines at most.
			Meta: l.Description, MetaLines: 2, MetaAttrs: node{"data-delivered-what": l.ID},
			Attrs: node{"data-increment": l.ID}})
	}
	return cardList(cards, 3, nil, node{"data-view": "delivered-rows", "data-release": g.ID})
}
