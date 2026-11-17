// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// reindexTimeout bounds a kb/run: sync, extract and index of the input/.
const reindexTimeout = 10 * time.Minute

// carrier is a knowledge carrier's folders.
type carrier struct {
	id, folder, kb, input string
}

// carrierOf resolves a carrier: a module, whose folder holds its .kb/ and
// input/. Any other object is refused — a carrier that is not a module has no
// folder of its own yet.
func carrierOf(ctx context.Context, cwm cwmapi.CWMService, id string) (carrier, error) {
	if strings.TrimSpace(id) == "" {
		return carrier{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return carrier{}, err
	}
	if module != id {
		return carrier{}, fmt.Errorf("%w: %q is not a module: a knowledge carrier is a module, whose folder holds its .kb/ and input/", cwmapi.ErrInvalidInput, id)
	}
	st, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: module, Path: "."})
	if err != nil {
		return carrier{}, fmt.Errorf("sources: module %q folder: %w", module, err)
	}
	folder := filepath.ToSlash(st.ProjectPath)
	dir := filepath.Join(projectRoot(ctx, cwm, module), filepath.FromSlash(folder))
	return carrier{id: id, folder: folder, kb: filepath.Join(dir, api.KBFolder), input: filepath.Join(dir, api.InputFolder)}, nil
}

// SourcesOf lists a carrier's sources and where its knowledge base stands.
func (s *Service) SourcesOf(ctx context.Context, in api.SourcesOfInput) (api.SourcesOfOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SourcesOfOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	c, err := carrierOf(ctx, cwm, in.ID)
	if err != nil {
		return api.SourcesOfOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.SourcesOfOutput{}, err
	}
	decls := s.drawn(ctx, srcs)
	if err := followed(ctx, cwm, in.ID, srcs, decls); err != nil {
		return api.SourcesOfOutput{}, err
	}
	out := api.SourcesOfOutput{ID: in.ID, Folder: c.folder, KBPath: c.kb, InputPath: c.input, Sources: srcs}
	out.Documents, out.Pending = kbCounts(srcs)
	out.Deposited = deposited(ctx, cwm, srcs)
	out.DocumentsLabel = fmt.Sprintf("%d document(s)", out.Documents)
	out.PendingLabel = "à jour"
	if out.Pending > 0 {
		out.PendingLabel = fmt.Sprintf("%d en cours", out.Pending)
	}
	// Opening the « Sources » tab re-reads its on-open sources, in the
	// background, and a daily source keeps its carrier's timer.
	s.onOpen(ctx, cwm, in.ID, srcs)
	if _, err := os.Stat(filepath.Join(c.kb, "KnowledgeBase.yaml")); err == nil {
		out.KBExists = true
		out.KBSize = dirSize(c.kb)
	}
	out.Label = fmt.Sprintf("%d source(s)", len(srcs))
	if out.Connectors, err = s.connectorUses(ctx, cwm, srcs); err != nil {
		return api.SourcesOfOutput{}, err
	}
	out.KBLabel = "Base de connaissance absente"
	if out.KBExists {
		out.KBLabel = "Base de connaissance : " + humanSize(out.KBSize)
	}
	return out, nil
}

// WaitingCount is the « Sources » tab's badge: the
// carrier's own sources that wait on the person (api.WaitsOnPerson). A
// source only feeding it waits at its own carrier's tab. It reads the
// sources without SourcesOf's side effects (no on-open re-read, no timer).
func (s *Service) WaitingCount(ctx context.Context, in api.SourcesOfInput) (api.WaitingCountOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.WaitingCountOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.ID); err != nil {
		return api.WaitingCountOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.WaitingCountOutput{}, err
	}
	out := api.WaitingCountOutput{}
	for _, src := range srcs {
		if src.Via == "child" && api.WaitsOnPerson(api.InformationSourceFields{Error: src.Error, States: src.States}) {
			out.Count++
		}
	}
	if out.Count > 0 {
		out.Color = "red"
	}
	return out, nil
}

// connectorUses is every contributed connector as the carrier sees it: used
// when one of its own sources (a child, never one feeding it from elsewhere)
// reads through it.
func (s *Service) connectorUses(ctx context.Context, cwm cwmapi.CWMService, srcs []api.SourceSummary) ([]api.ConnectorUse, error) {
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return nil, err
	}
	own := map[string]api.SourceSummary{}
	for _, src := range srcs {
		if _, seen := own[src.Connector]; src.Via == "child" && !seen {
			own[src.Connector] = src
		}
	}
	out := make([]api.ConnectorUse, 0, len(list.Connectors))
	for _, c := range list.Connectors {
		u := api.ConnectorUse{Name: c.Name, Key: c.Name, Label: c.Label, Description: c.Description, Icon: c.Icon, UsedLabel: "non utilisée", Color: c.Color}
		if u.Label == "" {
			u.Label = c.Name
		}
		if u.Color == "" {
			u.Color = "gray"
		}
		u.State, u.Meta = &api.CardState{Text: "non utilisée", Color: "gray"}, c.Description
		if src, ok := own[c.Name]; ok {
			u.Used, u.On, u.Source, u.UsedLabel = true, true, src.ID, "utilisée"
			u.Count = pickedCount(srcs, src.ID, c)
			card(&u, src)
			// A from-connector source the other source kept values for
			// since its last save proposes what they bring.
			if kept := src.Picked + src.Held; c.From != nil && src.From != nil && kept > 0 && src.Error == "" && src.State != api.SourceStateFailed {
				if n, titles := s.fromNews(ctx, cwm, src); n > 0 {
					fc, err := s.connector(ctx, src.From.Connector)
					if err != nil {
						fc = api.ConnectorDecl{Name: src.From.Connector}
					}
					fromNewsCard(&u, c, fc, kept, n, titles)
				}
			}
		}
		u.Chips = []string{u.UsedLabel}
		out = append(out, u)
	}
	return out, nil
}

// card fills a used connector's card from its source: its
// state in a word and a colour, its meta line, the titles of the items it
// keeps, what to do when it failed or changed.
func card(u *api.ConnectorUse, src api.SourceSummary) {
	for _, k := range src.Kept {
		title := k.Title
		if title == "" {
			title = k.Key
		}
		u.Items = append(u.Items, title)
	}
	switch src.State {
	case api.SourceStateSynced:
		u.State = &api.CardState{Text: "à jour", Color: "green"}
	case api.SourceStateStale:
		u.State = &api.CardState{Text: "changée", Color: "yellow"}
		u.Note = "A changé depuis la dernière lecture : « Actualiser » pour la relire."
	case api.SourceStateFailed:
		failed := 0
		for _, st := range src.States {
			if st.State == api.ItemFailed {
				failed++
			}
		}
		u.State = &api.CardState{Text: "injoignable", Color: "red"}
		if failed > 0 {
			u.State.Text = plural(failed, "erreur")
		}
		u.Note = src.Error
	default:
		// A source declared unreachable carries its why; one
		// that locates and picks nothing has nothing to read yet
		//
		u.State = &api.CardState{Text: "pas encore lue", Color: "gray"}
		switch {
		case src.Error != "":
			u.State = &api.CardState{Text: "injoignable", Color: "red"}
			u.Note = src.Error
		case !src.Located && src.Picked == 0 && src.Held == 0:
			u.State = &api.CardState{Text: "à configurer", Color: "gray"}
			u.Note = "Choisir… pour dire quoi lire"
		}
	}
	u.Meta = "jamais lue"
	if src.LastSynced != "" {
		u.Meta = plural(src.Items, "élément")
		if at, err := time.Parse(time.RFC3339, src.LastSynced); err == nil {
			u.Meta += " · lu le " + at.Local().Format("02/01 15:04")
		}
	}
}

// plural is n and its unit, an s on the unit past one.
func plural(n int, unit string) string {
	if n > 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s", n, unit)
}

// Use starts reading a carrier's knowledge from a contributed connector: one
// information source under the carrier, declared, unless one of its own
// sources reads through it already.
func (s *Service) Use(ctx context.Context, in api.UseInput) (api.UseOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.UseOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.ID); err != nil {
		return api.UseOutput{}, err
	}
	c, err := s.connector(ctx, strings.TrimSpace(in.Connector))
	if err != nil {
		return api.UseOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.UseOutput{}, err
	}
	label := c.Label
	if label == "" {
		label = c.Name
	}
	if strings.EqualFold(strings.TrimSpace(in.On), "false") {
		for _, src := range srcs {
			if src.Via == "child" && src.Connector == c.Name {
				r, err := s.Remove(ctx, api.RemoveInput{ID: src.ID})
				return api.UseOutput{OK: r.OK, Source: src.ID, Message: r.Message}, err
			}
		}
		return api.UseOutput{OK: true, Message: fmt.Sprintf("%s n'est pas utilisée.", label)}, nil
	}
	for _, src := range srcs {
		if src.Via == "child" && src.Connector == c.Name {
			return api.UseOutput{OK: true, Source: src.ID, Message: fmt.Sprintf("%s est déjà utilisée : %s.", label, src.Name)}, nil
		}
	}
	module := moduleOf(in.ID)
	local := c.Name
	for i := 2; ; i++ {
		if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: module + "." + local}); err != nil {
			break
		}
		local = fmt.Sprintf("%s-%d", c.Name, i)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = label
	}
	scope := strings.TrimSpace(in.Scope)
	if scope != "" && (c.Scopes == nil || !c.Scopes.Add) {
		return api.UseOutput{}, fmt.Errorf("%w: the connector %s lets no carrier add a scope", cwmapi.ErrInvalidInput, c.Name)
	}
	obj := cwmapi.NewContainerObject(module, local, api.InformationSourceType, module, name, "")
	raw, err := json.Marshal(api.InformationSourceFields{Connector: c.Name, Sync: "manual"})
	if err != nil {
		return api.UseOutput{}, err
	}
	obj.SetFieldsJSON(raw)
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
	if err != nil {
		return api.UseOutput{}, fmt.Errorf("sources: use %s on %s: %w", c.Name, in.ID, err)
	}
	if scope != "" {
		if _, err := s.ScopeAdd(ctx, api.ScopeAddInput{Connector: c.Name, Object: in.ID, Scope: scope}); err != nil {
			return api.UseOutput{}, err
		}
	}
	return api.UseOutput{OK: true, Created: true, Source: made.Object.ID,
		Open:    &render.Navigate{Object: made.Object.ID, Panel: api.SourceTakePanel},
		Message: fmt.Sprintf("%s est utilisée : choisissez ce qu'elle lit, puis synchronisez.", label)}, nil
}

// drawn gives each source its connector as the views draw it - its declared
// label, icon and colour, its name only when it declares no label
// : no view shows a connector's name.
// It answers the connectors' declarations by name.
func (s *Service) drawn(ctx context.Context, srcs []api.SourceSummary) map[string]api.ConnectorDecl {
	by := map[string]api.ConnectorDecl{}
	if list, err := s.Connectors(ctx, api.ConnectorsInput{}); err == nil {
		for _, c := range list.Connectors {
			by[c.Name] = c
		}
	}
	for i := range srcs {
		c := by[srcs[i].Connector]
		srcs[i].ConnectorLabel, srcs[i].ConnectorIcon, srcs[i].ConnectorColor = c.Label, c.Icon, c.Color
		if srcs[i].ConnectorLabel == "" {
			srcs[i].ConnectorLabel = srcs[i].Connector
		}
	}
	return by
}

// followed gives each source whose connector declares from the values it
// follows, read as its sync reads them: from its own
// carrier - the carrier asked for a child, the one it is under for a source
// feeding it.
func followed(ctx context.Context, cwm cwmapi.CWMService, carrierID string, srcs []api.SourceSummary, decls map[string]api.ConnectorDecl) error {
	for i := range srcs {
		c, ok := decls[srcs[i].Connector]
		if !ok || c.From == nil {
			continue
		}
		carrier := carrierID
		if srcs[i].Via != "child" {
			carrier = carrierOfSource(ctx, cwm, srcs[i].ID)
		}
		got, err := fromItems(ctx, cwm, carrier, c)
		if err != nil {
			return err
		}
		srcs[i].From = got
	}
	return nil
}

// sourcesOf is a carrier's information sources: its children of the type,
// then those that feed it, each once, sorted by id within each group.
func sourcesOf(ctx context.Context, cwm cwmapi.CWMService, id string) ([]api.SourceSummary, error) {
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: id, Type: api.InformationSourceType})
	if err != nil {
		return nil, fmt.Errorf("sources: the sources under %s: %w", id, err)
	}
	out := []api.SourceSummary{}
	seen := map[string]bool{}
	// A list answers resident metadata: each source is read whole for its
	// fields.
	for _, o := range kids.Objects {
		module, err := cwmapi.ModuleOfObjectID(o.ID)
		if err != nil {
			return nil, err
		}
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: o.ID})
		if err != nil {
			return nil, fmt.Errorf("sources: read %s: %w", o.ID, err)
		}
		seen[o.ID] = true
		out = append(out, summaryOf(got.Object, "child"))
	}
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: id, Kind: api.FeedsEdge, Direction: cwmapi.DirectionIn})
	if err != nil {
		return nil, fmt.Errorf("sources: the sources feeding %s: %w", id, err)
	}
	var fed []api.SourceSummary
	for _, e := range edges.Edges {
		if e.To != id || seen[e.From] {
			continue
		}
		module, err := cwmapi.ModuleOfObjectID(e.From)
		if err != nil {
			continue
		}
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: e.From})
		if err != nil || got.Object.Type != api.InformationSourceType {
			continue
		}
		seen[e.From] = true
		fed = append(fed, summaryOf(got.Object, "feeds"))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	sort.Slice(fed, func(i, j int) bool { return fed[i].ID < fed[j].ID })
	return append(out, fed...), nil
}

func summaryOf(o cwmapi.Object, via string) api.SourceSummary {
	var f api.InformationSourceFields
	if raw := o.FieldsJSON(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &f)
	}
	state, _ := o.Capabilities["lifecycle"]["state"].(string)
	if state == "" {
		state = api.SourceStateDeclared
	}
	name := o.Name
	if name == "" {
		name = localID(o.ID)
	}
	picked := api.SelectedItems(f.Selection)
	return api.SourceSummary{ID: o.ID, Name: name, Connector: f.Connector, State: state,
		LastSynced: f.LastSynced, Items: f.Items, Picked: len(picked), PickedLabel: pickedLabel(picked), Held: len(f.Held), Located: located(f.Location), States: f.States, Error: f.Error, Via: via,
		Kept: append(append([]api.SelectedItem{}, picked...), f.Held...)}
}

// located says whether a source's location names somewhere to read: one
// value at least that is not empty.
func located(location map[string]any) bool {
	for _, v := range location {
		if s, ok := v.(string); ok {
			if strings.TrimSpace(s) != "" {
				return true
			}
			continue
		}
		if v != nil {
			return true
		}
	}
	return false
}

// pickedLabel names the items picked for a source's row: their titles (keys
// when untitled), the first few, then how many more.
func pickedLabel(items []api.SelectedItem) string {
	const shown = 3
	var names []string
	for i, it := range items {
		if i == shown {
			names = append(names, fmt.Sprintf("+%d", len(items)-shown))
			break
		}
		n := it.Title
		if n == "" {
			n = it.Key
		}
		if it.WithChildren {
			n += " (+ sous-éléments)"
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return ""
	}
	return "Choisis : " + strings.Join(names, ", ")
}

// SyncAll syncs each source of the carrier, then reindexes its knowledge base.
func (s *Service) SyncAll(ctx context.Context, in api.SyncAllInput) (api.SyncAllOutput, error) {
	of, err := s.SourcesOf(ctx, api.SourcesOfInput{ID: in.ID})
	if err != nil {
		return api.SyncAllOutput{}, err
	}
	out := api.SyncAllOutput{Sources: []api.SourceSyncResult{}}
	failed := 0
	// Each source a step, the reindex one more when the base is set up.
	steps := len(of.Sources)
	if of.KBExists {
		steps++
	}
	for i, src := range of.Sources {
		step(ctx, "Synchronisation de "+sourceLabel(src), i, steps)
		res, err := s.SourceSync(quiet(ctx), api.SourceSyncInput{ID: src.ID})
		r := api.SourceSyncResult{ID: src.ID, OK: err == nil && res.OK, State: res.State, Items: res.Items}
		switch {
		case err != nil:
			r.Error = err.Error()
		case !res.OK:
			r.Error = res.Message
		}
		if !r.OK {
			failed++
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %s", src.ID, r.Error))
		}
		out.Sources = append(out.Sources, r)
	}
	synced := fmt.Sprintf("%d source(s) synchronisée(s)", len(of.Sources)-failed)
	if failed > 0 {
		synced += fmt.Sprintf(", %d en échec", failed)
	}
	if !of.KBExists {
		out.Message = synced + " ; la base de connaissance n'est pas créée (" + of.KBPath + ") : rien n'est réindexé."
		out.OK = failed == 0
		step(ctx, "Sources synchronisées", steps, steps)
		return out, nil
	}
	step(ctx, "Réindexation de la base de connaissance", len(of.Sources), steps)
	if err := reindex(ctx, of.KBPath); err != nil {
		out.Errors = append(out.Errors, "reindex: "+err.Error())
		out.Message = synced + " ; la réindexation a échoué : " + err.Error()
		return out, nil
	}
	step(ctx, "Base de connaissance réindexée", steps, steps)
	out.Reindexed = true
	out.OK = failed == 0
	out.Message = synced + " ; base de connaissance réindexée."
	return out, nil
}

// Reindex reindexes a carrier's knowledge base.
func (s *Service) Reindex(ctx context.Context, in api.ReindexInput) (api.ReindexOutput, error) {
	of, err := s.SourcesOf(ctx, api.SourcesOfInput{ID: in.ID})
	if err != nil {
		return api.ReindexOutput{}, err
	}
	if !of.KBExists {
		return api.ReindexOutput{Message: "La base de connaissance n'est pas créée (" + of.KBPath + ")."}, nil
	}
	step(ctx, "Réindexation de la base de connaissance", 0, 1)
	if err := reindex(ctx, of.KBPath); err != nil {
		return api.ReindexOutput{Message: "La réindexation a échoué : " + err.Error()}, nil
	}
	step(ctx, "Base de connaissance réindexée", 1, 1)
	return api.ReindexOutput{OK: true, Message: "Base de connaissance réindexée."}, nil
}

// moduleOf is the module an object id names (the id itself for a module).
func moduleOf(id string) string {
	if m, err := cwmapi.ModuleOfObjectID(id); err == nil {
		return m
	}
	return id
}

// reindex runs the knowledge base engine on a .kb folder. A run that
// answers is not yet a reindexed base: the failures it reports - its sync's
// errors, its failed extractions, its index errors - fail the reindex, the
// first of each named with its count.
func reindex(ctx context.Context, kb string) error {
	run, err := call[kbapi.RunOutput](ctx, api.KBRunURI, reindexTimeout, map[string]any{"path": kb})
	if err != nil {
		return err
	}
	var why []string
	failures := func(what string, n int, errs []string) {
		if n < len(errs) {
			n = len(errs)
		}
		if n == 0 {
			return
		}
		w := fmt.Sprintf("%d %s", n, what)
		if len(errs) > 0 {
			w += " (" + errs[0] + ")"
		}
		why = append(why, w)
	}
	failures("erreur(s) de synchronisation", 0, run.Sync.Errors)
	failures("extraction(s) en échec", run.Extract.Failed, run.Extract.Errors)
	failures("erreur(s) d'indexation", 0, run.Index.Errors)
	if len(why) > 0 {
		return fmt.Errorf("%s", strings.Join(why, " ; "))
	}
	return nil
}

// dirSize is the bytes of the files under dir.
func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f Mo", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f Ko", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d o", n)
}

// kbCounts is the knowledge-base box: the documents the
// carrier's own sources brought, and the items still in progress - held,
// or waiting for a read or a removal.
func kbCounts(srcs []api.SourceSummary) (documents, pending int) {
	for _, src := range srcs {
		if src.Via != "child" {
			continue
		}
		documents += src.Items
		pending += src.Held
		for _, st := range src.States {
			if st.State == api.ItemWaiting {
				pending++
			}
		}
	}
	return documents, pending
}

// pickedCount is what a used source picks, in a few words: « 2
// applications », the connector's unit.
func pickedCount(srcs []api.SourceSummary, id string, c api.ConnectorDecl) string {
	unit := "élément"
	if c.Picker != nil && c.Picker.Unit != "" {
		unit = c.Picker.Unit
	}
	for _, src := range srcs {
		if src.ID != id {
			continue
		}
		n := src.Picked + src.Held
		if n == 0 {
			return "tout"
		}
		if n > 1 {
			unit += "s"
		}
		return fmt.Sprintf("%d %s", n, unit)
	}
	return ""
}
