// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A QUEUE WAIT HAS ONE KIND AND ONE HOLDER: the one
// mapping from the scheduler's structured wait of a ready to-do to
// planification's kind and actor answers, per cause, the kind (K1-K6) and who
// moves it - what planning check and the pull's waiting line both say.
func TestAQueueWaitHasOneKindAndHolder(t *testing.T) {
	worker := cwmapi.Actor{ID: "session-b", Kind: "agent"}
	agent := func(step string) api.WorkTodo {
		return api.WorkTodo{Todo: schedapi.Todo{ID: "billing.TD-2", For: schedapi.ForAgent, Title: step + " — print"}, Packet: api.WorkPacket{Step: step}}
	}
	person := func(interaction string) api.WorkTodo {
		w := agent("give")
		w.For, w.Packet.Interaction = schedapi.ForPerson, interaction
		return w
	}
	rows := []struct {
		name      string
		todo      api.WorkTodo
		wait      schedapi.QueueWait
		kind      string
		actor     string
		actorKind string
		since     string
	}{
		{"lock", agent("intake"), schedapi.QueueWait{Cause: schedapi.WaitLock, Module: "billing", HolderTodo: "billing.TD-1", Holder: &worker, HolderRequest: "billing.RQ-1"},
			api.BlockerHeld, "session-b (billing.RQ-1)", api.ActorAgent, ""},
		{"reserved", agent("intake"), schedapi.QueueWait{Cause: schedapi.WaitReserved, Module: "billing", HolderTodo: "billing.TD-1", HolderRequest: "billing.RQ-1"},
			api.BlockerHeld, "billing.TD-1 (billing.RQ-1), ready ahead in the queue", api.ActorAgent, ""},
		{"backoff", agent("intake"), schedapi.QueueWait{Cause: schedapi.WaitBackoff, Until: "2026-11-06T15:00:00Z"},
			api.BlockerHeld, "the scheduler's back-off", api.ActorSystem, "2026-11-06T15:00:00Z"},
		{"assigned", agent("intake"), schedapi.QueueWait{Cause: schedapi.WaitAssigned, Holder: &worker},
			api.BlockerReady, "session-b", api.ActorAgent, ""},
		{"members", agent("intake"), schedapi.QueueWait{Cause: schedapi.WaitMembers, Group: "reviewers"},
			api.BlockerReady, "a member of reviewers", api.ActorAgent, ""},
		{"person provide", person(api.InteractionProvide), schedapi.QueueWait{Cause: schedapi.WaitPerson},
			api.BlockerAnswer, "the person", api.ActorPerson, ""},
		{"person no interaction", person(""), schedapi.QueueWait{Cause: schedapi.WaitPerson},
			api.BlockerAnswer, "the person", api.ActorPerson, ""},
		{"person validate", person(api.InteractionValidate), schedapi.QueueWait{Cause: schedapi.WaitPerson},
			api.BlockerAct, "the person", api.ActorPerson, ""},
		{"person decide, given to another", person("decide"), schedapi.QueueWait{Cause: schedapi.WaitAssigned, Holder: &worker},
			api.BlockerAct, "the person", api.ActorPerson, ""},
		{"unknown cause", agent("intake"), schedapi.QueueWait{Cause: "weather"},
			api.BlockerIncident, "the reconciler", api.ActorSystem, ""},
	}
	for _, r := range rows {
		got := queueWaitWords(r.todo, r.wait)
		if got.Kind != r.kind || got.Actor != r.actor || got.ActorKind != r.actorKind || got.Since != r.since {
			t.Errorf("%s: %s by %q (%s) since %q; want %s by %q (%s) since %q",
				r.name, got.Kind, got.Actor, got.ActorKind, got.Since, r.kind, r.actor, r.actorKind, r.since)
		}
		if got.Reason == "" {
			t.Errorf("%s: no reason", r.name)
		}
		// The pull's waiting line says the same kind and holder.
		raw := r.todo.Todo
		encode(&raw, r.todo.Packet)
		if label := NameWait(context.Background(), raw, r.wait); label != got.label() || !strings.HasPrefix(label, api.BlockerClass(r.kind)+" "+r.kind+" by ") {
			t.Errorf("%s: the pull's label %q; want %q", r.name, label, got.label())
		}
	}
}
