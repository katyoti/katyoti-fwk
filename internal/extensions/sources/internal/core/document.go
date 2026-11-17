// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// documentFields are the source-document fields its detail reads.
type documentFields struct {
	SourceURI       string `json:"source_uri"`
	ContentSnapshot string `json:"content_snapshot"`
	IngestedAt      string `json:"ingested_at"`
	ItemKey         string `json:"item_key"`
	ItemKind        string `json:"item_kind"`
	DataFile        string `json:"data_file"`
	DataType        string `json:"data_type"`
	TextFile        string `json:"text_file"`
}

// DocumentDetail opens a source-document: what its pane's
// item-detail node draws. A document a sync brought under an
// information source with its item's key is opened through that source's
// fetch - the item as the connector answers it now; an anchor with no key
// (sources/Ingest), one whose source's connector declares no fetch (a LeanIX
// item: nothing failed, nothing said), or one whose fetch fails - said in
// note, never an error page -, answers what is stored: its text file or its snapshot, its data
// file and data-type, the item from its name, kind and source URI. used lists
// the objects derived from it, each with its name and type.
func (s *Service) DocumentDetail(ctx context.Context, in api.DocumentDetailInput) (api.DocumentDetailOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.DocumentDetailOutput{}, fmt.Errorf("%w: sources.DocumentDetail: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.DocumentDetailOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.DocumentDetailOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.DocumentDetailOutput{}, err
	}
	doc := got.Object
	if doc.Type != api.SourceDocumentType {
		return api.DocumentDetailOutput{}, fmt.Errorf("%w: %q is a %s, not a %s", cwmapi.ErrInvalidInput, in.ID, doc.Type, api.SourceDocumentType)
	}
	var f documentFields
	if raw := doc.FieldsJSON(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &f); err != nil {
			return api.DocumentDetailOutput{}, fmt.Errorf("sources: %s fields: %w", in.ID, err)
		}
	}

	var out api.DocumentDetailOutput
	opened := false
	if f.ItemKey != "" && doc.Parent != "" && s.opensItems(ctx, cwm, doc.Parent) {
		fetched, err := s.SourceFetch(ctx, api.SourceFetchInput{ID: doc.Parent, Key: f.ItemKey})
		if err == nil {
			out = api.DocumentDetailOutput{Item: fetched.Item, Data: fetched.Data, DataType: fetched.DataType, Text: fetched.Text}
			opened = true
		} else {
			out.Note = "La source n'a pas pu ouvrir l'élément, ce qui en est gardé est montré : " + err.Error()
		}
	}
	if !opened {
		stored(ctx, cwm, module, doc, f, &out)
	}

	ids, err := derivedFrom(ctx, cwm, in.ID)
	if err != nil {
		return api.DocumentDetailOutput{}, fmt.Errorf("sources: the objects derived from %s: %w", in.ID, err)
	}
	out.Used = []api.DocumentUse{}
	for _, id := range ids {
		use := api.DocumentUse{ID: id}
		if m, err := cwmapi.ModuleOfObjectID(id); err == nil {
			if o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: id}); err == nil {
				use.Name, use.Type = o.Object.Name, o.Object.Type
			}
		}
		out.Used = append(out.Used, use)
	}
	return out, nil
}

// opensItems says whether id is an information source whose connector opens
// an item - one declaring no fetch keeps only what its sync stored. A
// connector that cannot be read is tried, its fetch failing with why.
func (s *Service) opensItems(ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	f, _, err := sourceFields(ctx, cwm, id)
	if err != nil {
		return false
	}
	c, err := s.connector(ctx, f.Connector)
	return err != nil || c.Fetch != ""
}

// stored fills a document's detail from what it keeps: the item from its
// name, kind and source URI; its text file (else its snapshot); its data file
// and data-type - the files read inside the project only.
func stored(ctx context.Context, cwm cwmapi.CWMService, module string, doc cwmapi.Object, f documentFields, out *api.DocumentDetailOutput) {
	key := f.ItemKey
	if key == "" {
		key = doc.ID
	}
	out.Item = api.PickItem{Key: key, Title: doc.Name, Kind: f.ItemKind, URI: f.SourceURI, Modified: f.IngestedAt}
	root := projectRoot(ctx, cwm, module)
	if raw, ok := projectFile(root, f.TextFile); ok {
		out.Text = string(raw)
	} else {
		out.Text = f.ContentSnapshot
	}
	if raw, ok := projectFile(root, f.DataFile); ok {
		var data map[string]any
		if json.Unmarshal(raw, &data) == nil && len(data) > 0 {
			out.Data, out.DataType = data, f.DataType
		}
	}
}

// projectFile reads a file named relative to the project root, refusing one
// outside it.
func projectFile(root, rel string) ([]byte, bool) {
	if root == "" || strings.TrimSpace(rel) == "" || filepath.IsAbs(rel) {
		return nil, false
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	if r, err := filepath.Rel(root, full); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return nil, false
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, false
	}
	return raw, true
}
