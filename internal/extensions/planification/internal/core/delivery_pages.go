// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY SCREEN - the approved mock-up
// (modules/research-delivery-overview/delivery-overview-mockup.html): one
// screen of placed regions sharing a selection. The streams' lanes on a
// timeline; the selected release below - its header with its stage stepper,
// gates and next action, its increments as dense rows; the selected increment
// in the side panel. Each part is a page of its own, the pane of its object:
// the release pane is the release level, the increment pane the drawer, the
// stream pane its lane with its next release selected. The rows are the
// delivery data types' (api/delivery_rows.go), drawn by display's components;
// these builders only place them.

// The delivery data types the screen draws.
const (
	streamLaneSchema   = "ktw-agents:/data-type/stream-lane"
	releaseRowsSchema  = "ktw-agents:/data-type/release-rows"
	incrementRowSchema = "ktw-agents:/data-type/increment-row"
	// The sources of the screen's regions.
	releaseRowsPageURI = "ktw-agents:releases/rows-page"
)

// deliveryWords are the Delivery screen's own words, per language
// (deliveryWord).
var deliveryWords = map[string]map[string]string{
	"en": {
		"intro":       "Every stream on a timeline, with its release mode, its releases and its backlog. Every item shows its id, and a click on any item opens it: a stream, a release, an increment, a goal, a feature or a request. Colours are the Workflows view's: done, running, waiting, blocked, planned for.",
		"release":     "Click a release on the timeline to see what it ships and what holds it.",
		"no_stream":   "No stream yet: create one (streams create) to see its releases and its backlog here.",
		"tab_planned": "Planned delivery", "tab_open": "Open increments",
		"title": "Delivery",
		// The release's level.
		"ships": "Ships in this release (%d)", "nothing_ships": "Nothing ships in it yet.",
		"planned":      "Planned for this release (%d) — they ship in it once it is the stream's next release",
		"ship_planned": "Ship the %d planned increments",
		"next_release": " · the stream's next release", "due": "due %s",
		"days_late": " · %d days late", "days_left": " · %d days left",
		"freeze": "Freeze it", "deliver": "Deliver a candidate", "letter": "Write the letter of %s",
		"signoff": "Sign %s off", "promote": "Promote %s", "needs_you": "needs you",
		// Change release's options.
		"mode_ships-in": "ships in", "mode_planned-for": "planned for",
		// The lane cards and the increment's drawer.
		"card_planned": "+%d planned", "card_done": "%d/%d done",
		"stream_manual": "manual", "stream_periodic": "periodic", "stream_on-change": "on-change", "at_most_every": "at most every %s",
		"next_cut": "next: cut %s", "next_on_change": "next: when an increment is delivered",
		"next_no_period": "next: no valid period", "next_by_hand": "next: by hand",
		"eyebrow": "Increment · %s", "ships_in": "Ships in %s", "planned_for": "Planned for %s",
		"ships_once": " - ships once %s is out", "unassigned_no_stream": "Not assigned, and aimed at no stream",
		"unassigned": "Not assigned: it waits in its stream's backlog",
		// Where its proposal stands.
		"proposal_not_run":      "not run yet: running it analyses its delivery goals and proposes what delivers them",
		"proposal_analysed":     "running: its delivery goals are analysed",
		"proposal_proposed":     "proposed - %d operation(s) await your approval",
		"proposal_approved":     "approved - %d operation(s) await their apply",
		"proposal_apply_held":   ": its work waits for the increment to run",
		"proposal_applied":      "approved and applied",
		"proposal_applied_runs": "approved and applied - its work runs",
	},
	"fr": {
		"intro":       "Chaque flux sur une frise, avec son mode de livraison, ses versions et son backlog. Chaque élément montre son id, et un clic sur n'importe lequel l'ouvre : un flux, une version, un incrément, un objectif, une fonctionnalité ou une demande. Les couleurs sont celles de la vue Workflows : fait, en cours, en attente, bloqué, prévu pour.",
		"release":     "Cliquez sur une version de la frise pour voir ce qu'elle livre et ce qui la retient.",
		"no_stream":   "Aucun flux pour l'instant : créez-en un (streams create) pour voir ici ses versions et son backlog.",
		"tab_planned": "Livraison planifiée", "tab_open": "Incréments ouverts",
		"title": "Livraison",
		"ships": "Livrés dans cette version (%d)", "ships_one": "Livré dans cette version (%d)",
		"nothing_ships": "Rien n'y est encore livré.",
		"planned":       "Prévus pour cette version (%d) — ils y sont livrés dès qu'elle est la prochaine version du flux",
		"planned_one":   "Prévu pour cette version (%d) — il y est livré dès qu'elle est la prochaine version du flux",
		"ship_planned":  "Livrer les %d incréments prévus", "ship_planned_one": "Livrer l'incrément prévu",
		"next_release": " · la prochaine version du flux", "due": "échéance %s",
		"days_late": " · %d jours de retard", "days_late_one": " · %d jour de retard",
		"days_left": " · %d jours restants", "days_left_one": " · %d jour restant",
		"finish": "Terminer %d incréments, puis la geler", "finish_one": "Terminer %d incrément, puis la geler",
		"freeze": "La geler", "deliver": "Livrer une version candidate", "letter": "Écrire la lettre de %s",
		"signoff": "Valider %s", "promote": "Promouvoir %s", "needs_you": "à vous",
		"legend_done": "fait", "legend_running": "en cours", "legend_waiting": "en attente",
		"legend_todo": "à faire", "legend_blocked": "bloqué",
		"mode_ships-in": "livré dans", "mode_planned-for": "prévu pour",
		"card_planned": "+%d prévus", "card_planned_one": "+%d prévu", "card_done": "%d/%d faits",
		"stream_manual": "manuel", "stream_periodic": "périodique", "stream_on-change": "à chaque livraison", "at_most_every": "au plus tous les %s",
		"next_cut": "prochaine coupe : %s", "next_on_change": "prochaine coupe : quand un incrément est livré",
		"next_no_period": "prochaine coupe : aucune période valide", "next_by_hand": "prochaine coupe : à la main",
		"eyebrow": "Incrément · %s", "ships_in": "Livré dans %s", "planned_for": "Prévu pour %s",
		"ships_once": " - livré dès que %s est sortie", "unassigned_no_stream": "Non assigné, et rattaché à aucun flux",
		"unassigned":            "Non assigné : il attend dans le backlog de son flux",
		"proposal_not_run":      "pas encore lancé : le lancer analyse ses objectifs de livraison et propose ce qui les livre",
		"proposal_analysed":     "en cours : ses objectifs de livraison sont analysés",
		"proposal_proposed":     "proposé - %d opération(s) attendent votre validation",
		"proposal_approved":     "validé - %d opération(s) attendent leur application",
		"proposal_apply_held":   " : son travail attend que l'incrément soit lancé",
		"proposal_applied":      "validé et appliqué",
		"proposal_applied_runs": "validé et appliqué - son travail est en cours",
	},
}

// waitWords are the words of what an increment waits on (incrementWaitsIn),
// per language: the Delivery rows' « waits » column.
var waitWords = map[string]map[string]string{
	"en": {
		"on_hold": "on hold: ", "waits_on": "waits on ", "its_proposal": "its proposal",
		"awaits_confirm": ": awaits a person's confirmation", "no_work": ": no work planned",
		"step_failed": "a step failed or was stopped", "blocked": " is blocked: ",
	},
	"fr": {
		"on_hold": "suspendu : ", "waits_on": "attend ", "its_proposal": "sa proposition",
		"awaits_confirm": " : attend la confirmation d'une personne", "no_work": " : aucun travail planifié",
		"step_failed": "une étape a échoué ou a été arrêtée", "blocked": " — blocage : ",
		"ask":        " attend une personne (%s)",
		"ask_answer": " attend la réponse d'une personne", "ask_validate": " attend la validation d'une personne",
		"ask_decide": " attend la décision d'une personne", "ask_challenge": " attend la contestation d'une personne",
		"ask_provide": " attend ce qu'une personne fournit", "ask_confirm": " attend la confirmation d'une personne",
	},
}

// waitWord is a word of what an increment waits on, in lang.
func waitWord(lang, key string) string {
	if deliveryLocal(lang) {
		if s, ok := waitWords[lang][key]; ok {
			return s
		}
	}
	return waitWords["en"][key]
}

// gateWords are a gate's reasons in French (gateWhy), each by its English
// form: whole, or the prefix before a title or a candidate's name.
var gateWords = map[string][][2]string{
	"fr": {
		{"needs a candidate", "attend une version candidate"},
		{"no candidate delivered yet", "aucune version candidate livrée"},
		{"open: ", "ouverts : "},
		{"not run on ", "non lancé sur "},
		{"its work is open: ", "son travail est ouvert : "},
	},
}

// gateWhy is a gate's reason in lang: its English
// words replaced by lang's where gateWords knows them - « no person signed
// <candidate> off » too -, the titles and names kept; the reason as it is
// in English.
func gateWhy(lang, why string) string {
	if !deliveryLocal(lang) || why == "" {
		return why
	}
	for _, w := range gateWords[lang] {
		if why == w[0] {
			return w[1]
		}
		if strings.HasSuffix(w[0], " ") && strings.HasPrefix(why, w[0]) {
			return w[1] + strings.TrimPrefix(why, w[0])
		}
	}
	if name, ok := strings.CutPrefix(why, "no person signed "); ok && strings.HasSuffix(name, " off") && lang == "fr" {
		return "personne n'a validé " + strings.TrimSuffix(name, " off")
	}
	return why
}

// localGates is a card's gate dots, their reasons in lang.
func localGates(lang string, gates []api.GateDot) []api.GateDot {
	if !deliveryLocal(lang) {
		return gates
	}
	out := make([]api.GateDot, len(gates))
	for i, g := range gates {
		g.Why = gateWhy(lang, g.Why)
		out[i] = g
	}
	return out
}

// deliveryWord is a word of the Delivery screen in lang.
func deliveryWord(lang, key string) string {
	if l, ok := deliveryWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return deliveryWords["en"][key]
}

// deliveryLocal is whether the Delivery screen speaks lang rather than
// English: a language other than English that it has words for.
func deliveryLocal(lang string) bool {
	_, ok := deliveryWords[lang]
	return ok && lang != "en"
}

// deliveryCount is the word of key for a count of n: its singular form
// (key_one) where the language has it - for one, and in French for zero too
// (« 0 jour restant »).
func deliveryCount(lang string, n int, key string) string {
	if (n == 1 || (n == 0 && lang == "fr")) && deliveryLocal(lang) {
		if s, ok := deliveryWords[lang][key+"_one"]; ok {
			return s
		}
	}
	return deliveryWord(lang, key)
}

// deliveryCountf is deliveryCount's word with n in it, where it has a place
// for it.
func deliveryCountf(lang string, n int, key string) string {
	w := deliveryCount(lang, n, key)
	if !strings.Contains(w, "%d") {
		return w
	}
	return fmt.Sprintf(w, n)
}

// deliveryLegend is the release level's legend, its words in lang.
func deliveryLegend(lang string) render.Node {
	words := []string{"done", "running", "waiting", "todo", "blocked"}
	if !deliveryLocal(lang) {
		return render.Legend(words...)
	}
	items := make([]any, len(words))
	for i, w := range words {
		items[i] = node{"word": w, "label": deliveryWord(lang, "legend_"+w)}
	}
	return render.Node{"kind": "legend", "items": items}
}

// deliveryScreen is the Delivery view: the lanes, the selected release's
// level below them, the selected increment's drawer beside.
func deliveryScreen(ctx context.Context, cwm cwmapi.CWMService, lanes []api.StreamLane, title, lang string) render.Def {
	release, increment := firstSelection(ctx, cwm, lanes)
	if deliveryLocal(lang) {
		// The release cards' gate reasons in the person's language.
		local := make([]api.StreamLane, len(lanes))
		for i, l := range lanes {
			l.Releases = append(make([]api.ReleaseCard, 0, len(l.Releases)), l.Releases...)
			for j := range l.Releases {
				l.Releases[j].Gates = localGates(lang, l.Releases[j].Gates)
			}
			local[i] = l
		}
		lanes = local
	}
	// The mock-up: the timeline full width on top; below it the selected
	// release on the left and the selected increment on the right.
	// Each is a section fed by its own load:
	// its args read the page's selection.
	releaseArgs := node{"id": "${selection.release}"}
	if lang != "" {
		releaseArgs["language"] = lang
	}
	releaseRegion := loadedSection("release", releaseRowsPageURI, releaseArgs,
		deliveryWord(lang, "release"), render.PlaceholderRows, 4)
	// The short panel of the increment picked: its
	// proposal and requests stay on its own page.
	pick := panelWord(lang, "pick")
	sideArgs := node{"id": "${selection.increment}"}
	if lang != "" {
		sideArgs["language"] = lang
	}
	side := loadedSection("increment", panelPageURI, sideArgs, pick, render.PlaceholderSkeleton, 4)
	kids := []node{
		{"kind": "text", "content": deliveryWord(lang, "intro"), "size": "sm", "color": "dimmed"},
		{"kind": "timeline-lanes", "schema": streamLaneSchema, "variant": "default", "data": lanes, "data-view": "delivery-lanes"},
		{"kind": "split", "sideWidth": 380, "sticky": true, "main": releaseRegion, "side": side},
	}
	if len(lanes) == 0 {
		kids = []node{kids[0], render.Dimmed(deliveryWord(lang, "no_stream"))}
	}
	tree := node{"kind": "vbox", "gap": "md", "data-view": "delivery", "children": kids}
	selection := node{}
	if release != "" {
		selection["release"] = release
	}
	if increment != "" {
		selection["increment"] = increment
	}
	return render.Def{Kind: "page", Title: title, DSLTree: tree, Data: node{"selection": selection}}
}

// firstSelection is what the screen opens on: the first lane's next release
// (else its first release still to ship), and that release's first increment.
func firstSelection(ctx context.Context, cwm cwmapi.CWMService, lanes []api.StreamLane) (release, increment string) {
	for _, l := range lanes {
		for _, c := range l.Releases {
			if c.Next {
				release = c.ID
				break
			}
		}
		if release == "" {
			for _, c := range l.Releases {
				if c.Stage != api.ReleaseReleased && c.Stage != api.ReleaseDropped {
					release = c.ID
					break
				}
			}
		}
		if release != "" {
			break
		}
	}
	if release == "" {
		return "", ""
	}
	if rel, err := loadRelease(ctx, cwm, release); err == nil {
		for _, inc := range carriedIncrements(ctx, cwm, rel) {
			return release, inc.ID
		}
	}
	return release, ""
}

// RowsPage answers a release's level: the region below the timeline, and the
// release's own pane.
func (r *Releases) RowsPage(ctx context.Context, in api.ReleaseRowsPageInput) (render.Def, error) {
	lang := in.Language
	rows, err := r.rowsIn(ctx, in.ID, lang)
	if err != nil {
		return render.Def{}, err
	}
	kids := []node{
		// Its header carries its next action, at its top right.
		{"kind": "view", "schema": releaseRowsSchema, "variant": "header", "data": rows},
		deliveryLegend(lang),
		render.SectionLabel(deliveryCountf(lang, len(rows.Ships), "ships")),
	}
	if len(rows.Ships) > 0 {
		kids = append(kids, node{"kind": "table", "schema": incrementRowSchema, "variant": "default", "data": rows.Ships, "data-view": "ships"})
	} else {
		kids = append(kids, render.Dimmed(deliveryWord(lang, "nothing_ships")))
	}
	if len(rows.Planned) > 0 {
		kids = append(kids,
			render.SectionLabel(deliveryCountf(lang, len(rows.Planned), "planned")),
			node{"kind": "table", "schema": incrementRowSchema, "variant": "default", "data": rows.Planned, "data-view": "planned"})
		if rows.ShipPlanned {
			label := deliveryCountf(lang, len(rows.Planned), "ship_planned")
			kids = append(kids, render.Button(label, render.RunURI(label, releaseShipPlannedURI, map[string]any{"id": rows.Release.ID, "by": "${person}"})))
		}
	}
	page := node{"kind": "vbox", "gap": "sm", "data-view": "release-level", "data-release": rows.Release.ID, "children": kids}
	return render.Def{Kind: "page", Title: rows.Title, DSLTree: page}, nil
}

// openRows counts the rows not done.
func openRows(rows []api.IncrementRow) int {
	n := 0
	for _, r := range rows {
		if r.Standing != api.StandingDone && r.Standing != api.StandingShipped && r.Standing != api.StandingDropped {
			n++
		}
	}
	return n
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// drawerButton is a button opening a form in a side drawer.
func drawerButton(label, title string, form node) node {
	return render.Button(label, render.Action{Label: label, Drawer: &render.Drawer{Title: title, Node: form}}).With("variant", "default")
}

// idChip is an object's id as a mono chip opening it.
func idChip(id string) node {
	return node{"kind": "button", "label": id, "variant": "subtle", "size": "xs", "mono": true, "data-open": id,
		"action": render.NavigateTo("Open "+id, id, "")}
}

// loadedSection is a part of a planning page fed by its own load
// : method run with args - their
// ${selection.*} read against the page's selection -, empty shown while a key
// they read is unset, a placeholder of kind and count while it loads.
func loadedSection(id, method string, args node, empty, kind string, count int) node {
	return render.LoadedSection(id, render.Load{Method: method, Args: args}, render.Placeholder{Kind: kind, Count: count}, empty)
}
