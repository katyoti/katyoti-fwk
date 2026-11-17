// SPDX-License-Identifier: Apache-2.0

package core

import (
	"regexp"
	"strings"
)

// THE BLOCKERS' REASONS IN THE PAGE'S LANGUAGE: the reader of what
// holds a piece of work (blockers.go) and the reconciler say it in English,
// one line each; the Blockers tab says it in the person's language. Each
// line the reader writes has its rule here - the parts it carries (ids, a
// step's title, a note) kept as they are, a reason nested in another said
// again - and a line no rule knows is shown as written.

// sayRule is one line the reader writes, as a pattern, and its French: $1,
// $2… the parts kept; a part listed in again is said again first.
type sayRule struct {
	re    *regexp.Regexp
	fr    string
	again []int
}

func rule(pattern, fr string, again ...int) sayRule {
	return sayRule{re: regexp.MustCompile("^" + pattern + "$"), fr: fr, again: again}
}

// sayRules are in order: the first that matches says the line.
var sayRules = []sayRule{
	// What holds one piece of work (blockers.go).
	rule(`taken by (.+): an object has one open increment or release - aim it at one \(increments target\), the other gives it up`,
		"pris par $1 : un objet n'a qu'un incrément ou une version ouverts - visez-en un (increments target), l'autre y renonce"),
	rule(`its lifecycle is done while its plan is still (.+)`, "son cycle de vie est terminé alors que son plan est encore $1"),
	rule(`its plan is blocked while its step (.+) is offered to agents`, "son plan est bloqué alors que son étape $1 est offerte aux agents"),
	rule(`its question in (\S+) was answered \((.+)\) but the request did not resume`, "sa question dans $1 a reçu une réponse ($2) mais la demande n'a pas repris"),
	rule(`its question in (\S+) was answered \((.+)\) but its step did not resume`, "sa question dans $1 a reçu une réponse ($2) mais son étape n'a pas repris"),
	rule(`its question waits for an answer in (.+)`, "sa question attend une réponse dans $1"),
	rule(`nothing is planned: what next is someone's call \(plan its work\)(.*)`, "rien n'est planifié : la suite revient à quelqu'un (planifier son travail)$1"),
	rule(`its plan is blocked(.*)`, "son plan est bloqué$1"),
	rule(`its plan is (\S+) with no step to do`, "son plan est $1 sans étape à faire"),
	rule(`its plan is split: its part (\S+) carries its work`, "son plan est découpé : sa partie $1 porte son travail"),
	rule(`its plan is split: its follow-up (\S+) carries its work`, "son plan est découpé : sa suite $1 porte son travail"),
	rule(`its plan is split and none of its parts or follow-ups is open`, "son plan est découpé et aucune de ses parties ni de ses suites n'est ouverte"),
	rule(`its result was reported and never collected`, "son résultat a été rapporté et jamais collecté"),
	rule(`its result is being collected`, "son résultat est en cours de collecte"),
	rule(`a worker holds it`, "un agent le tient"),
	rule(`the person's answer: (.+)`, "votre réponse : $1"),
	rule(`the person's validate: (.+)`, "votre validation : $1"),
	rule(`the person's challenge: (.+)`, "votre contestation : $1"),
	rule(`the person's decide: (.+)`, "votre décision : $1"),
	rule(`the person's confirm: (.+)`, "votre confirmation : $1"),
	rule(`the person's (\S+): (.+)`, "votre acte ($1) : $2"),
	rule(`module (\S+) is off: no worker takes its to-dos`, "le module $1 est à l'arrêt : aucun agent ne prend ses tâches"),
	rule(`module (\S+) is being changed \(held by (\S+)\)`, "le module $1 est en cours de modification (tenu par $2)"),
	rule(`it is ready but the queue does not offer it`, "il est prêt mais la file ne le propose pas"),
	rule(`its claim by (\S+) lapsed at (\S+): (any agent may take it now.*)`, "la prise par $1 a expiré à $2 : $3", 3),
	rule(`any agent may take it now - no live taker since (\S+)`, "tout agent peut le prendre maintenant - aucune boucle ne le prend depuis $1"),
	rule(`any agent may take it now - no live taker`, "tout agent peut le prendre maintenant - aucune boucle ne le prend"),
	rule(`any agent may take it now`, "tout agent peut le prendre maintenant"),
	rule(`retried after (.+)`, "réessayé après $1"),
	rule(`an unclassified wait: (.*)`, "une attente non classée : $1"),
	rule(`a wait of an unknown kind (\S+): (.+)`, "une attente d'un genre inconnu $1 : $2"),
	rule(`a cycle: (.+)`, "un cycle : $1"),
	rule(`(.+) - at its root: (.+)`, "$1 - à sa racine : $2", 1, 2),
	rule(`(.+) - (\S+) cannot be read`, "$1 - $2 ne peut pas être lu", 1),
	rule(`(.+) - (\S+) has ended, the wait was not cleared`, "$1 - $2 est terminé, l'attente n'a pas été levée", 1),
	rule(`(.+) - nobody works on (\S+) \(no open plan\)`, "$1 - personne ne travaille sur $2 (aucun plan ouvert)", 1),
	rule(`(.+), which is no longer open`, "$1, qui n'est plus ouvert", 1),
	rule(`its work is done: it waits to be accepted`, "son travail est fait : il attend d'être accepté"),
	rule(`no work planned: nothing open works on it`, "aucun travail prévu : rien d'ouvert n'y travaille"),
	rule(`it waits on (\S+)(.*)`, "il attend $1$2"),
	rule(`waits on (\S+)(.*)`, "attend $1$2"),
	// The reconciler's incidents and repairs (reconcile.go).
	rule(`the reconciler repaired it (\d+) times and it came back: (.+)`, "le réconciliateur l'a réparé $1 fois et c'est revenu : $2", 2),
	rule(`end its plan done`, "terminer son plan"),
	rule(`plan its template's steps`, "planifier les étapes de son modèle"),
	rule(`end it as its last step's collect would have`, "le terminer comme l'aurait fait la collecte de sa dernière étape"),
	rule(`report the step with the answer`, "rapporter l'étape avec la réponse"),
	rule(`resume the request with the answer`, "reprendre la demande avec la réponse"),
	rule(`clear the wait and settle the step again`, "lever l'attente et régler l'étape à nouveau"),
	rule(`settle it again: its labels recomputed`, "le régler à nouveau : ses étiquettes recalculées"),
	// Who acts (the reader's actors).
	rule(`an agent: a loop on (.+)`, "un agent : une boucle sur $1"),
	rule(`(\S+) \(waiting on (\S+)\)`, "$1 (en attente sur $2)"),
	rule(`(\S+) \(working (\S+)\)`, "$1 (travaille sur $2)"),
	rule(`(\S+) \(pulled (\d+) min ago\)`, "$1 (a tiré il y a $2 min)"),
	rule(`(\S+) \(pulled lately\)`, "$1 (a tiré récemment)"),
	rule(`the person`, "vous"),
	rule(`plan its work`, "planifier son travail"),
	rule(`plan (\S+)`, "planifier $1"),
	rule(`accept (\S+)`, "accepter $1"),
	rule(`set module (\S+)'s mode`, "régler le mode du module $1"),
	rule(`break one of its waits`, "rompre une de ses attentes"),
	rule(`find what undoes the repair`, "trouver ce qui défait la réparation"),
	rule(`end its plan or reopen it`, "terminer son plan ou le rouvrir"),
	rule(`plan its next step or end it`, "planifier sa prochaine étape ou le terminer"),
	rule(`the reconciler: (.+)`, "le réconciliateur : $1", 1),
	rule(`the reconciler`, "le réconciliateur"),
	rule(`the scheduler's back-off`, "l'attente de l'ordonnanceur"),
	rule(`an outside party`, "un tiers"),
	// Where a root lies outside the increment.
	rule(`(\S+), in no increment or release`, "$1, dans aucun incrément ni version"),
	rule(`(\S+), in no release`, "$1, dans aucune version"),
	rule(`(\S+), in (\S+)`, "$1, dans $2"),
	rule(`in no release`, "dans aucune version"),
}

// blockerSay is one line of the reader in the page's language: French by its
// rules, English (and an unknown line) as written.
func blockerSay(lang, line string) string {
	if lang != "fr" || line == "" {
		return line
	}
	return sayFr(line, 0)
}

func sayFr(line string, depth int) string {
	if depth > 4 {
		return line
	}
	// A line of several parts joined by « · » says each part.
	if parts := strings.Split(line, " · "); len(parts) > 1 {
		for i, p := range parts {
			parts[i] = sayFr(p, depth+1)
		}
		return strings.Join(parts, " · ")
	}
	for _, r := range sayRules {
		m := r.re.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		groups := make([]string, len(m)/2)
		for g := range groups {
			if m[2*g] >= 0 {
				groups[g] = line[m[2*g]:m[2*g+1]]
			}
		}
		for _, g := range r.again {
			if g < len(groups) && groups[g] != "" {
				groups[g] = sayFr(groups[g], depth+1)
			}
		}
		out := r.fr
		for g := len(groups) - 1; g >= 1; g-- {
			out = strings.ReplaceAll(out, "$"+string(rune('0'+g)), groups[g])
		}
		return out
	}
	return line
}
