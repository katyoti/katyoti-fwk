// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// SearchSources searches every source in use at once: each
// connector's propose - else its search or, reading from another source,
// its browse - starts together, its state emitted each time it moves; a
// connector reading from another source waits for that source's answer and
// follows what it proposed beside what the carrier kept. What the carrier
// already holds is left out (« Chercher encore »); with no carrier nothing
// is held or created. Only searches one source again alone
func (s *Service) SearchSources(ctx context.Context, in api.SearchSourcesInput) (api.SearchSourcesOutput, error) {
	if len(in.Terms) == 0 && strings.TrimSpace(in.Text) == "" {
		return api.SearchSourcesOutput{}, fmt.Errorf("%w: sources.SearchSources: terms or text required", cwmapi.ErrInvalidInput)
	}
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.SearchSourcesOutput{}, err
	}
	byName := map[string]api.ConnectorDecl{}
	for _, c := range list.Connectors {
		byName[c.Name] = c
	}
	names := in.Connectors
	if len(names) == 0 {
		for _, c := range list.Connectors {
			if picks(c) {
				names = append(names, c.Name)
			}
		}
	}
	var asked []api.ConnectorDecl
	for _, name := range names {
		c, ok := byName[name]
		if !ok {
			return api.SearchSourcesOutput{}, fmt.Errorf("%w: no source connector %q", cwmapi.ErrInvalidInput, name)
		}
		if picks(c) {
			asked = append(asked, c)
		}
	}
	// Only: that source alone is answered; the sources it reads from are
	// searched again quietly, for what it follows.
	shown := map[string]bool{}
	if only := strings.TrimSpace(in.Only); only != "" {
		c, ok := byName[only]
		if !ok || !picks(c) {
			return api.SearchSourcesOutput{}, fmt.Errorf("%w: no source connector %q that picks", cwmapi.ErrInvalidInput, only)
		}
		shown[only] = true
		asked = []api.ConnectorDecl{c}
		// Every connector it follows, and theirs in turn.
		for i := 0; i < len(asked); i++ {
			for _, name := range asked[i].From.Names() {
				if src, ok := byName[name]; ok && !shown[src.Name] && !inList(asked, src.Name) {
					asked = append(asked, src)
				}
			}
		}
	} else {
		for _, c := range asked {
			shown[c.Name] = true
		}
	}
	held := map[string]map[string]bool{}
	if in.Object != "" {
		if held, err = carrierHolds(ctx, in.Object); err != nil {
			return api.SearchSourcesOutput{}, err
		}
	}
	r := &searchRun{s: s, in: in, held: held, shown: shown, ctx: ctx,
		at: map[string]int{}, done: map[string]chan struct{}{}, heldItems: map[string][]api.ProposedItem{}}
	for _, c := range asked {
		st := api.SourceSearch{Connector: c.Name, Label: c.Label, Icon: c.Icon, Color: c.Color, Kinds: c.Kinds, State: api.SearchSearching, Items: []api.ProposedItem{}}
		if c.Label == "" {
			st.Label = c.Name
		}
		for _, name := range c.From.Names() {
			if inList(asked, name) {
				st.State, st.Waiting, st.WaitingLabel = api.SearchWaiting, name, labelOf(byName[name])
				break
			}
		}
		r.at[c.Name] = len(r.states)
		r.states = append(r.states, st)
		r.done[c.Name] = make(chan struct{})
	}
	r.emit()
	var wg sync.WaitGroup
	for _, c := range asked {
		wg.Add(1)
		go func(c api.ConnectorDecl) {
			defer wg.Done()
			defer close(r.done[c.Name])
			r.search(c)
		}(c)
	}
	wg.Wait()
	out := api.SearchSourcesOutput{Sources: []api.SourceSearch{}}
	for _, st := range r.states {
		if shown[st.Connector] {
			out.Sources = append(out.Sources, st)
		}
	}
	return out, nil
}

// searchRun is one multi-source search: where each source stands, and the
// channel each closes once it has answered.
type searchRun struct {
	s     *Service
	ctx   context.Context
	in    api.SearchSourcesInput
	held  map[string]map[string]bool
	shown map[string]bool
	mu    sync.Mutex
	at    map[string]int
	done  map[string]chan struct{}
	// states is written under mu; a source's goroutine owns its entry.
	states []api.SourceSearch
	// heldItems are the items a source found that the carrier holds: left
	// out of its answer, followed by a source reading from it.
	heldItems map[string][]api.ProposedItem
}

// search answers one source: its status, then - once the source it reads
// from has answered - its proposal, the carrier's held items left out.
func (r *searchRun) search(c api.ConnectorDecl) {
	ctx := r.ctx
	var from *api.FromItems
	if c.From != nil {
		// It waits on every source it follows that this search asks.
		waited := false
		for _, name := range c.From.Names() {
			wait, ok := r.done[name]
			if !ok {
				continue
			}
			select {
			case <-wait:
			case <-ctx.Done():
				r.set(c.Name, func(st *api.SourceSearch) { st.State, st.Message = api.SearchUnreachable, ctx.Err().Error() })
				return
			}
			waited = true
		}
		if waited {
			r.set(c.Name, func(st *api.SourceSearch) { st.State, st.Waiting, st.WaitingLabel = api.SearchSearching, "", "" })
		}
		var err error
		if from, err = r.from(c); err != nil {
			r.set(c.Name, func(st *api.SourceSearch) { st.State, st.Message = api.SearchUnreachable, err.Error() })
			return
		}
	}
	if c.Test != "" {
		got, err := call[api.ConnectorTestOutput](ctx, c.Test, statusTimeout, api.ConnectorSyncInput{})
		switch {
		case err != nil:
			r.set(c.Name, func(st *api.SourceSearch) { st.State, st.Message = api.SearchUnreachable, err.Error() })
			return
		case got.NotConfigured:
			r.set(c.Name, func(st *api.SourceSearch) {
				st.State, st.Message, st.Configure = api.SearchNotConfigured, got.Message, c.Configure
			})
			return
		case !got.OK:
			r.set(c.Name, func(st *api.SourceSearch) { st.State, st.Message = api.SearchUnreachable, got.Message })
			return
		}
	}
	got, err := r.propose(c, from)
	if err != nil {
		r.set(c.Name, func(st *api.SourceSearch) {
			st.State, st.Message, st.Errors = api.SearchUnreachable, err.Error(), got.Errors
		})
		return
	}
	items, kept := []api.ProposedItem{}, []api.ProposedItem{}
	for _, it := range got.Items {
		if r.held[c.Name][it.Key] {
			kept = append(kept, it)
			continue
		}
		items = append(items, it)
	}
	r.set(c.Name, func(st *api.SourceSearch) {
		st.Items, st.Found, st.Held, st.Errors = items, len(items), len(kept), got.Errors
		st.State = api.SearchFound
		if len(items) == 0 {
			st.State = api.SearchNothingNew
		}
		r.heldItems[c.Name] = kept
	})
}

// from is what a connector reading from another source follows: the
// attribute's values on what the carrier kept of that source, then on what
// that source proposed in this search, each once, with its item's title.
func (r *searchRun) from(c api.ConnectorDecl) (*api.FromItems, error) {
	out := &api.FromItems{Connector: c.From.First(), Attr: c.From.Attr, Values: []string{}}
	if r.in.Object != "" {
		cwm, err := application.UseService[cwmapi.CWMService](r.ctx)
		if err != nil {
			return nil, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		kept, err := fromItems(r.ctx, cwm, r.in.Object, c)
		if err != nil {
			return nil, err
		}
		out.Connector = kept.Connector
		out.Values = append(out.Values, kept.Values...)
		for v, t := range kept.Titles {
			out.SetTitle(v, t)
		}
		for v, n := range kept.Connectors {
			out.SetConnector(v, n)
		}
	}
	seen := map[string]bool{}
	for _, v := range out.Values {
		seen[v] = true
	}
	for _, name := range c.From.Names() {
		for _, it := range r.followed(name) {
			if v := it.Attrs[c.From.Attr]; v != "" && !seen[v] {
				seen[v] = true
				out.Values = append(out.Values, v)
				out.SetTitle(v, it.Title)
				out.SetConnector(v, name)
			}
		}
	}
	return out, nil
}

// followed is everything the source a connector reads from answered in this
// search - the items the carrier holds included.
func (r *searchRun) followed(name string) []api.ProposedItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.at[name]
	if !ok {
		return nil
	}
	return append(append([]api.ProposedItem{}, r.states[i].Items...), r.heldItems[name]...)
}

// propose is one connector's proposal from the context: its propose method,
// else its search once per term, else - reading from another source - its
// browse of what it follows. A connector that does none of them proposes
// nothing: it is browsed by hand.
func (r *searchRun) propose(c api.ConnectorDecl, from *api.FromItems) (api.ConnectorProposeOutput, error) {
	ctx := r.ctx
	var location map[string]any
	root := ""
	if r.in.Object != "" {
		var err error
		if location, root, err = r.s.carrierLocation(ctx, r.in.Object, c.Name); err != nil {
			return api.ConnectorProposeOutput{}, err
		}
	}
	text := strings.TrimSpace(r.in.Text)
	// The carrier's source's own keywords and bound.
	kws, max := r.s.searchSettings(ctx, r.in.Object, c.Name)
	terms := withKeywords(r.in.Terms, kws)
	switch {
	case c.Propose != "":
		out, err := call[api.ConnectorProposeOutput](ctx, c.Propose, proposeTimeout,
			api.ConnectorProposeInput{Location: location, Root: root, Terms: terms, Text: text, From: from, Max: max})
		out = bounded(out, max)
		if out.Items == nil {
			out.Items = []api.ProposedItem{}
		}
		return out, err
	case c.Search != "":
		return api.ProposeBySearch(ctx, api.ConnectorProposeInput{Terms: terms, Text: text, Max: max}, func(ctx context.Context, q string) ([]api.PickItem, error) {
			got, err := call[api.ConnectorSearchOutput](ctx, c.Search, searchTimeout,
				api.ConnectorSearchInput{Location: location, Root: root, Query: q, From: from})
			return got.Items, err
		})
	case from != nil && c.Browse != "":
		out := api.ConnectorProposeOutput{Items: []api.ProposedItem{}}
		if len(from.Values) == 0 {
			return out, nil // nothing to follow yet
		}
		got, err := call[api.ConnectorBrowseOutput](ctx, c.Browse, browseTimeout,
			api.ConnectorBrowseInput{Location: location, Root: root, From: from})
		if err != nil {
			return out, err
		}
		var followed []string
		for _, n := range from.Followed() {
			followed = append(followed, labelOf(api.ConnectorDecl{Name: n, Label: r.label(n)}))
		}
		why := "Suit " + andList(followed)
		for _, it := range got.Items {
			out.Items = append(out.Items, api.ProposedItem{PickItem: api.PickItem{Key: it.Key, Title: it.Title, Parent: it.Parent,
				Kind: it.Kind, URI: it.URI, Badges: it.Badges, HasChildren: it.HasChildren, Path: it.Path, Detail: it.Detail,
				Attrs: it.Attrs, Modified: it.Modified}, Why: why, Score: 1})
		}
		return out, nil
	}
	return api.ConnectorProposeOutput{Items: []api.ProposedItem{}}, nil
}

// label is a source's label in this search.
func (r *searchRun) label(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i, ok := r.at[name]; ok {
		return r.states[i].Label
	}
	return name
}

// set moves one source and emits the search as it stands.
func (r *searchRun) set(name string, move func(*api.SourceSearch)) {
	r.mu.Lock()
	move(&r.states[r.at[name]])
	r.mu.Unlock()
	r.emit()
}

// emit publishes the search as it stands: the shown sources as a snapshot
// (progress.Emit - the CLI's live view, a test) and, in a panel the run lent,
// laid over its source-search node; a progress line counts the sources
// answered.
func (r *searchRun) emit() {
	r.mu.Lock()
	snap := api.SearchSourcesOutput{Sources: []api.SourceSearch{}}
	answered := 0
	var last string
	for _, st := range r.states {
		if !r.shown[st.Connector] {
			continue
		}
		st.Items = append([]api.ProposedItem{}, st.Items...)
		snap.Sources = append(snap.Sources, st)
		if st.Done() {
			answered++
			last = st.Label + " : " + stateWords(st)
		}
	}
	r.mu.Unlock()
	progress.Emit(r.ctx, snap)
	progress.Emit(r.ctx, panel.Progress{Label: last, Current: int64(answered), Total: int64(len(snap.Sources))})
	if h, ok := panel.HandleFrom(r.ctx); ok {
		if err := h.Patch(r.ctx, render.Op{Op: render.OpMerge, Target: api.SearchSourcesNode, Value: map[string]any{"data": snap}}); err != nil {
			slog.Warn("sources: a search answer was not sent", "panel", h.ID(), "err", err)
		}
	}
}

// stateWords says a source's state in the panel's words.
func stateWords(st api.SourceSearch) string {
	switch st.State {
	case api.SearchFound:
		return fmt.Sprintf("%d trouvé(s)", st.Found)
	case api.SearchNothingNew:
		return "rien de nouveau"
	case api.SearchUnreachable:
		return "injoignable"
	case api.SearchNotConfigured:
		return "non configurée"
	case api.SearchWaiting:
		return "attend " + st.WaitingLabel
	}
	return "recherche…"
}

// carrierHolds is, by connector, the keys of the items the carrier's own
// sources already pick or hold.
func carrierHolds(ctx context.Context, carrierID string) (map[string]map[string]bool, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, carrierID); err != nil {
		return nil, err
	}
	srcs, err := sourcesOf(ctx, cwm, carrierID)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, src := range srcs {
		if src.Via != "child" {
			continue
		}
		if out[src.Connector] == nil {
			out[src.Connector] = map[string]bool{}
		}
		for _, it := range src.Kept {
			out[src.Connector][it.Key] = true
		}
	}
	return out, nil
}

func labelOf(c api.ConnectorDecl) string {
	if c.Label != "" {
		return c.Label
	}
	return c.Name
}

func inList(cs []api.ConnectorDecl, name string) bool {
	for _, c := range cs {
		if c.Name == name {
			return true
		}
	}
	return false
}
