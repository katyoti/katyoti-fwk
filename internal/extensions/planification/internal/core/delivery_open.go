// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY VIEW'S TWO TABS (the approved mock-up
// modules/research-delivery-overview/delivery-planning-mockup.html):
// « Planned delivery » - the timeline, the chosen release's increments and
// the short panel of the increment picked (PanelPage: its header, its
// actions, its requests bar, one bar per delivery goal, Open the increment);
// « Open increments » - every open increment in no release on a grid board
// of its stream's lane by priority P0-P3 and On hold, filtered by stream and
// by text. A card moves to another priority (BoardMove), to On hold saying
// why, back to a priority (resumed there), ships into a release of its
// stream (BoardShip) or gets its stream (SetStream); each answer carries the
// action that takes it back (« Undo »).

// The columns of the Open increments board.
const (
	heldColumn   = "held"
	noStreamRow  = "none"
	panelPageURI = "ktw-agents:increments/panel-page"
	boardMoveURI = "ktw-agents:increments/board-move"
	boardShipURI = "ktw-agents:increments/board-ship"
	setStreamURI = "ktw-agents:increments/set-stream"
)

var boardPriorities = []string{"P0", "P1", "P2", "P3"}

// boardWords are the answers of BoardMove, BoardShip and SetStream - their
// toast and its Undo - per language (boardWord).
var boardWords = map[string]map[string]string{
	"en": {
		"undo": "Undo", "other_stream": "%s stays in its stream: change its stream from its card or its page.",
		"already_held": "%s is already on hold.", "say_why": "Say why it is on hold: the reason shows wherever it appears.",
		"on_hold": "%s is on hold: %s", "not_priority": "%q is not a priority (P0 to P3) nor On hold.",
		"resumed": "%s resumed at %s", "moved": "%s: %s → %s",
		"no_release": "%s ships in no release.", "taken_out": "%s taken out of %s: it is back in Open increments",
		"already_ships": "%s already ships there.", "ships_in": "%s ships in %s: see Planned delivery",
		"in_stream": "%s is now in the %s stream",
	},
	"fr": {
		"undo": "Annuler", "other_stream": "%s reste dans son flux : changez son flux depuis sa carte ou sa page.",
		"already_held": "%s est déjà suspendu.", "say_why": "Dites pourquoi il est suspendu : la raison s'affiche partout où il apparaît.",
		"on_hold": "%s est suspendu : %s", "not_priority": "%q n'est ni une priorité (P0 à P3) ni suspendu.",
		"resumed": "%s repris en %s", "moved": "%s : %s → %s",
		"no_release": "%s n'est livré dans aucune version.", "taken_out": "%s retiré de %s : il est de nouveau dans Incréments ouverts",
		"already_ships": "%s y est déjà livré.", "ships_in": "%s livré dans %s : voir Livraison planifiée",
		"in_stream": "%s est maintenant dans le flux %s",
	},
}

// boardWord is an answer's word in lang.
func boardWord(lang, key string) string {
	if l, ok := boardWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return boardWords["en"][key]
}

// boardf is an answer's message in lang.
func boardf(lang, key string, a ...any) string {
	return fmt.Sprintf(boardWord(lang, key), a...)
}

// withLanguage is args carrying the person's language: the action answers in
// it.
func withLanguage(args map[string]any, lang string) map[string]any {
	if lang != "" {
		args["language"] = lang
	}
	return args
}

// boardUndo is an answer's Undo: the action that takes it back, in lang.
func boardUndo(lang, uri string, args map[string]any) render.Action {
	return render.RunURI(boardWord(lang, "undo"), uri, withLanguage(args, lang))
}

type plannedRelease struct{ id, label, date string }

// plannedReleases are a stream's releases still planned, by date.
func plannedReleases(ctx context.Context, cwm cwmapi.CWMService, stream string) []plannedRelease {
	rels, err := inStream(ctx, cwm, stream, api.ReleaseType)
	if err != nil {
		return nil
	}
	var out []plannedRelease
	for _, r := range rels {
		if releaseState(r) != api.ReleasePlanned {
			continue
		}
		out = append(out, plannedRelease{id: r.ID, label: releaseLabelOf(ctx, cwm, r.ID), date: releaseFields(r).Date})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].date == "") != (out[j].date == "") {
			return out[i].date != ""
		}
		return out[i].date < out[j].date
	})
	return out
}

// PanelPage is the short panel of the increment picked in Planned delivery:
// its open requests (openRequestsPanel) - its proposal
// approvals and requests stay on its own page.
func (i *Increments) PanelPage(ctx context.Context, in api.IncrementPanelInput) (render.Def, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	inc := toIncrement(ctx, o)
	p, err := i.paneData(withReadCache(ctx), cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	p.lang = in.Language
	return render.Def{Kind: "page", Title: inc.Title, DSLTree: openRequestsPanel(ctx, cwm, p)}, nil
}

// BoardMove moves an increment on the board: to another priority (resumed
// there when on hold), or to On hold saying why. A row named must be its own
// stream's: a card never changes stream by a drop.
func (i *Increments) BoardMove(ctx context.Context, in api.BoardMoveInput) (render.Result, error) {
	if err := checkActor(in.By); err != nil {
		return render.Result{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Result{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return render.Result{}, err
	}
	inc := toIncrement(ctx, o)
	if in.Row != "" {
		own := inc.Stream
		if own == "" {
			own = noStreamRow
		}
		if in.Row != own {
			return render.Result{OK: false, Message: boardf(in.Language, "other_stream", inc.Title)}, nil
		}
	}
	old := inc.Priority
	if in.Column == heldColumn {
		if inc.Held != nil {
			return render.Result{OK: true, Message: boardf(in.Language, "already_held", inc.Title)}, nil
		}
		if strings.TrimSpace(in.Reason) == "" {
			return render.Result{OK: false, Message: boardWord(in.Language, "say_why")}, nil
		}
		if _, err := i.Hold(ctx, api.IncrementHoldInput{ID: in.ID, Reason: in.Reason, By: in.By}); err != nil {
			return render.Result{}, err
		}
		undo := boardUndo(in.Language, boardMoveURI, map[string]any{"id": in.ID, "column": old, "by": actorArg(in.By)})
		return render.Result{OK: true, Message: boardf(in.Language, "on_hold", inc.Title, in.Reason), Undo: &undo}, nil
	}
	if !isPriority(in.Column) {
		return render.Result{OK: false, Message: boardf(in.Language, "not_priority", in.Column)}, nil
	}
	held := inc.Held
	if held != nil {
		if _, err := i.Resume(ctx, api.IncrementHoldInput{ID: in.ID, By: in.By}); err != nil {
			return render.Result{}, err
		}
	}
	if in.Column != old {
		if _, err := New().Assess(ctx, api.AssessInput{ID: in.ID, Priority: in.Column, By: in.By}); err != nil {
			return render.Result{}, err
		}
	}
	if held != nil {
		undo := boardUndo(in.Language, boardMoveURI, map[string]any{"id": in.ID, "column": heldColumn, "reason": held.Reason, "by": actorArg(in.By)})
		return render.Result{OK: true, Message: boardf(in.Language, "resumed", inc.Title, in.Column), Undo: &undo}, nil
	}
	undo := boardUndo(in.Language, boardMoveURI, map[string]any{"id": in.ID, "column": old, "by": actorArg(in.By)})
	return render.Result{OK: true, Message: boardf(in.Language, "moved", inc.Title, old, in.Column), Undo: &undo}, nil
}

// BoardShip ships an increment into a release of its stream - the next ships
// it in, a later one plans it - or, with « out », takes it out of its release.
func (i *Increments) BoardShip(ctx context.Context, in api.BoardShipInput) (render.Result, error) {
	if err := checkActor(in.By); err != nil {
		return render.Result{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Result{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return render.Result{}, err
	}
	inc := toIncrement(ctx, o)
	// The card of an increment in no stream offers the streams: the choice
	// puts it in one.
	if isStreamID(in.In) {
		return i.SetStream(ctx, api.IncrementStreamInput{ID: in.ID, Stream: in.In, By: in.By, Language: in.Language})
	}
	was := inc.ShipsIn + inc.PlannedFor
	back := "out"
	if was != "" {
		back = was
	}
	undo := boardUndo(in.Language, boardShipURI, map[string]any{"id": in.ID, "in": back, "by": actorArg(in.By)})
	if in.In == "" || in.In == "out" {
		switch {
		case inc.ShipsIn != "":
			if _, err := i.Unship(ctx, api.GetInput{ID: in.ID}); err != nil {
				return render.Result{}, err
			}
		case inc.PlannedFor != "":
			if _, err := i.Unplan(ctx, api.GetInput{ID: in.ID}); err != nil {
				return render.Result{}, err
			}
		default:
			return render.Result{OK: true, Message: boardf(in.Language, "no_release", inc.Title)}, nil
		}
		return render.Result{OK: true, Message: boardf(in.Language, "taken_out", inc.Title, releaseLabelOf(ctx, cwm, was)), Undo: &undo}, nil
	}
	if in.In == was {
		return render.Result{OK: true, Message: boardf(in.Language, "already_ships", inc.Title)}, nil
	}
	if _, err := i.PlanFor(ctx, api.PlanForInput{ID: in.ID, In: in.In, By: in.By}); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	return render.Result{OK: true, Message: boardf(in.Language, "ships_in", inc.Title, releaseLabelOf(ctx, cwm, in.In)), Undo: &undo}, nil
}

// SetStream puts an increment in a stream (its in-stream edge).
func (i *Increments) SetStream(ctx context.Context, in api.IncrementStreamInput) (render.Result, error) {
	if err := checkActor(in.By); err != nil {
		return render.Result{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Result{}, err
	}
	o, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return render.Result{}, err
	}
	inc := toIncrement(ctx, o)
	if _, err := NewStreams().AimIncrement(ctx, api.StreamInInput{ID: in.ID, Stream: in.Stream}); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	res := render.Result{OK: true, Message: boardf(in.Language, "in_stream", inc.Title, titleOfID(ctx, cwm, in.Stream))}
	if inc.Stream != "" {
		undo := boardUndo(in.Language, setStreamURI, map[string]any{"id": in.ID, "stream": inc.Stream, "by": actorArg(in.By)})
		res.Undo = &undo
	}
	return res, nil
}

func isPriority(p string) bool {
	for _, x := range boardPriorities {
		if x == p {
			return true
		}
	}
	return false
}

// actorArg is an actor as an undo's written-out argument.
func actorArg(a cwmapi.Actor) map[string]any {
	return map[string]any{"id": a.ID, "kind": a.Kind}
}

// plannedCount is how many open increments ship in, or are planned for, a
// release still to ship: the Planned delivery tab's count.
func plannedCount(ctx context.Context) int {
	is, err := NewIncrements().List(ctx, api.ListIncrementsInput{State: api.IncrementOpen})
	if err != nil {
		return 0
	}
	n := 0
	for _, i := range is.Increments {
		if i.ShipsIn != "" || i.PlannedFor != "" {
			n++
		}
	}
	return n
}
