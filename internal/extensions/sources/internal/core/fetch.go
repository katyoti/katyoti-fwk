// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// fetchTimeout bounds one item's opening.
const fetchTimeout = 60 * time.Second

// SourceFetch opens one item of a source through its connector's fetch
// : the item by its roles, its data - typed by the data-type
// the connector declares for its kind, named in data_type - and its markdown
// text, either or both. The source's own location, or a connector and a
// location before a source is saved. With a source, only an item the source
// keeps opens: one it picked or holds, one applied, or one
// a sync brought under it as a source-document; any other key is refused,
// saying why. The connector and location form - a picker browsing before a
// source keeps anything - opens any key.
func (s *Service) SourceFetch(ctx context.Context, in api.SourceFetchInput) (api.SourceFetchOutput, error) {
	if strings.TrimSpace(in.Key) == "" {
		return api.SourceFetchOutput{}, fmt.Errorf("%w: sources.SourceFetch: key required", cwmapi.ErrInvalidInput)
	}
	connector, location, root := in.Connector, in.Location, ""
	if in.ID != "" {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.SourceFetchOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		f, module, err := sourceFields(ctx, cwm, in.ID)
		if err != nil {
			return api.SourceFetchOutput{}, err
		}
		if !keepsItem(ctx, cwm, module, in.ID, f, in.Key) {
			return api.SourceFetchOutput{}, fmt.Errorf("%w: la source %s ne garde pas l'élément %q : seul un élément choisi, retenu ou apporté par la source s'ouvre par elle", cwmapi.ErrInvalidInput, in.ID, in.Key)
		}
		connector, location, root = f.Connector, f.Location, projectRoot(ctx, cwm, module)
	}
	if connector == "" {
		return api.SourceFetchOutput{}, fmt.Errorf("%w: an information source, or a connector and its location, required", cwmapi.ErrInvalidInput)
	}
	c, err := s.connector(ctx, connector)
	if err != nil {
		return api.SourceFetchOutput{}, err
	}
	if c.Fetch == "" {
		return api.SourceFetchOutput{}, fmt.Errorf("%w: the connector %s cannot open an item: it declares no fetch", cwmapi.ErrInvalidInput, c.Name)
	}
	got, err := call[api.ConnectorFetchOutput](ctx, c.Fetch, fetchTimeout, api.ConnectorFetchInput{Location: location, Key: in.Key, Root: root})
	if err != nil {
		return api.SourceFetchOutput{}, err
	}
	out := api.SourceFetchOutput{Item: got.Item, Data: got.Data, Text: got.Text}
	if len(got.Data) > 0 {
		out.DataType = c.Items[got.Item.Kind]
	}
	return out, nil
}

// keepsItem says whether the source keeps the item of key: picked or held in
// its choice, applied (its state recorded), or brought by a sync as a
// source-document under it carrying that key (a source read whole has no
// list of items, only what it brought).
func keepsItem(ctx context.Context, cwm cwmapi.CWMService, module, id string, f api.InformationSourceFields, key string) bool {
	for _, it := range append(api.SelectedItems(f.Selection), f.Held...) {
		if it.Key == key {
			return true
		}
	}
	if _, ok := f.States[key]; ok {
		return true
	}
	docs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: id, Type: api.SourceDocumentType})
	if err != nil {
		return false
	}
	for _, d := range docs.Objects {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: d.ID})
		if err != nil {
			continue
		}
		var df struct {
			ItemKey string `json:"item_key"`
		}
		if json.Unmarshal(got.Object.FieldsJSON(), &df) == nil && df.ItemKey == key {
			return true
		}
	}
	return false
}
