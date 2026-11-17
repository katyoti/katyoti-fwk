// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// blockersFixture is an increment held by one root of each kind but held
// (K4 empty) - the reader's own English lines -, an incident only the person
// clears, and one repair the reconciler made: the mock-up's groups.
func blockersFixture() api.IncrementBlockersOutput {
	b := func(kind, work, title, reason, actor, actorKind string) api.IncrementBlocker {
		return api.IncrementBlocker{Blocker: api.Blocker{Class: api.BlockerClass(kind), Kind: kind, Work: work, Title: title, Reason: reason, Actor: actor, ActorKind: actorKind, Since: "2026-10-20T08:12:00Z"}, Age: "4d"}
	}
	answer := b(api.BlockerAnswer, "shop.RQ-051", "Choose the dossier's export format", "its question waits for an answer in mail.E-12", "the person", api.ActorPerson)
	answer.Holds = []string{"shop.F-049", "shop.F-050"}
	act := b(api.BlockerAct, "shop.F-049", "The dossier opens on its overview", "the person's confirm: confirm the feature", "jdoe", api.ActorPerson)
	act.Act = "confirm"
	ready := b(api.BlockerReady, "docs.RQ-012", "The document engine assembles a dossier's sections", "any agent may take it now", "an agent: a loop on release:roadmap.RL-014", api.ActorAgent)
	ready.Chain = []string{"shop.F-033", "shop.F-050"}
	outside := b(api.BlockerOutside, "shop.RQ-049", "", "waits on the hub's GUI test runner", "core.RQ-053", api.ActorOutside)
	outside.Outside = "core.RQ-053, in no release"
	incident := b(api.BlockerIncident, "ui.F-050", "", "its step verify waits on it - nobody works on shop.F-032 (no open plan)", "the reconciler", api.ActorSystem)
	incident.On, incident.Incident, incident.Sweeps, incident.Cause = "shop.F-032", "wait-on-nothing:ui.F-050", 2, api.CauseNobodyWorks
	nowork := b(api.BlockerIncident, "shop.F-011", "The dossier is exported", "no work planned: nothing open works on it", "the person: plan its work", api.ActorPerson)
	nowork.Cause = api.CauseNoWork
	group := func(kind string, bs ...api.IncrementBlocker) api.IncrementBlockerGroup {
		return api.IncrementBlockerGroup{Class: api.BlockerClass(kind), Kind: kind, Title: api.BlockerGroupTitles[kind], Blockers: bs}
	}
	repaired := api.IncrementRepair{RepairDone: api.RepairDone{Repair: api.Repair{Repair: api.RepairEndPlan, Work: "store.F-085", Reason: "its lifecycle is done while its plan is still ready", Action: "end its plan done", Applied: true}, At: "2026-10-20T09:50:00Z"}, Title: "A plan closes after its feature is done"}
	return api.IncrementBlockersOutput{Increment: "roadmap.IN-046", Total: 6,
		Counts:   map[string]int{api.BlockerAnswer: 1, api.BlockerAct: 1, api.BlockerReady: 1, api.BlockerOutside: 1, api.BlockerIncident: 2},
		Groups:   []api.IncrementBlockerGroup{group(api.BlockerAnswer, answer), group(api.BlockerAct, act), group(api.BlockerReady, ready), group(api.BlockerOutside, outside), group(api.BlockerIncident, incident, nowork)},
		Repaired: []api.IncrementRepair{repaired}}
}

// fixtureFile is the tree the display library's bun test draws
// (app/ktw-display-lib/tests/increment-blockers.test.ts): written by this
// test with KTW_UPDATE_FIXTURE=1, compared otherwise.
var fixtureFile = filepath.Join("..", "..", "..", "..", "..", "app", "ktw-display-lib", "tests", "fixtures", "increment-blockers.json")

// THE INCREMENT'S BLOCKERS TAB draws `increments blockers <id>`
// exactly as its mock-up: the state « bloqué · N blocages » with Recalculer
// and the person's next action, a verdict with its counts (yours counting the
// incident only the person clears, and the repairs), the six groups K1-K6 in
// reading order - each with its lead, its count and « Rien. » when empty -
// each root a card striped in its kind's colour with its reason in French,
// who acts and the act it calls for (an incident its second way out), and
// what the reconciler repaired, folded last.
func TestTheIncrementsBlockersTabDrawsEachKindWithWhoActs(t *testing.T) {
	tree := blockersBody("fr", blockersFixture(), 2)
	raw, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)

	kids, _ := tree["children"].([]node)
	if len(kids) != 9 {
		t.Fatalf("%d children; want the state, the verdict, six groups and the repairs", len(kids))
	}
	// The state, Recalculer (the next round) and the person's next action.
	bar, _ := json.Marshal(kids[0])
	for _, want := range []string{"bloqué · 6 blocages", `"id":"blockers-state"`, `"id":"blockers-recompute"`, "Recalculer", `"key":"blockers_round","value":3`, `"id":"blockers-next"`, "Ma prochaine action", `"object":"shop.RQ-051"`} {
		if !strings.Contains(string(bar), want) {
			t.Errorf("the state line has no %s: %s", want, bar)
		}
	}
	// The verdict: it cannot end, three acts are the person's (the incident
	// only they clear among them), the counts, the repairs.
	verdict, _ := json.Marshal(kids[1])
	for _, want := range []string{`"callout":"red"`, "Cet incrément ne peut pas finir aujourd'hui.", "3 actions vous reviennent", "Pour vous 3", "Agents 1", "Extérieur 1", "Incidents 2", "Réparés 1"} {
		if !strings.Contains(string(verdict), want) {
			t.Errorf("the verdict has no %s: %s", want, verdict)
		}
	}

	// Six groups, K1 first, each with its count; K4 empty says « Rien. ».
	for k, want := range []struct {
		title string
		count int
	}{
		{"K1 · Vos réponses", 1}, {"K2 · Vos actions", 1}, {"K3 · Travail prêt pour un agent", 1},
		{"K4 · Travail en attente d'autre chose", 0}, {"K5 · En dehors de l'incrément", 1}, {"K6 · Incidents", 2},
	} {
		g := kids[k+2]
		if g["kind"] != "section" || g["title"] != want.title || g["badge"] != want.count || g["description"] == "" || g["empty"] != "Rien." {
			t.Errorf("group %d: kind %v title %v badge %v empty %v; want the section %q with %d", k+1, g["kind"], g["title"], g["badge"], g["empty"], want.title, want.count)
		}
		if rows, _ := g["children"].([]node); len(rows) != want.count {
			t.Errorf("%s: %d rows; want %d", want.title, len(rows), want.count)
		}
	}

	// Each root a card striped in its kind's colour, its reason in French,
	// who acts, the act it calls for.
	for _, want := range []string{
		`"id": "blocker-K1-shop.RQ-051"`, `"id": "blocker-K2-shop.F-049"`, `"id": "blocker-K3-docs.RQ-012"`, `"id": "blocker-K5-shop.RQ-049"`, `"id": "blocker-K6-ui.F-050"`, `"id": "blocker-K6-shop.F-011"`,
		`"stripe": "yellow"`, `"stripe": "orange"`, `"stripe": "grape"`, `"stripe": "red"`,
		"sa question attend une réponse dans mail.E-12 · depuis 4d · retient 2 de ses travaux",
		"votre confirmation : confirm the feature", "tout agent peut le prendre maintenant", "attend the hub's GUI test runner",
		"core.RQ-053, dans aucune version", "its step verify waits on it - personne ne travaille sur shop.F-032 (aucun plan ouvert)",
		"aucun travail prévu : rien d'ouvert n'y travaille",
		"Qui agit :", `"content": "Vous"`, `"content": "Vous (jdoe)"`, `"content": "Agent · un agent : une boucle sur release:roadmap.RL-014"`,
		`"content": "Responsable · core.RQ-053"`, `"content": "Système · le réconciliateur"`, `"content": "Vous : planifier son travail"`,
		`"label": "Répondre"`, `"label": "Confirmer"`, `"label": "Lancer une boucle roadmap.RL-014"`, "/k-loop --release roadmap.RL-014", `"label": "Ouvrir la demande"`,
		`"label": "Créer la demande sur shop.F-032"`, `"id": "blocker-alt-ui.F-050"`, `"label": "Retirer l'attente"`,
		`"id": "blocker-open-shop.F-011"`, `"label": "Planifier"`, `"uri": "ktw-agents:requests/file"`, `"label": "Sortir de l'incrément"`, `"uri": "ktw-agents:increments/untarget"`,
		"incident wait-on-nothing:ui.F-050 · vu par 2 balayages",
		`"text": "docs.RQ-012"`, `"text": "shop.F-050"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the tab has no %s", want)
		}
	}
	if strings.Contains(s, `"variant": "dot"`) {
		t.Error("a row still draws its kind as a dot badge; the mock-up stripes it")
	}

	// The repairs: folded last, each with what was done and why, in French.
	fold, _ := json.Marshal(kids[8])
	for _, want := range []string{`"kind":"accordion"`, "Réparé automatiquement (1) - par le réconciliateur, sans agent", `"data-repaired":"1"`, "store.F-085",
		"terminer son plan - son cycle de vie est terminé alors que son plan est encore ready · 20/10 09:50"} {
		if !strings.Contains(string(fold), want) {
			t.Errorf("the repairs' fold has no %s: %s", want, fold)
		}
	}
	if open := kids[8]["defaultOpen"]; open != nil {
		t.Errorf("the repairs' fold opens at first: %v", open)
	}

	// English for a person who reads English: the reader's lines as written.
	en, _ := json.Marshal(blockersBody("en", blockersFixture(), 0))
	for _, want := range []string{"K1 · Your answers", "This increment cannot end today.", "blocked · 6 blockers", "Recompute", "its question waits for an answer in mail.E-12", `"label":"Confirm"`, "Repaired 1"} {
		if !strings.Contains(string(en), want) {
			t.Errorf("the English tab has no %s", want)
		}
	}
	// Nothing holds it: a green verdict, no state chip, every group « Rien. ».
	free, _ := json.Marshal(blockersBody("fr", api.IncrementBlockersOutput{Increment: "x.IN-001", Counts: map[string]int{}}, 0))
	if !strings.Contains(string(free), `"callout":"green"`) || !strings.Contains(string(free), "Rien ne bloque cet incrément.") || strings.Contains(string(free), "blockers-next") || strings.Contains(string(free), "blockers-state") {
		t.Errorf("nothing holds it: %s", free)
	}

	// The display library draws this very tree.
	raw = append(raw, '\n')
	if os.Getenv("KTW_UPDATE_FIXTURE") == "1" {
		if err := os.WriteFile(fixtureFile, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(fixtureFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), raw) {
		t.Errorf("%s is not the tab's tree: run this test with KTW_UPDATE_FIXTURE=1", fixtureFile)
	}
}

// A ready root untaken for an hour asks for a loop on its scope; one ready a
// moment ago only opens; a K2 act is named by its kind.
func TestABlockersActIsNamedAsTheAct(t *testing.T) {
	ready := api.IncrementBlocker{Blocker: api.Blocker{Kind: api.BlockerReady, Work: "m.RQ-1", Actor: "an agent: a loop on increment:m.IN-002", ActorKind: api.ActorAgent,
		Since: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)}}
	acts, _ := json.Marshal(blockerActs("fr", "m.IN-002", api.BlockerReady, ready))
	if !strings.Contains(string(acts), "Lancer une boucle m.IN-002") || !strings.Contains(string(acts), "/k-loop --increment m.IN-002") {
		t.Errorf("ready two hours: %s", acts)
	}
	ready.Since = time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	if acts, _ := json.Marshal(blockerActs("fr", "m.IN-002", api.BlockerReady, ready)); strings.Contains(string(acts), "boucle") || !strings.Contains(string(acts), `"label":"Ouvrir"`) {
		t.Errorf("ready five minutes: %s", acts)
	}
	// The act is read from the blocker's typed act, never its reason: a
	// reason that says another act is not read.
	for act, want := range map[string]string{
		api.InteractionValidate: "Valider", api.InteractionDecide: "Décider", api.ActAccept: "Accepter",
		api.ActPlan: "Planifier", api.ActUnblock: "Débloquer", "": "Agir",
	} {
		b := api.IncrementBlocker{Blocker: api.Blocker{Kind: api.BlockerAct, Work: "m.F-1", Act: act, Reason: "its plan is blocked", ActorKind: api.ActorPerson}}
		if act == api.ActUnblock {
			b.Reason = "the person's validate: review"
		}
		if acts, _ := json.Marshal(blockerActs("fr", "m.IN-002", api.BlockerAct, b)); !strings.Contains(string(acts), `"label":"`+want+`"`) {
			t.Errorf("%q: want %s, got %s", act, want, acts)
		}
	}
}

// Every line the reader writes says itself in French, its parts kept and a
// nested reason said again; a line no rule knows is shown as written, and
// English stays English.
func TestABlockersReasonIsSaidInThePagesLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"a worker holds it": "un agent le tient",
		"module planification is being changed (held by m.TD-1)":                                           "le module planification est en cours de modification (tenu par m.TD-1)",
		"its claim by claude-a lapsed at 10:00: any agent may take it now":                                 "la prise par claude-a a expiré à 10:00 : tout agent peut le prendre maintenant",
		"waits on x.F-1 - at its root: its plan is blocked":                                                "attend x.F-1 - à sa racine : son plan est bloqué",
		"a cycle: a → b → a":                                                                               "un cycle : a → b → a",
		"its step waits - x.RQ-2 has ended, the wait was not cleared":                                      "its step waits - x.RQ-2 est terminé, l'attente n'a pas été levée",
		"the reconciler repaired it 3 times and it came back: its result was reported and never collected": "le réconciliateur l'a réparé 3 fois et c'est revenu : son résultat a été rapporté et jamais collecté",
		"x.RQ-9, in roadmap.RL-014":                                                                        "x.RQ-9, dans roadmap.RL-014",
		"a line no rule knows":                                                                             "a line no rule knows",
		"any agent may take it now - no live taker since 2026-11-09T10:00:00Z":                             "tout agent peut le prendre maintenant - aucune boucle ne le prend depuis 2026-11-09T10:00:00Z",
		"its claim by claude-a lapsed at 10:00: any agent may take it now - no live taker":                 "la prise par claude-a a expiré à 10:00 : tout agent peut le prendre maintenant - aucune boucle ne le prend",
		"claude-a (waiting on increment:m.IN-002)":                                                         "claude-a (en attente sur increment:m.IN-002)",
		"claude-a (working m.RQ-7)":                                                                        "claude-a (travaille sur m.RQ-7)",
		"claude-a (pulled 3 min ago)":                                                                      "claude-a (a tiré il y a 3 min)",
	} {
		if got := blockerSay("fr", in); got != want {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
	}
	if got := blockerSay("en", "a worker holds it"); got != "a worker holds it" {
		t.Errorf("English changed: %q", got)
	}
}

// The incidents' list keeps the repairs a sweep applied - not the skipped
// ones - with when, and drops those older than a week.
func TestTheIncidentsListKeepsTheRepairsOfTheWeek(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	kept := []api.RepairDone{
		{Repair: api.Repair{Repair: api.RepairEndPlan, Work: "m.F-1", Applied: true}, At: now.Add(-8 * 24 * time.Hour).Format(time.RFC3339)},
		{Repair: api.Repair{Repair: api.RepairEndPlan, Work: "m.F-2", Applied: true}, At: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)},
	}
	sweep := []api.Repair{{Repair: api.RepairPutStep, Work: "m.F-3", Applied: true}, {Repair: api.RepairPutStep, Work: "m.F-4", Skipped: "dry-run"}}
	got := foldRepairs(kept, sweep, now)
	if len(got) != 2 || got[0].Work != "m.F-2" || got[1].Work != "m.F-3" || got[1].At != "2026-11-05T12:00:00Z" {
		t.Errorf("the repairs kept: %+v; want m.F-2 then m.F-3 at now", got)
	}
}

// Every incident cause the one reader names has its act - Créer la
// demande, Planifier or Casser le cycle - and its second way out
// (blocker-alt-<work>): Retirer l'attente or Sortir de l'incrément. The acts
// are chosen by the cause, with an empty reason; an
// incident with no cause is planned again or taken out.
func TestEveryIncidentReasonNamesItsActAndItsSecondAction(t *testing.T) {
	for _, c := range []struct{ reason, on, act, uri, alt, altURI string }{
		{api.CauseCycle, "m.TD-1", "Casser le cycle", "", "Sortir de l'incrément", incrementUntargetURI},
		{api.CauseNobodyWorks, "x.F-2", "Créer la demande sur x.F-2", requestFileURI, "Retirer l'attente", ""},
		{api.CauseNobodyWorks, "", "Planifier", deckPlanURI, "Sortir de l'incrément", incrementUntargetURI},
		{api.CauseNoWork, "", "Planifier", requestFileURI, "Sortir de l'incrément", incrementUntargetURI},
		{api.CauseUnreadable, "x.RQ-9", "Planifier", deckPlanURI, "Retirer l'attente", ""},
		{api.CauseWaitNotCleared, "x.RQ-9", "Planifier", deckPlanURI, "Retirer l'attente", ""},
		{api.CauseStepClosed, "", "Planifier", deckPlanURI, "Retirer l'attente", ""},
		{api.CauseUnclassified, "", "Planifier", deckPlanURI, "Retirer l'attente", ""},
		{api.CauseUnknownKind, "", "Planifier", deckPlanURI, "Retirer l'attente", ""},
		{"", "", "Planifier", deckPlanURI, "Sortir de l'incrément", incrementUntargetURI},
		{api.CauseTakenTwice, "", "Planifier", "", "Sortir de l'incrément", incrementUntargetURI},
	} {
		b := api.IncrementBlocker{Blocker: api.Blocker{Kind: api.BlockerIncident, Work: "m.RQ-5", On: c.on, Cause: c.reason, ActorKind: api.ActorPerson}}
		acts := blockerActs("fr", "m.IN-002", api.BlockerIncident, b)
		if len(acts) != 2 {
			t.Errorf("%q: %d actions; want the act and its second way out", c.reason, len(acts))
			continue
		}
		main, _ := json.Marshal(acts[0])
		alt, _ := json.Marshal(acts[1])
		if !strings.Contains(string(main), `"label":"`+c.act+`"`) || !strings.Contains(string(main), `"id":"blocker-open-m.RQ-5"`) || c.uri != "" && !strings.Contains(string(main), `"uri":"`+c.uri+`"`) {
			t.Errorf("%q: want the act %s (%s), got %s", c.reason, c.act, c.uri, main)
		}
		if !strings.Contains(string(alt), `"label":"`+c.alt+`"`) || !strings.Contains(string(alt), `"id":"blocker-alt-m.RQ-5"`) || c.altURI != "" && !strings.Contains(string(alt), `"uri":"`+c.altURI+`"`) {
			t.Errorf("%q: want the second action %s (%s), got %s", c.reason, c.alt, c.altURI, alt)
		}
	}
}

// The taker K3 names: a pull waiting now first, then a session busy on
// another to-do, then the latest pull ended within the lease
func TestTheLiveTakerNamedPrefersAWaitingPull(t *testing.T) {
	ended := schedapi.Taker{By: cwmapi.Actor{ID: "early"}, At: "2026-11-09T10:00:00Z"}
	later := schedapi.Taker{By: cwmapi.Actor{ID: "late"}, At: "2026-11-09T10:05:00Z"}
	busy := schedapi.Taker{By: cwmapi.Actor{ID: "busy"}, Holds: "m.TD-1"}
	waiting := schedapi.Taker{By: cwmapi.Actor{ID: "waiting"}, Waiting: true}
	for _, c := range []struct {
		takers []schedapi.Taker
		want   string
	}{
		{[]schedapi.Taker{ended, later}, "late"},
		{[]schedapi.Taker{later, busy}, "busy"},
		{[]schedapi.Taker{busy, ended, waiting}, "waiting"},
	} {
		if got, ok := livePull(c.takers); !ok || got.By.ID != c.want {
			t.Errorf("%+v: named %q; want %q", c.takers, got.By.ID, c.want)
		}
	}
	if _, ok := livePull(nil); ok {
		t.Error("no taker named one")
	}
	r := &holdReader{byID: map[string]api.WorkTodo{"m.TD-1": {Todo: schedapi.Todo{Request: "m.RQ-7"}}}}
	todo := api.WorkTodo{Todo: schedapi.Todo{Module: "m", Labels: []string{"increment:m.IN-002"}}}
	if got := r.takerWords(waiting, todo); got != "waiting (waiting on increment:m.IN-002)" {
		t.Errorf("waiting: %q", got)
	}
	if got := r.takerWords(busy, todo); got != "busy (working m.RQ-7)" {
		t.Errorf("busy: %q", got)
	}
}

// On a to-do Unwait accepts, « Retirer l'attente » removes its wait (plans
// unwait), asking first; on a to-do it refuses - the
// queue's unclassified wait on a ready to-do - or on the object alone it
// opens the work.
func TestRemoveTheWaitRunsUnwaitOnAToDo(t *testing.T) {
	queued := api.IncrementBlocker{Blocker: api.Blocker{Kind: api.BlockerIncident, Work: "m.RQ-5", Todo: "m.TD-7", Cause: api.CauseUnclassified, ActorKind: api.ActorSystem}}
	if alt, _ := json.Marshal(blockerActs("fr", "m.IN-002", api.BlockerIncident, queued)[1]); strings.Contains(string(alt), planUnwaitURI) {
		t.Errorf("a to-do Unwait refuses is offered « Retirer l'attente »: %s", alt)
	}
	for _, c := range []struct {
		todo api.WorkTodo
		want bool
	}{
		{api.WorkTodo{Todo: schedapi.Todo{State: schedapi.StateWaiting, Waits: "after m.TD-gone"}}, true},
		{api.WorkTodo{Todo: schedapi.Todo{State: schedapi.StateWaiting, Waits: "held by the review of m.TD-9"}}, false},
		{api.WorkTodo{Todo: schedapi.Todo{State: schedapi.StateReady}}, false},
	} {
		if got := waitRemovable(c.todo); got != c.want {
			t.Errorf("waitRemovable(%s %q) = %v; want %v", c.todo.State, c.todo.Waits, got, c.want)
		}
	}
	b := api.IncrementBlocker{Blocker: api.Blocker{Kind: api.BlockerIncident, Work: "m.RQ-5", Todo: "m.TD-7", On: "x.F-2", Cause: api.CauseNobodyWorks, ActorKind: api.ActorPerson}, Unwait: true}
	acts := blockerActs("fr", "m.IN-002", api.BlockerIncident, b)
	if len(acts) != 2 {
		t.Fatalf("%d actions; want the act and its second way out", len(acts))
	}
	alt, _ := json.Marshal(acts[1])
	for _, want := range []string{`"label":"Retirer l'attente"`, `"uri":"` + planUnwaitURI + `"`, `"todo":"m.TD-7"`, `"by":"${person}"`, `"confirm":"Retirer l'attente de m.RQ-5`} {
		if !strings.Contains(string(alt), want) {
			t.Errorf("the second action %s; want %s", alt, want)
		}
	}
}
