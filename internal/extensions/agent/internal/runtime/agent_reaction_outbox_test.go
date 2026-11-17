// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN AGENT RUN A REACTION STARTED WRITES UNDER THE REACTION: the
// run's tools write with the dispatch context withSessionValues builds; on the
// real CWM, a write made with it is staged in the reaction's outbox — what it
// triggers waits for the reaction's success — never handed on at once.

var reactionPack = fstest.MapFS{
	"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")},
	"object-types/noted.yaml": {Data: []byte(`name: noted
hooks:
  on_create:
    - uri: x:react/noted
      async: {accepts_caused: true}
`)},
}

func TestAnAgentRunUnderAReactionStagesItsWritesInTheReactionsOutbox(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(reactionPack))
	cwmapp.Module(t, ctx, cwm, "m")

	reaction := cwmapi.ReactionID{
		Subscription: cwmapi.SubscriptionID("type:item", cwmapi.HookOnUpdate, "x:react/mark"),
		EventID:      "e-1",
	}.String()
	reactionCtx := cause.WithCause(ctx, cause.Cause{Kind: cause.KindReaction, ID: reaction, Depth: 1})
	runCtx := machineWithRunID().withSessionValues(reactionCtx)

	note := cwmapi.NewContainerObject("m", "note", "noted", "m", "Note", "")
	if _, err := cwm.ObjectCreate(runCtx, cwmapi.ObjectCreateInput{Module: "m", Object: note}); err != nil {
		t.Fatal(err)
	}

	boxes := outboxes(t, ctx, cwm)
	if len(boxes) != 1 || boxes[0].Reaction != reaction || boxes[0].Writes != 1 || boxes[0].Matches != 1 {
		t.Fatalf("outboxes = %+v, want the reaction %q holding the run's 1 write and its 1 match", boxes, reaction)
	}
}

func outboxes(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) []cwmapi.OutboxSummary {
	t.Helper()
	out, err := cwm.OutboxList(ctx, cwmapi.OutboxListInput{})
	if err != nil {
		t.Fatal(err)
	}
	return out.Outboxes
}
