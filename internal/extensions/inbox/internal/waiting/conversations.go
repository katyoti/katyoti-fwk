// SPDX-License-Identifier: Apache-2.0

package waiting

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// CONVERSATIONS - « À faire » lists the chats running in the
// person's sessions: the display panels of type chat the universe's panel
// server keeps that have not ended, those waiting on the
// person's reply first - each with what it asks them -, then the last moved
// first. They are listed, never counted: a conversation is followed, not
// answered here; opening one opens its chat.

// panelList is the hub's panel server's list of the panels it keeps;
// panelSnapshot one panel as its events left it.
const (
	panelList     = "ktw-hub:panel-service/list"
	panelSnapshot = "ktw-hub:panel-service/snapshot"
)

// panelSummary is what the list says of one panel.
type panelSummary struct {
	Panel   string    `json:"panel"`
	State   string    `json:"state,omitempty"`
	Type    string    `json:"type,omitempty"`
	Title   string    `json:"title,omitempty"`
	Owner   string    `json:"owner,omitempty"`
	Waiting bool      `json:"waiting,omitempty"`
	Updated time.Time `json:"updated,omitzero"`
}

// panels reads the panels the universe keeps (a test sets it).
var panels = func(ctx context.Context) ([]panelSummary, error) {
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return nil, err
	}
	out, err := actions.Run(ctx, actionsapi.RunInput{URI: panelList, Input: json.RawMessage(`{}`)})
	if err != nil {
		return nil, err
	}
	var got struct {
		Panels []panelSummary `json:"panels"`
	}
	if err := json.Unmarshal(out.Value, &got); err != nil {
		return nil, err
	}
	return got.Panels, nil
}

// asked reads what a panel asks the person: its last ask's question, "" with
// none (a test sets it).
var asked = func(ctx context.Context, id string) (string, error) {
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return "", err
	}
	in, _ := json.Marshal(map[string]string{"panel": id})
	out, err := actions.Run(ctx, actionsapi.RunInput{URI: panelSnapshot, Input: in})
	if err != nil {
		return "", err
	}
	var snap struct {
		Events []panel.Event `json:"events"`
	}
	if err := json.Unmarshal(out.Value, &snap); err != nil {
		return "", err
	}
	for i := len(snap.Events) - 1; i >= 0; i-- {
		if a := snap.Events[i].Ask; a != nil && strings.TrimSpace(a.Question) != "" {
			return a.Question, nil
		}
	}
	return "", nil
}

// lastWord is a question as a card quotes it: its first line, cut at
// lastWordMax characters.
func lastWord(q string) string {
	q, _, _ = strings.Cut(strings.TrimSpace(q), "\n")
	if r := []rune(q); len(r) > lastWordMax {
		q = strings.TrimSpace(string(r[:lastWordMax-1])) + "…"
	}
	return q
}

// lastWordMax bounds the question a conversation's card quotes.
const lastWordMax = 120

// conversationsOf are the chats not ended, waiting on the person first, then
// the last moved first - a chat waiting on them with what it asks; none when
// the panels cannot be read (logged).
func conversationsOf(ctx context.Context) []api.Conversation {
	ps, err := panels(ctx)
	if err != nil {
		slog.Debug("inbox: the panels cannot be read; « À faire » lists no conversation", "err", err)
		return nil
	}
	var out []api.Conversation
	for _, p := range ps {
		if p.Type != api.ChatPanelType || p.State == "complete" || p.State == "closed" {
			continue
		}
		c := api.Conversation{Panel: p.Panel, Title: p.Title, Owner: p.Owner, Waiting: p.Waiting || p.State == "asking"}
		if !p.Updated.IsZero() {
			c.Updated = p.Updated.UTC().Format(time.RFC3339)
		}
		if c.Waiting {
			if q, err := asked(ctx, p.Panel); err != nil {
				slog.Debug("inbox: what a chat asks cannot be read; its card shows when it last moved", "panel", p.Panel, "err", err)
			} else {
				c.Last = lastWord(q)
			}
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Waiting != out[j].Waiting {
			return out[i].Waiting
		}
		return out[i].Updated > out[j].Updated
	})
	return out
}
