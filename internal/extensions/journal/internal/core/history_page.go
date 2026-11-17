// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// THE HISTORY PANEL — History drawn on every object's pane: each record,
// newest first, as a timeline entry - what happened, when, who did it and why.

// historyTexts are the panel's words, in the languages the GUI speaks; a key
// missing in a language reads the English.
var historyTexts = map[string]map[string]string{
	"en": {
		"title": "History", "empty": "Nothing has been recorded for this object yet.",
		"created": "Created", "updated": "Changed", "deleted": "Deleted", "moved": "Moved from %s",
		"split": "Split into %s", "archived": "Archived", "transitioned": "%s → %s", "invoked": "Ran %s (%s)",
		"event": "Event %s", "by": "by %s", "cause": "because of %s %s", "truncated": "Only the latest %d are shown.",
	},
	"fr": {
		"title": "Historique", "empty": "Rien n'a encore été enregistré pour cet objet.",
		"created": "Créé", "updated": "Modifié", "deleted": "Supprimé", "moved": "Déplacé depuis %s",
		"split": "Scindé en %s", "archived": "Archivé", "transitioned": "%s → %s", "invoked": "%s lancé (%s)",
		"event": "Événement %s", "by": "par %s", "cause": "à cause de %s %s", "truncated": "Seuls les %d derniers sont affichés.",
	},
}

func historyText(lang, key string) string {
	if s, ok := historyTexts[lang][key]; ok {
		return s
	}
	return historyTexts["en"][key]
}

// historyIcons are each record's icon and color on the timeline.
var historyIcons = map[string][2]string{
	"created": {"circle-plus", "green"}, "updated": {"pencil", "blue"}, "deleted": {"trash", "red"},
	"moved": {"arrows-move", "violet"}, "split": {"arrows-split", "violet"}, "archived": {"archive", "gray"},
	"transitioned": {"route", "teal"}, "invoked": {"bolt", "orange"}, "event": {"bell", "yellow"},
}

// HistoryPage is History drawn: the History panel of every object.
func (s *Service) HistoryPage(ctx context.Context, in api.HistoryPageInput) (render.Def, error) {
	if in.ID == "" {
		return render.Def{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	got, err := s.History(ctx, api.HistoryInput{ID: in.ID})
	if err != nil {
		return render.Def{}, err
	}
	lang := in.Language
	title := historyText(lang, "title")
	if len(got.Records) == 0 {
		return render.Def{Kind: "page", Title: title, DSLTree: render.Dimmed(historyText(lang, "empty"))}, nil
	}
	items := make([]map[string]any, 0, len(got.Records))
	for _, r := range got.Records {
		items = append(items, historyItem(lang, r))
	}
	kids := []render.Node{{"kind": "timeline", "items": items, "bullet": "icon"}}
	if got.Truncated {
		kids = append(kids, render.Dimmed(fmt.Sprintf(historyText(lang, "truncated"), len(got.Records))))
	}
	return render.Def{Kind: "page", Title: title, DSLTree: render.VBox(kids...)}, nil
}

// historyItem is one record as a timeline entry: what happened as its title,
// who and why as its body, the note as its details.
func historyItem(lang string, r api.Record) map[string]any {
	var what string
	switch r.Action {
	case "moved":
		what = fmt.Sprintf(historyText(lang, "moved"), firstNonEmpty(r.FromID, r.FromParent))
	case "split":
		what = fmt.Sprintf(historyText(lang, "split"), strings.Join(r.Successors, ", "))
	case "transitioned":
		what = fmt.Sprintf(historyText(lang, "transitioned"), firstNonEmpty(r.FromState, "-"), r.ToState)
	case "invoked":
		what = fmt.Sprintf(historyText(lang, "invoked"), r.URI, r.Status)
	case "event":
		what = fmt.Sprintf(historyText(lang, "event"), r.Event)
	default:
		what = historyText(lang, r.Action)
		if what == "" {
			what = r.Action
		}
	}
	if r.ID != "" && r.Action != "moved" {
		what += " · " + r.ID
	}
	var body []string
	if r.By != nil && r.By.ID != "" {
		body = append(body, fmt.Sprintf(historyText(lang, "by"), r.By.ID))
	}
	if r.Cause != nil && r.Cause.ID != "" {
		body = append(body, fmt.Sprintf(historyText(lang, "cause"), r.Cause.Kind, r.Cause.ID))
	}
	if r.Reason != "" {
		body = append(body, r.Reason)
	}
	it := map[string]any{"date": r.At.Format("2006-01-02 15:04"), "title": what, "body": strings.Join(body, " · ")}
	if r.Note != "" {
		it["details"] = r.Note
	}
	if ic, ok := historyIcons[r.Action]; ok {
		it["icon"], it["color"] = ic[0], ic[1]
	}
	return it
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
