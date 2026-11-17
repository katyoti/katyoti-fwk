// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// Hold keeps a connector's chosen items for a carrier without reading them
// : the held choice of its own source for the connector
// created when it has none. A sync reads the selection, never them.
func (s *Service) Hold(ctx context.Context, in api.HoldInput) (api.HoldOutput, error) {
	if in.ID == "" {
		in.ID = in.Object
	}
	for _, it := range in.Items {
		if strings.TrimSpace(it.Key) == "" {
			return api.HoldOutput{}, fmt.Errorf("%w: a held item has no key", cwmapi.ErrInvalidInput)
		}
	}
	used, err := s.Use(ctx, api.UseInput{ID: in.ID, Connector: in.Connector})
	if err != nil {
		return api.HoldOutput{}, err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.HoldOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, module, err := sourceFields(ctx, cwm, used.Source)
	if err != nil {
		return api.HoldOutput{}, err
	}
	f.Held = keepers(in.Items, append(api.SelectedItems(f.Selection), f.Held...), in.By, time.Now().UTC().Format(time.RFC3339))
	s.seeFrom(ctx, cwm, used.Source, &f)
	if err := s.recordSource(ctx, cwm, module, used.Source, f); err != nil {
		return api.HoldOutput{}, err
	}
	out := api.HoldOutput{OK: true, Source: used.Source, Held: len(f.Held)}
	if !in.Read {
		out.Message = fmt.Sprintf("%d élément(s) retenu(s), pas encore lus : ils le seront quand vous appliquerez.", len(f.Held))
		return out, nil
	}
	// « Enregistrer et lire »: what is kept is read at
	// once and lands in the knowledge base.
	if len(f.Held) == 0 {
		out.Message = "Rien de coché : rien à lire."
		return out, nil
	}
	res, err := s.applyOne(ctx, cwm, used.Source)
	if err != nil {
		return out, err
	}
	out.Result = &res
	out.Message = readMessage(res) + s.reindexNote(ctx, in.ID)
	return out, nil
}

// seeFrom records, on a save of a source whose connector reads from another
// source, the values that other source keeps now: its take
// tab marks a value kept after it as new. Nothing kept: the empty list.
func (s *Service) seeFrom(ctx context.Context, cwm cwmapi.CWMService, source string, f *api.InformationSourceFields) {
	c, err := s.connector(ctx, f.Connector)
	if err != nil || c.From == nil {
		return
	}
	from, err := fromItems(ctx, cwm, carrierOfSource(ctx, cwm, source), c)
	if err != nil {
		return
	}
	f.FromSeen = []string{}
	if from != nil {
		f.FromSeen = append(f.FromSeen, from.Values...)
	}
}

// readMessage says what reading a kept choice read and, for what it could
// not, why and what reads it later.
func readMessage(res api.ApplyResult) string {
	msg := fmt.Sprintf("%d élément(s) lu(s).", len(res.Read))
	if len(res.Failed) > 0 {
		why := res.Why
		if why == "" {
			why = "la synchronisation ne les a pas apportés"
		}
		msg += fmt.Sprintf(" %d non lu(s) : %s. Ils restent retenus : la prochaine synchronisation de la source les lit.", len(res.Failed), strings.TrimRight(why, ". "))
	}
	return msg
}

// reindexNote reindexes the carrier's knowledge base once its sources moved:
// "" when done, else what stops it.
func (s *Service) reindexNote(ctx context.Context, carrier string) string {
	// What the sources now bring is the knowledge base: the documents no
	// longer brought leave it.
	if r, err := s.Reindex(ctx, api.ReindexInput{ID: carrier}); err != nil {
		return " Base non réindexée : " + err.Error()
	} else if !r.OK {
		return " " + r.Message
	}
	return ""
}

// keepers are the items held, each with who kept it and when: an item
// already kept (picked or held before) keeps its keeper; a new one is kept by
// by, now.
func keepers(items, before []api.SelectedItem, by *api.Keeper, now string) []api.SelectedItem {
	known := map[string]api.SelectedItem{}
	for _, it := range before {
		if it.KeptBy != nil || it.KeptAt != "" {
			known[it.Key] = it
		}
	}
	out := make([]api.SelectedItem, 0, len(items))
	for _, it := range items {
		if k, ok := known[it.Key]; ok {
			it.KeptBy, it.KeptAt = k.KeptBy, k.KeptAt
		} else if by != nil && by.ID != "" {
			it.KeptBy, it.KeptAt = by, now
		}
		out = append(out, it)
	}
	return out
}

// keptInto writes who kept an item and when into its selection entry
func keptInto(m map[string]any, it api.SelectedItem) {
	if it.KeptBy != nil {
		m["kept_by"] = map[string]any{"id": it.KeptBy.ID, "kind": it.KeptBy.Kind}
	}
	if it.KeptAt != "" {
		m["kept_at"] = it.KeptAt
	}
}

// Apply gives each of the carrier's own sources holding a choice that choice
// as its selection, its held choice emptied, then syncs it:
// what it adds is read, what it drops leaves the knowledge base, and each
// item records whether it is read, removed or failed and why; the knowledge
// base is reindexed once.
func (s *Service) Apply(ctx context.Context, in api.ApplyInput) (api.ApplyOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ApplyOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.ID); err != nil {
		return api.ApplyOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.ApplyOutput{}, err
	}
	out := api.ApplyOutput{OK: true, Sources: []string{}}
	for _, src := range srcs {
		if src.Via != "child" || src.Held == 0 {
			continue
		}
		res, err := s.applyOne(ctx, cwm, src.ID)
		if err != nil {
			return out, err
		}
		out.Sources = append(out.Sources, src.ID)
		out.Results = append(out.Results, res)
	}
	reindexed := ""
	if len(out.Sources) > 0 {
		reindexed = s.reindexNote(ctx, in.ID)
	}
	out.Message = fmt.Sprintf("%d source(s) appliquée(s).%s", len(out.Sources), reindexed)
	return out, nil
}

// applyOne gives a source its held choice as its selection, its held choice
// emptied, then syncs it (F-015).
func (s *Service) applyOne(ctx context.Context, cwm cwmapi.CWMService, id string) (api.ApplyResult, error) {
	f, module, err := sourceFields(ctx, cwm, id)
	if err != nil {
		return api.ApplyResult{Source: id}, err
	}
	before := map[string]bool{}
	for _, it := range api.SelectedItems(f.Selection) {
		before[it.Key] = true
	}
	now := map[string]bool{}
	f.States = map[string]api.ItemState{}
	items := make([]any, 0, len(f.Held))
	for _, it := range f.Held {
		now[it.Key] = true
		f.States[it.Key] = api.ItemState{State: api.ItemWaiting, Why: "lecture en cours"}
		m := map[string]any{"key": it.Key}
		if it.Title != "" {
			m["title"] = it.Title
		}
		if it.WithChildren {
			m["with_children"] = true
		}
		if it.Parent != "" {
			m["parent"] = it.Parent
		}
		keptInto(m, it)
		items = append(items, m)
	}
	if f.Selection == nil {
		f.Selection = map[string]any{}
	}
	f.Selection[api.SelectionItems] = items
	f.Held = nil
	var dropped []string
	for k := range before {
		if !now[k] {
			dropped = append(dropped, k)
			f.States[k] = api.ItemState{State: api.ItemWaiting, Why: "retrait en cours"}
		}
	}
	sort.Strings(dropped)
	if err := s.recordSource(ctx, cwm, module, id, f); err != nil {
		return api.ApplyResult{Source: id}, err
	}
	return s.applied(ctx, cwm, id, now, dropped)
}

// applied syncs a source whose selection was just applied and records where
// each item stands: read (brought by the sync), failed (not brought, why);
// an item dropped from the choice leaves the states once a whole sync removed
// it, else waits for one.
func (s *Service) applied(ctx context.Context, cwm cwmapi.CWMService, id string, now map[string]bool, dropped []string) (api.ApplyResult, error) {
	res := api.ApplyResult{Source: id}
	sync, err := s.SourceSync(ctx, api.SourceSyncInput{ID: id})
	if err != nil {
		return res, err
	}
	f, module, err := sourceFields(ctx, cwm, id)
	if err != nil {
		return res, err
	}
	brought := map[string]bool{}
	for _, k := range sync.Keys {
		brought[k] = true
	}
	// A connector that says no key: a sync read whole brought every item.
	whole := sync.OK && len(sync.Keys) == 0 && len(sync.Errors) == 0
	why := strings.Join(sync.Errors, " ; ")
	if why == "" && !sync.OK {
		why = sync.Message
	}
	keys := make([]string, 0, len(now))
	for k := range now {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	f.States = map[string]api.ItemState{}
	for _, k := range keys {
		switch {
		case brought[k] || whole:
			f.States[k] = api.ItemState{State: api.ItemRead}
			res.Read = append(res.Read, k)
		default:
			w := why
			if w == "" {
				w = "la synchronisation ne l'a pas apporté"
			}
			f.States[k] = api.ItemState{State: api.ItemFailed, Why: w}
			res.Failed = append(res.Failed, k)
			res.Why = w
		}
	}
	for _, k := range dropped {
		if sync.OK && len(sync.Errors) == 0 {
			// Its removal is done: the item left the source and keeps no
			// state, so the detail shows no row for it.
			res.Removed = append(res.Removed, k)
		} else {
			// A sync with errors keeps what it brought before: the item
			// stays until a whole sync removes it.
			f.States[k] = api.ItemState{State: api.ItemWaiting, Why: "retiré à la prochaine synchronisation complète"}
		}
	}
	if err := s.recordSource(ctx, cwm, module, id, f); err != nil {
		return res, err
	}
	res.Message = sync.Message
	return res, nil
}

// UseSet makes a source-set value the carrier's sources: each connector's
// items held on its one source, then applied - read, in the background unless
// asked to wait.
func (s *Service) UseSet(ctx context.Context, in api.UseSetInput) (api.UseSetOutput, error) {
	byConnector := map[string][]api.SelectedItem{}
	var order []string
	for _, it := range in.Items {
		if strings.TrimSpace(it.Connector) == "" || strings.TrimSpace(it.Key) == "" {
			return api.UseSetOutput{}, fmt.Errorf("%w: a source-set item names its connector and its key", cwmapi.ErrInvalidInput)
		}
		if _, ok := byConnector[it.Connector]; !ok {
			order = append(order, it.Connector)
		}
		byConnector[it.Connector] = append(byConnector[it.Connector], api.SelectedItem{Key: it.Key, Title: it.Title, WithChildren: it.WithChildren})
	}
	out := api.UseSetOutput{OK: true, Sources: []string{}}
	for _, c := range order {
		held, err := s.Hold(ctx, api.HoldInput{ID: in.ID, Connector: c, Items: byConnector[c]})
		if err != nil {
			return out, err
		}
		out.Sources = append(out.Sources, held.Source)
	}
	if len(out.Sources) == 0 {
		out.Message = "Aucune source choisie."
		return out, nil
	}
	if in.Wait {
		ap, err := s.Apply(ctx, api.ApplyInput{ID: in.ID})
		if err != nil {
			return out, err
		}
		out.Message = ap.Message
		return out, nil
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		if _, err := s.Apply(bg, api.ApplyInput{ID: in.ID}); err != nil {
			slog.Warn("sources: use-set apply", "carrier", in.ID, "err", err)
		}
	}()
	out.Message = fmt.Sprintf("%d source(s) créée(s) avec leur sélection : elles sont lues en arrière-plan.", len(out.Sources))
	return out, nil
}
