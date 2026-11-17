// SPDX-License-Identifier: Apache-2.0

// Package core is the sources service: Ingest, Reanchor and Stale over CWM's
// object API and the anchor computation.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/internal/anchor"
)

// Service implements api.SourcesService; CWM is resolved from the context.
type Service struct {
	anchor *anchor.Service
}

// New returns the service with the `file` fetcher registered.
func New() *Service { return &Service{anchor: anchor.New()} }

// Reanchor reads the source-document through CWM, recomputes its anchor,
// writes the recorded fields back through ObjectUpdate (so the hooks run and
// the change is an event) and reports the objects carrying a `derived-from`
// edge to it when the section drifted or disappeared.
func (s *Service) Reanchor(ctx context.Context, in api.ReanchorInput) (api.ReanchorOutput, error) {
	if in.SourceDocID == "" {
		return api.ReanchorOutput{}, fmt.Errorf("%w: source_doc_id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ReanchorOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.SourceDocID)
	if err != nil {
		return api.ReanchorOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.SourceDocID})
	if err != nil {
		return api.ReanchorOutput{}, err
	}
	doc := got.Object
	if doc.Type != api.SourceDocumentType {
		return api.ReanchorOutput{}, fmt.Errorf("%w: %q is a %s, not a %s", cwmapi.ErrInvalidInput, in.SourceDocID, doc.Type, api.SourceDocumentType)
	}
	res, err := s.anchor.Reanchor(withProjectRoot(ctx, cwm, module), doc.FieldsJSON(), in.NewContent, in.UpdateOnDrift)
	if err != nil {
		return api.ReanchorOutput{}, err
	}
	if _, err := withinBudget(res.Body, func(fields json.RawMessage) error {
		doc.SetFieldsJSON(fields)
		_, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: doc, Revision: got.Revision})
		return err
	}); err != nil {
		return api.ReanchorOutput{}, err
	}

	out := api.ReanchorOutput{OldHash: res.OldHash, NewHash: res.NewHash, Status: res.Status}
	switch res.AnchorStatus {
	case anchor.AnchorStatusStale, anchor.AnchorStatusMissing:
		ids, err := derivedFrom(ctx, cwm, in.SourceDocID)
		if err != nil {
			return api.ReanchorOutput{}, err
		}
		if res.AnchorStatus == anchor.AnchorStatusStale {
			out.StaleItemIDs = ids
		} else {
			out.MissingItemIDs = ids
		}
	}
	return out, nil
}

// Ingest reads the source section, anchors it with the computation Reanchor
// uses, and creates the source-document — or, when it exists, records the new
// anchor on it — through ObjectCreate / ObjectUpdate.
func (s *Service) Ingest(ctx context.Context, in api.IngestInput) (api.IngestOutput, error) {
	if in.SourceDocID == "" || in.SourceURI == "" {
		return api.IngestOutput{}, fmt.Errorf("%w: source_doc_id and source_uri required", cwmapi.ErrInvalidInput)
	}
	module, err := cwmapi.ModuleOfObjectID(in.SourceDocID)
	if err != nil {
		return api.IngestOutput{}, err
	}
	if module == in.SourceDocID {
		return api.IngestOutput{}, fmt.Errorf("%w: source_doc_id %q must be module-qualified (module.local)", cwmapi.ErrInvalidInput, in.SourceDocID)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.IngestOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.SourceDocID})
	exists := err == nil
	switch {
	case exists && got.Object.Type != api.SourceDocumentType:
		return api.IngestOutput{}, fmt.Errorf("%w: %q is a %s, not a %s", cwmapi.ErrInvalidInput, in.SourceDocID, got.Object.Type, api.SourceDocumentType)
	case err != nil && !errors.Is(err, cwmapi.ErrNotFound):
		return api.IngestOutput{}, err
	}
	var raw []byte
	if exists {
		raw = got.Object.FieldsJSON()
	}
	res, err := s.anchor.Ingest(withProjectRoot(ctx, cwm, module), raw, in.SourceURI, in.SectionPath, in.NewContent, in.IngestedBy)
	if err != nil {
		return api.IngestOutput{}, err
	}
	if res.Body, err = withGeneratorMark(res.Body, in.Generated, in.Generator); err != nil {
		return api.IngestOutput{}, err
	}
	if res.Body, err = withItemFiles(res.Body, in); err != nil {
		return api.IngestOutput{}, err
	}
	out := api.IngestOutput{SourceDocID: in.SourceDocID, Created: !exists, ContentHash: res.Hash}
	if exists {
		var prev struct {
			ContentHash string `json:"content_hash"`
		}
		_ = json.Unmarshal(raw, &prev)
		out.Changed = prev.ContentHash != "" && prev.ContentHash != res.Hash
		doc := got.Object
		fields, err := withinBudget(res.Body, func(fields json.RawMessage) error {
			doc.SetFieldsJSON(fields)
			_, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: doc, Revision: got.Revision})
			return err
		})
		if err != nil {
			return api.IngestOutput{}, err
		}
		out.SnapshotChars = snapshotChars(fields)
		return out, nil
	}
	parent := in.Parent
	if parent == "" {
		parent = module
	}
	name := in.Name
	if name == "" {
		name = in.SourceURI
		if n := len(in.SectionPath); n > 0 {
			name = in.SectionPath[n-1]
		}
	}
	doc := cwmapi.NewContainerObject(module, in.SourceDocID, api.SourceDocumentType, parent, name, "")
	fields, err := withinBudget(res.Body, func(fields json.RawMessage) error {
		doc.SetFieldsJSON(fields)
		_, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: doc})
		return err
	})
	if err != nil {
		return api.IngestOutput{}, err
	}
	out.SnapshotChars = snapshotChars(fields)
	return out, nil
}

// maxSnapshotCuts bounds the cuts one write may take to fit its budget.
const maxSnapshotCuts = 8

// withinBudget writes the source-document's fields, cutting the snapshot
// while CWM refuses the write as over the type's size budget (max_size_bytes):
// CWM measures the file as it would write it, the refusal says by how much,
// and the snapshot — the one field of unbounded length — gives the bytes back.
// It answers the fields written.
func withinBudget(fields json.RawMessage, write func(json.RawMessage) error) (json.RawMessage, error) {
	for cuts := 0; ; cuts++ {
		err := write(fields)
		if err == nil || !errors.Is(err, cwmapi.ErrBudgetExceeded) || cuts == maxSnapshotCuts {
			return fields, err
		}
		over := 0 // unknown: the refusal came without its numbers
		var be *cwmapi.SizeBudgetError
		if errors.As(err, &be) {
			over = be.Over()
		}
		next, ok, cerr := anchor.CutSnapshot(fields, over)
		if cerr != nil {
			return fields, cerr
		}
		if !ok {
			return fields, err // the rest of the document is over budget: nothing to cut
		}
		fields = next
	}
}

// snapshotChars is the length, in characters, of the snapshot the fields carry.
func snapshotChars(fields json.RawMessage) int {
	body, err := anchor.DecodeBody(fields)
	if err != nil {
		return 0
	}
	return utf8.RuneCountInString(body.ContentSnapshot)
}

// Stale lists the module's source-documents whose anchor_status is stale or
// missing, each with the objects carrying a `derived-from` edge to it.
func (s *Service) Stale(ctx context.Context, in api.StaleInput) (api.StaleOutput, error) {
	if in.Module == "" {
		return api.StaleOutput{}, fmt.Errorf("%w: module required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.StaleOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.SourceDocumentType})
	if err != nil {
		return api.StaleOutput{}, err
	}
	out := api.StaleOutput{Anchors: []api.StaleAnchor{}}
	for _, listed := range list.Objects {
		// The list carries the resident metadata; the fields are read per object.
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: in.Module, ID: listed.ID})
		if err != nil {
			return api.StaleOutput{}, err
		}
		doc := got.Object
		body, err := anchor.DecodeBody(doc.FieldsJSON())
		if err != nil {
			return api.StaleOutput{}, fmt.Errorf("sources: %s: %w", doc.ID, err)
		}
		if body.AnchorStatus != anchor.AnchorStatusStale && body.AnchorStatus != anchor.AnchorStatusMissing {
			continue
		}
		ids, err := derivedFrom(ctx, cwm, doc.ID)
		if err != nil {
			return api.StaleOutput{}, err
		}
		out.Anchors = append(out.Anchors, api.StaleAnchor{SourceDocID: doc.ID, AnchorStatus: body.AnchorStatus,
			SourceURI: body.SourceURI, SectionPath: body.SectionPath, DerivedItemIDs: ids})
	}
	sort.Slice(out.Anchors, func(i, j int) bool { return out.Anchors[i].SourceDocID < out.Anchors[j].SourceDocID })
	return out, nil
}

// derivedFrom returns the ids of the objects carrying a `derived-from` edge to
// the document, sorted.
func derivedFrom(ctx context.Context, cwm cwmapi.CWMService, docID string) ([]string, error) {
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: docID, Kind: api.DerivedFromEdge, Direction: cwmapi.DirectionIn})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range edges.Edges {
		ids = append(ids, e.From)
	}
	sort.Strings(ids)
	return ids, nil
}

// withProjectRoot puts the project root on the context, for the file fetcher.
// When it cannot be found the context carries none and the file fetcher
// refuses to read, saying so.
func withProjectRoot(ctx context.Context, cwm cwmapi.CWMService, module string) context.Context {
	if root := projectRoot(ctx, cwm, module); root != "" {
		return anchor.WithProjectRoot(ctx, root)
	}
	return ctx
}

// projectRoot is the project root CWM answers with the module's resolution.
func projectRoot(ctx context.Context, cwm cwmapi.CWMService, module string) string {
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	if err != nil {
		return ""
	}
	return res.ProjectRoot
}

// withGeneratorMark records on an anchor's fields whether its document
// carries a generator's mark (generated, generator), so a carrier's sources
// show which inputs were generated; an unmarked document has neither.
// withItemFiles records on an anchor what its item brings:
// the key of the item it was brought for, its kind, its data file and
// data-type, its text file - each set afresh, so a file the item no longer
// brings leaves the anchor.
func withItemFiles(body json.RawMessage, in api.IngestInput) (json.RawMessage, error) {
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("sources: the anchor's fields: %w", err)
	}
	for k, v := range map[string]string{"item_key": in.ItemKey, "item_kind": in.ItemKind, "data_file": in.DataFile, "data_type": in.DataType, "text_file": in.TextFile} {
		delete(fields, k)
		if v != "" {
			fields[k] = v
		}
	}
	return json.Marshal(fields)
}

func withGeneratorMark(body json.RawMessage, generated bool, generator string) (json.RawMessage, error) {
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("sources: the anchor's fields: %w", err)
	}
	delete(fields, "generated")
	delete(fields, "generator")
	if generated {
		fields["generated"] = true
		if generator != "" {
			fields["generator"] = generator
		}
	}
	return json.Marshal(fields)
}
