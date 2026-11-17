// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// THE CATCH-UP CARD. A catch-up that found more
// than cwmapi.CatchUpPerObjectMax objects moved outside the CWM verbs (a git
// pull, a bulk copy) publishes its grouped `project.caught-up` event held: no
// per-object event goes out, so no reaction runs unasked. The inbox files ONE
// pending card for it - « N objects changed by commit X » - naming the
// count, the commit and its author, the modules touched. Approving it asks
// CWM to publish the held per-object events (CatchUpRelease); dismissing or
// rejecting it keeps them held. A second catch-up of the same commit files no
// second card. CWM depends on no inbox: the inbox reacts to CWM's event.
//
// The card is an ordinary approval-request expecting a verdict; its asker
// (requested_by) names the catch-up: `cwm:catch-up/<event id>[@<sha>]`.

// catchUpAsker prefixes the requested_by of a catch-up's card.
const catchUpAsker = "cwm:catch-up/"

// catchUpRequestedBy is the asker of the card of the grouped event p.
func catchUpRequestedBy(p cwmapi.ObjectsCaughtUpEventPayload) string {
	s := catchUpAsker + p.EventID
	if p.Sha != "" {
		s += "@" + p.Sha
	}
	return s
}

// catchUpOf reads a card's asker: the held catch-up's event id and its
// commit; ok false for any other asker.
func catchUpOf(requestedBy string) (eventID, sha string, ok bool) {
	rest, found := strings.CutPrefix(requestedBy, catchUpAsker)
	if !found || rest == "" {
		return "", "", false
	}
	eventID, sha, _ = strings.Cut(rest, "@")
	return eventID, sha, eventID != ""
}

// OnCaughtUp is the inbox's reaction to CWM's events (a cwmapi.EventObserver
// handler): a held catch-up gets its card; any other event is nothing to it.
func OnCaughtUp(ctx context.Context, ev cwmapi.EventEnvelope) {
	if !strings.HasSuffix(ev.Topic, ".project.caught-up") {
		return
	}
	var p cwmapi.ObjectsCaughtUpEventPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		slog.Warn("inbox: a catch-up event cannot be read; no card filed", "topic", ev.Topic, "err", err)
		return
	}
	e, filed, err := New().FileCatchUp(ctx, p)
	switch {
	case err != nil:
		slog.Warn("inbox: the card of a held catch-up could not be filed", "event", p.EventID, "count", p.Count, "err", err)
	case filed:
		slog.Info("inbox: a held catch-up got its card", "event", p.EventID, "count", p.Count, "card", e.ID)
	}
}

// FileCatchUp files the card of a held catch-up: one pending verdict naming
// the count, the commit, its author and the modules touched. A catch-up not
// held (CatchUpPerObjectMax objects or fewer) gets none; a commit that already
// has a card - pending or answered - gets no second one (filed false, that
// card returned).
func (s *Service) FileCatchUp(ctx context.Context, p cwmapi.ObjectsCaughtUpEventPayload) (api.Entry, bool, error) {
	if !p.Held || p.Count <= cwmapi.CatchUpPerObjectMax || p.EventID == "" {
		return api.Entry{}, false, nil
	}
	if p.Sha != "" {
		if e, ok, err := s.catchUpCard(ctx, p.Sha); err != nil || ok {
			return e, false, err
		}
	}
	out, err := s.File(ctx, api.FileInput{
		Title:       catchUpTitle(p),
		Description: catchUpDescription(p),
		RequestedBy: catchUpRequestedBy(p),
		Expects:     api.ExpectsVerdict,
	})
	if err != nil {
		return api.Entry{}, false, err
	}
	return out.Request, true, nil
}

// catchUpCard is the card already filed for the commit sha, if any.
func (s *Service) catchUpCard(ctx context.Context, sha string) (api.Entry, bool, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.Entry{}, false, err
	}
	inbox, err := findInbox(ctx, cwm)
	if err != nil || inbox == "" {
		return api.Entry{}, false, err
	}
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: inbox, Type: api.RequestType})
	if err != nil {
		return api.Entry{}, false, err
	}
	for _, o := range kids.Objects {
		m, _ := cwmapi.ModuleOfObjectID(o.ID)
		full, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: o.ID})
		if err != nil {
			continue // gone meanwhile
		}
		o = full.Object // the body is not resident in a list
		if _, got, ok := catchUpOf(bodyOf(o).RequestedBy); ok && got == sha {
			return toEntry(inbox, o), true, nil
		}
	}
	return api.Entry{}, false, nil
}

// releaseCatchUp asks CWM to publish what the catch-up eventID held - the
// approval of its card. Nothing held any more (released already) is no
// error: the card is answered all the same.
func releaseCatchUp(ctx context.Context, eventID string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	out, err := cwm.CatchUpRelease(ctx, cwmapi.CatchUpReleaseInput{EventID: eventID})
	if errors.Is(err, cwmapi.ErrNotFound) {
		slog.Info("inbox: the catch-up holds nothing any more; its card is answered", "event", eventID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("inbox.Answer: release the events the catch-up %s held: %w", eventID, err)
	}
	slog.Info("inbox: a held catch-up was released", "event", eventID, "published", out.Published)
	return nil
}

// catchUpTitle: « N objects changed by commit X » (by its author).
func catchUpTitle(p cwmapi.ObjectsCaughtUpEventPayload) string {
	what := "outside the CWM verbs (not committed)"
	if p.Sha != "" {
		what = "by commit " + shortSha(p.Sha)
	}
	t := fmt.Sprintf("%d objects changed %s", p.Count, what)
	if p.By.ID != "" {
		t += " (" + p.By.ID + ")"
	}
	return t
}

// catchUpDescription says what moved, where and what the answer does.
func catchUpDescription(p cwmapi.ObjectsCaughtUpEventPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d objects changed outside the CWM verbs", p.Count)
	if p.Sha != "" {
		fmt.Fprintf(&b, ", by commit %s", p.Sha)
	}
	if p.By.ID != "" {
		fmt.Fprintf(&b, " of %s", p.By.ID)
	}
	fmt.Fprintf(&b, ": %d added, %d modified, %d removed.", len(p.Added), len(p.Modified), len(p.Removed))
	if mods := touchedModules(p); len(mods) > 0 {
		fmt.Fprintf(&b, "\n\nModules touched: %s.", strings.Join(mods, ", "))
	}
	fmt.Fprintf(&b, "\n\nTheir reactions are held. Approve to run them (CWM publishes one event per object); dismiss to keep them held.")
	return b.String()
}

// touchedModules are the modules of the moved ids, sorted.
func touchedModules(p cwmapi.ObjectsCaughtUpEventPayload) []string {
	seen := map[string]bool{}
	for _, ids := range [][]string{p.Added, p.Modified, p.Removed} {
		for _, id := range ids {
			if m, err := cwmapi.ModuleOfObjectID(id); err == nil && m != "" {
				seen[m] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func shortSha(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}
