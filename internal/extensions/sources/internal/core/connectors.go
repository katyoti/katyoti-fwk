// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// Connectors lists the source-connector entries of the universe — of this
// binary alone when no catalog aggregates the universe (a test, an offline
// command).
func (s *Service) Connectors(ctx context.Context, _ api.ConnectorsInput) (api.ConnectorsOutput, error) {
	entries, err := mbapi.QueryTyped[api.ConnectorDecl](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.ContributionTypeSourceConnector})
	if err != nil {
		entries, err = mbapi.QueryTyped[api.ConnectorDecl](ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: api.ContributionTypeSourceConnector})
	}
	if err != nil {
		return api.ConnectorsOutput{}, fmt.Errorf("sources: list the source connectors: %w", err)
	}
	out := api.ConnectorsOutput{Connectors: []api.ConnectorDecl{}}
	for _, e := range entries {
		out.Connectors = append(out.Connectors, e.Body)
	}
	sort.SliceStable(out.Connectors, func(i, j int) bool { return out.Connectors[i].Name < out.Connectors[j].Name })
	// A from or propose_from naming a role follows every connector playing
	// it: the role is resolved here, once, so every reader
	// asks Names.
	for i := range out.Connectors {
		out.Connectors[i].From = out.Connectors[i].From.WithRole(out.Connectors)
		out.Connectors[i].ProposeFrom = out.Connectors[i].ProposeFrom.WithRole(out.Connectors)
	}
	return out, nil
}

// connector is the contributed connector of that name.
func (s *Service) connector(ctx context.Context, name string) (api.ConnectorDecl, error) {
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.ConnectorDecl{}, err
	}
	for _, c := range list.Connectors {
		if c.Name == name {
			return c, nil
		}
	}
	return api.ConnectorDecl{}, fmt.Errorf("%w: no source connector %q is contributed in this universe", cwmapi.ErrNotFound, name)
}

// call runs a connector method by its URI, with the connector's deadline.
func call[O any](ctx context.Context, uri string, timeout time.Duration, in any) (O, error) {
	var out O
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return out, fmt.Errorf("sources: resolve ActionService: %w", err)
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := actions.Run(ctx, actionsapi.RunInput{URI: uri, Input: raw, Context: callContext(ctx)})
	if err != nil {
		return out, err
	}
	if len(res.Value) == 0 {
		return out, fmt.Errorf("sources: %s answered no value", uri)
	}
	if err := json.Unmarshal(res.Value, &out); err != nil {
		return out, fmt.Errorf("sources: %s answered %s: %w", uri, res.Value, err)
	}
	return out, nil
}

// SourceCheck is sources' save gate on an information source:
// sources owns the type, so its before hooks ask the source's connector's
// check - a pack never gates a type it does not own. A source naming no
// connector, a connector not contributed here or one declaring no check is
// accepted by it. Before that, a source about to be created under a holder
// already reading its connector is refused: one source per connector per
// holder (heldConnector).
func (s *Service) SourceCheck(ctx context.Context, in api.SourceCheckInput) (cwmapi.HookAccept, error) {
	if in.Object == nil {
		return cwmapi.HookAccept{Accept: true}, nil
	}
	if reason := s.heldConnector(ctx, *in.Object); reason != "" {
		return cwmapi.HookAccept{Reason: reason}, nil
	}
	name, _ := in.Object.Fields["connector"].(string)
	if name == "" {
		return cwmapi.HookAccept{Accept: true}, nil
	}
	c, err := s.connector(ctx, name)
	if err != nil || c.Check == "" {
		return cwmapi.HookAccept{Accept: true}, nil
	}
	return call[cwmapi.HookAccept](ctx, c.Check, 30*time.Second, in)
}

// SourceTest asks the connector whether it can read a location.
// Given a saved source (id), its own connector, location and selection are
// checked.
func (s *Service) SourceTest(ctx context.Context, in api.SourceTestInput) (api.ConnectorTestOutput, error) {
	if in.ID != "" {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.ConnectorTestOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		f, _, err := sourceFields(ctx, cwm, in.ID)
		if err != nil {
			return api.ConnectorTestOutput{}, err
		}
		in.Connector, in.Location, in.Selection = f.Connector, f.Location, f.Selection
	}
	c, err := s.connector(ctx, in.Connector)
	if err != nil {
		return api.ConnectorTestOutput{}, err
	}
	if c.Test == "" {
		return api.ConnectorTestOutput{Message: fmt.Sprintf("Le connecteur %s ne se vérifie qu'en synchronisant.", c.Name)}, nil
	}
	return call[api.ConnectorTestOutput](ctx, c.Test, 75*time.Second, api.ConnectorSyncInput{Location: in.Location, Selection: in.Selection})
}

// SourceSync reads an information source again through its connector, anchors
// what it brought and records the outcome on the source.
func (s *Service) SourceSync(ctx context.Context, in api.SourceSyncInput) (api.SourceSyncOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SourceSyncOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.SourceSyncOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.SourceSyncOutput{}, err
	}
	src := got.Object
	if src.Type != api.InformationSourceType {
		return api.SourceSyncOutput{}, fmt.Errorf("%w: %q is a %s, not an %s", cwmapi.ErrInvalidInput, in.ID, src.Type, api.InformationSourceType)
	}
	var f api.InformationSourceFields
	if raw := src.FieldsJSON(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &f); err != nil {
			return api.SourceSyncOutput{}, fmt.Errorf("sources: %s fields: %w", in.ID, err)
		}
	}
	root := projectRoot(ctx, cwm, module)
	st, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: module, Path: "."})
	if err != nil {
		return api.SourceSyncOutput{}, fmt.Errorf("sources: module %q folder: %w", module, err)
	}
	folder := path.Join(filepath.ToSlash(st.ProjectPath), "input", localID(in.ID))
	dest := filepath.Join(root, filepath.FromSlash(folder))

	out := api.SourceSyncOutput{Folder: folder}
	// A generated document is read like any other, marked « IA »
	// (F-020): nothing is left out for it.
	step(ctx, "Lecture de la source par son connecteur", 0, 0)
	syncing(ctx, cwm, api.SourceSyncProgress{Source: in.ID, Phase: api.SyncStarted})
	res, decl, syncErr := s.runSync(ctx, f, dest, root, src.ParentID())
	// notRead counts the items the connector named and this sync could not
	// read or anchor, of total: a source with any is not synced.
	notRead, total := 0, 0
	if syncErr == nil {
		out.Items, out.Errors = len(res.Items), append([]string{}, res.Errors...)
		notRead, total = len(res.Errors), len(res.Items)+len(res.Errors)
		every := readEvery(total)
		for i, it := range res.Items {
			step(ctx, "Ancrage des éléments lus", i, len(res.Items))
			// Read counts the items anchored so far, as the closing event
			// does: an item that fails to anchor never counts, so a follower
			// never sees the count go back.
			if i%every == 0 {
				syncing(ctx, cwm, api.SourceSyncProgress{Source: in.ID, Phase: api.SyncItems, Read: len(out.Anchors), Total: total})
			}
			id, changed, err := s.anchorItem(ctx, root, module, in.ID, folder, it, decl)
			if changed {
				name := it.Title
				if name == "" {
					name = it.MainFile()
				}
				out.Changed = append(out.Changed, name)
			}
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", it.MainFile(), err))
				notRead++
				continue
			}
			out.Anchors = append(out.Anchors, id)
			if it.Key != "" {
				out.Keys = append(out.Keys, it.Key)
			}
		}
		syncing(ctx, cwm, api.SourceSyncProgress{Source: in.ID, Phase: api.SyncItems, Read: total - notRead, Total: total})
		// A sync read whole is what the source now brings: what an earlier
		// one brought and this one did not - an item unpicked, a file gone -
		// leaves its folder and its anchor is archived. A sync with errors
		// drops nothing: what it could not read may still be wanted.
		if len(out.Errors) == 0 {
			removed, err := s.dropUnbrought(ctx, cwm, module, in.ID, dest, res.Items, out.Anchors)
			if err != nil {
				out.Errors = append(out.Errors, err.Error())
			}
			out.Removed = removed
		}
	}

	// A whole sync removed what the choice dropped: an item no longer chosen
	// leaves the states, so it is no longer « à retirer ».
	if syncErr == nil && len(out.Errors) == 0 {
		chosen := map[string]bool{}
		for _, it := range api.SelectedItems(f.Selection) {
			chosen[it.Key] = true
		}
		for k := range f.States {
			if !chosen[k] {
				delete(f.States, k)
			}
		}
	}

	// Record the facts, then move the state. A sync that could not read some
	// of what its connector named is not synced: the source
	// fails, saying how many and why, and the next whole sync clears it.
	f.LastSynced, f.Folder, f.Items, f.Error = time.Now().UTC().Format(time.RFC3339), folder, out.Items, ""
	out.State = api.SourceStateSynced
	switch {
	case syncErr != nil:
		f.Error, out.State = syncErr.Error(), api.SourceStateFailed
		out.Errors = append(out.Errors, syncErr.Error())
		out.Message = "La synchronisation a échoué : " + syncErr.Error()
	case len(out.Errors) > 0:
		why := strings.Join(out.Errors, " ; ")
		if notRead > 0 {
			f.Error = fmt.Sprintf("%d élément(s) sur %d n'ont pas pu être lus : %s", notRead, total, why)
		} else {
			f.Error = "La synchronisation est incomplète : " + why
		}
		out.State = api.SourceStateFailed
		out.Message = fmt.Sprintf("Synchronisation incomplète : %d élément(s) lus dans %s ; %s", out.Items, folder, f.Error)
	default:
		out.OK = true
		out.Message = fmt.Sprintf("Source synchronisée : %d élément(s) dans %s.", out.Items, folder)
	}
	step(ctx, "Enregistrement de la source", len(res.Items), len(res.Items))
	// A sync that could not record its source still ends its events.
	abort := func(err error) (api.SourceSyncOutput, error) {
		syncing(ctx, cwm, api.SourceSyncProgress{Source: in.ID, Phase: api.SyncFailed, Read: total - notRead, Total: total, Error: err.Error()})
		return out, err
	}
	if err := s.recordSource(ctx, cwm, module, in.ID, f); err != nil {
		return abort(err)
	}
	if len(out.Changed) > 0 {
		s.raiseChanged(ctx, cwm, in.ID, out.Changed)
	}
	if err := s.moveSource(ctx, cwm, in.ID, out.State); err != nil {
		return abort(err)
	}
	end := api.SourceSyncProgress{Source: in.ID, Phase: api.SyncEnded, Read: total - notRead, Total: total}
	if out.State == api.SourceStateFailed {
		end.Phase, end.Error = api.SyncFailed, f.Error
	}
	syncing(ctx, cwm, end)
	return out, nil
}

// dropUnbrought removes from dest the files no item of this sync is, and
// archives the source's anchors this sync did not write: what was unpicked
// since. It answers the anchors archived.
func (s *Service) dropUnbrought(ctx context.Context, cwm cwmapi.CWMService, module, sourceID, dest string, items []api.ConnectorItem, anchors []string) ([]string, error) {
	keep := map[string]bool{}
	for _, it := range items {
		for _, f := range it.Files() {
			keep[filepath.Clean(filepath.Join(dest, filepath.FromSlash(f)))] = true
		}
	}
	var dirs []string
	err := filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dest {
				dirs = append(dirs, p)
			}
			return nil
		}
		if !keep[filepath.Clean(p)] {
			return os.Remove(p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sources: drop what %s no longer brings: %w", sourceID, err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // only an empty one goes
	}
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: sourceID, Type: api.SourceDocumentType})
	if err != nil {
		return nil, err
	}
	written := map[string]bool{}
	for _, a := range anchors {
		written[a] = true
	}
	var removed []string
	for _, k := range kids.Objects {
		if written[k.ID] {
			continue
		}
		if _, err := cwm.ObjectArchive(ctx, cwmapi.ObjectArchiveInput{ID: k.ID}); err != nil {
			return removed, fmt.Errorf("sources: archive %s: %w", k.ID, err)
		}
		removed = append(removed, k.ID)
	}
	sort.Strings(removed)
	return removed, nil
}

// runSync calls the source's connector; it answers what the sync brought
// and the connector's declaration.
func (s *Service) runSync(ctx context.Context, f api.InformationSourceFields, dest, root, carrier string) (api.ConnectorSyncOutput, api.ConnectorDecl, error) {
	if f.Connector == "" {
		return api.ConnectorSyncOutput{}, api.ConnectorDecl{}, errors.New("the source names no connector")
	}
	c, err := s.connector(ctx, f.Connector)
	if err != nil {
		return api.ConnectorSyncOutput{}, c, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return api.ConnectorSyncOutput{}, c, err
	}
	in := api.ConnectorSyncInput{Location: f.Location, Selection: f.Selection, Dest: dest, Root: root}
	// A connector reading from another source syncs what the carrier's
	// source of it kept.
	if c.From != nil {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.ConnectorSyncOutput{}, c, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		if in.From, err = fromItems(ctx, cwm, carrier, c); err != nil {
			return api.ConnectorSyncOutput{}, c, err
		}
	}
	out, err := call[api.ConnectorSyncOutput](ctx, c.Sync, c.SyncTimeout(), in)
	return out, c, err
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// anchorItem records one item as a source-document under the source: its
// main file as written, hashed by the anchor computation, and what the item
// brings - its kind, its data file checked against the
// data-type its connector declares for that kind, its text file.
func (s *Service) anchorItem(ctx context.Context, root, module, sourceID, folder string, it api.ConnectorItem, decl api.ConnectorDecl) (string, bool, error) {
	main := it.MainFile()
	if main == "" {
		return "", false, errors.New("the item names no file")
	}
	if err := it.CheckNames(); err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(folder), filepath.FromSlash(main)))
	if err != nil {
		return "", false, err
	}
	dataType := decl.Items[it.Kind]
	if it.Data != "" {
		if err := checkItemData(ctx, filepath.Join(root, filepath.FromSlash(folder), filepath.FromSlash(it.Data)), dataType); err != nil {
			return "", false, err
		}
	} else {
		dataType = ""
	}
	stem := main
	if low := strings.ToLower(main); strings.HasSuffix(low, api.ItemDataSuffix) {
		stem = main[:len(main)-len(api.ItemDataSuffix)]
	} else {
		stem = strings.TrimSuffix(main, path.Ext(main))
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(stem), "-"), "-")
	id := cwmapi.QualifyObjectID(module, localID(sourceID)+"-"+slug)
	uri := it.URI
	if uri == "" {
		uri = "file://./" + path.Join(folder, main)
	}
	name := it.Title
	if name == "" {
		name = main
	}
	in := api.IngestInput{SourceDocID: id, SourceURI: uri, Parent: sourceID, Name: name,
		NewContent: string(raw), IngestedBy: "sources/source-sync", Generated: it.Generated, Generator: it.Generator,
		ItemKey: it.Key, ItemKind: it.Kind, DataType: dataType}
	if it.Data != "" {
		in.DataFile = path.Join(folder, it.Data)
	}
	if it.Text != "" {
		in.TextFile = path.Join(folder, it.Text)
	}
	got, err := s.Ingest(ctx, in)
	if err != nil {
		return "", false, err
	}
	return id, got.Changed, nil
}

// recordSource writes the facts a sync learnt on the source.
func (s *Service) recordSource(ctx context.Context, cwm cwmapi.CWMService, module, id string, f api.InformationSourceFields) error {
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	obj := got.Object
	obj.SetFieldsJSON(raw)
	_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: obj, Revision: got.Revision})
	return err
}

// moveSource moves the source to the state a sync ended in, when it is not
// there already.
func (s *Service) moveSource(ctx context.Context, cwm cwmapi.CWMService, id, to string) error {
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to,
		By: cwmapi.Actor{ID: "sources", Kind: cwmapi.ActorAgent}, Note: "sources/source-sync"})
	if err != nil && strings.Contains(err.Error(), "no transition from "+to+" to "+to) {
		return nil
	}
	return err
}

// FolderSync is the local-folder connector: the files of location.path
// (absolute, or relative to the project) matching selection.include are
// copied into dest, each answered as an item.
func (s *Service) FolderSync(ctx context.Context, in api.ConnectorSyncInput) (api.ConnectorSyncOutput, error) {
	from, err := folderOf(in.Location, in.Root)
	if err != nil {
		return api.ConnectorSyncOutput{}, err
	}
	// Items picked one by one: only the files they bring.
	var picked map[string]bool
	if items := api.SelectedItems(in.Selection); items != nil {
		if picked, err = pickedFiles(from, items); err != nil {
			return api.ConnectorSyncOutput{}, fmt.Errorf("sources: read %s: %w", from, err)
		}
	}
	var include []string
	if v, ok := in.Selection["include"].([]any); ok {
		for _, x := range v {
			if p, ok := x.(string); ok && p != "" {
				include = append(include, p)
			}
		}
	}
	out := api.ConnectorSyncOutput{Items: []api.ConnectorItem{}}
	err = filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != from {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(from, p)
		rel = filepath.ToSlash(rel)
		if len(include) > 0 && !matchesAny(include, rel) {
			return nil
		}
		if picked != nil && !picked[rel] {
			return nil
		}
		if err := copyFile(p, filepath.Join(in.Dest, filepath.FromSlash(rel))); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", rel, err))
			return nil
		}
		out.Items = append(out.Items, api.ConnectorItem{Path: rel, URI: "file:///" + strings.TrimPrefix(filepath.ToSlash(p), "/"), Title: path.Base(rel)})
		return nil
	})
	if err != nil {
		return api.ConnectorSyncOutput{}, fmt.Errorf("sources: read %s: %w", from, err)
	}
	return out, nil
}

// localID is the part of a module-qualified id after the module.
func localID(id string) string {
	if _, local, err := cwmapi.SplitObjectID(id); err == nil {
		return local
	}
	return id
}

func matchesAny(globs []string, rel string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, rel); ok {
			return true
		}
		if ok, _ := path.Match(g, path.Base(rel)); ok {
			return true
		}
	}
	return false
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	r, err := os.Open(from)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// checkItemData reads an item's data file and checks it against the
// data-type its connector declares for the item's kind: JSON
// always, shaped by that data-type when there is one. A process with no
// SchemaService keeps the data unchecked beyond JSON.
func checkItemData(ctx context.Context, file, dataType string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if !json.Valid(raw) {
		return errors.New("its data file is no JSON")
	}
	if dataType == "" {
		return nil
	}
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil || schemas == nil {
		return nil
	}
	res, err := schemas.ValidateAgainst(ctx, schemaapi.ValidateAgainstInput{URI: dataType, Body: raw})
	if err != nil {
		return fmt.Errorf("its data cannot be checked against %s: %w", dataType, err)
	}
	if res.Valid {
		return nil
	}
	var why []string
	for _, d := range res.Diagnostics {
		if string(d.Severity) == "warning" {
			continue
		}
		if d.Path != "" {
			why = append(why, d.Path+": "+d.Message)
		} else {
			why = append(why, d.Message)
		}
	}
	return fmt.Errorf("its data does not fit %s: %s", dataType, strings.Join(why, " ; "))
}
