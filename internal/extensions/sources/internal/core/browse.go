// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// browseTimeout bounds a connector's browse: one level of candidates.
const browseTimeout = 75 * time.Second

// SourceBrowse lists the candidates of a source's location through its
// connector's browse method.
func (s *Service) SourceBrowse(ctx context.Context, in api.SourceBrowseInput) (api.ConnectorBrowseOutput, error) {
	return s.sourceBrowse(ctx, in, "")
}

// sourceBrowse is SourceBrowse; for a connector reading from another source,
// a typed value - one typed by hand - stands in for what that source kept.
func (s *Service) sourceBrowse(ctx context.Context, in api.SourceBrowseInput, typed string) (api.ConnectorBrowseOutput, error) {
	connector, location, root := in.Connector, in.Location, ""
	var saved []api.SelectedItem
	if in.ID != "" {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.ConnectorBrowseOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		f, module, err := sourceFields(ctx, cwm, in.ID)
		if err != nil {
			return api.ConnectorBrowseOutput{}, err
		}
		connector, location, root = f.Connector, f.Location, projectRoot(ctx, cwm, module)
		saved = api.SelectedItems(f.Selection)
	}
	if connector == "" {
		return api.ConnectorBrowseOutput{}, fmt.Errorf("%w: an information source, or a connector and its location, required", cwmapi.ErrInvalidInput)
	}
	c, err := s.connector(ctx, connector)
	if err != nil {
		return api.ConnectorBrowseOutput{}, err
	}
	if c.Browse == "" {
		return api.ConnectorBrowseOutput{}, fmt.Errorf("%w: the connector %s cannot be browsed: it syncs its whole location", cwmapi.ErrInvalidInput, c.Name)
	}
	req := api.ConnectorBrowseInput{Location: location, Parent: in.Parent, Query: in.Query, Root: root}
	// For a carrier (a picker's object), its own source for the connector
	// gives the location; a connector reading from another source gets what
	// the carrier's source of it kept.
	carrier := in.Object
	if in.ID == "" && in.Object != "" {
		if req.Location, req.Root, err = s.carrierLocation(ctx, in.Object, c.Name); err != nil {
			return api.ConnectorBrowseOutput{}, err
		}
	}
	if c.From != nil {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return api.ConnectorBrowseOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
		}
		if in.ID != "" {
			carrier = carrierOfSource(ctx, cwm, in.ID)
		}
		if req.From, err = fromItems(ctx, cwm, carrier, c); err != nil {
			return api.ConnectorBrowseOutput{}, err
		}
		if typed = strings.TrimSpace(typed); typed != "" {
			req.From = &api.FromItems{Connector: c.From.First(), Attr: c.From.Attr, Values: []string{typed}}
		}
	}
	out, err := call[api.ConnectorBrowseOutput](ctx, c.Browse, browseTimeout, req)
	if err != nil {
		return api.ConnectorBrowseOutput{}, err
	}
	if out.Items == nil {
		out.Items = []api.BrowseItem{}
	}
	// A source's saved choice comes with what it lists: the pick tree opens
	// with it ticked.
	for i, it := range out.Items {
		for _, p := range saved {
			if p.Key == it.Key {
				out.Items[i].Picked, out.Items[i].WithChildren = true, p.WithChildren
			}
		}
	}
	return out, nil
}

// SourceSelect records the items picked on a source, the whole choice; the
// selection's other keys are kept.
func (s *Service) SourceSelect(ctx context.Context, in api.SourceSelectInput) (api.SourceSelectOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.SourceSelectOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, module, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.SourceSelectOutput{}, err
	}
	for _, it := range in.Items {
		if strings.TrimSpace(it.Key) == "" {
			return api.SourceSelectOutput{}, fmt.Errorf("%w: a picked item has no key", cwmapi.ErrInvalidInput)
		}
	}
	picked := keepers(in.Items, append(api.SelectedItems(f.Selection), f.Held...), in.By, time.Now().UTC().Format(time.RFC3339))
	items := make([]any, 0, len(picked))
	for _, it := range picked {
		if strings.TrimSpace(it.Key) == "" {
			return api.SourceSelectOutput{}, fmt.Errorf("%w: a picked item has no key", cwmapi.ErrInvalidInput)
		}
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
	if err := s.recordSource(ctx, cwm, module, in.ID, f); err != nil {
		return api.SourceSelectOutput{}, err
	}
	return api.SourceSelectOutput{OK: true, Items: len(items),
		Message: fmt.Sprintf("%d élément(s) choisi(s) : la prochaine synchronisation n'apporte qu'eux.", len(items))}, nil
}

// sourceFields reads an information source's fields and its module.
func sourceFields(ctx context.Context, cwm cwmapi.CWMService, id string) (api.InformationSourceFields, string, error) {
	var f api.InformationSourceFields
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return f, "", err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return f, "", err
	}
	if got.Object.Type != api.InformationSourceType {
		return f, "", fmt.Errorf("%w: %q is a %s, not an %s", cwmapi.ErrInvalidInput, id, got.Object.Type, api.InformationSourceType)
	}
	if raw := got.Object.FieldsJSON(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &f); err != nil {
			return f, "", fmt.Errorf("sources: %s fields: %w", id, err)
		}
	}
	return f, module, nil
}

// folderOf is the local-folder connector's folder: location.path, absolute
// or relative to root.
func folderOf(location map[string]any, root string) (string, error) {
	from, _ := location["path"].(string)
	if strings.TrimSpace(from) == "" {
		return "", fmt.Errorf("%w: location.path required", cwmapi.ErrInvalidInput)
	}
	if !filepath.IsAbs(from) {
		if root == "" {
			return "", fmt.Errorf("%w: %q is relative and no project root was given", cwmapi.ErrInvalidInput, from)
		}
		from = filepath.Join(root, filepath.FromSlash(from))
	}
	return from, nil
}

// underFolder resolves an item key - a path relative to the folder - refusing
// one that leaves it.
func underFolder(from, key string) (string, error) {
	k := filepath.ToSlash(key)
	if path.IsAbs(k) || filepath.IsAbs(key) {
		return "", fmt.Errorf("%w: %q is not a path under the folder", cwmapi.ErrInvalidInput, key)
	}
	for _, seg := range strings.Split(k, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: %q leaves the folder", cwmapi.ErrInvalidInput, key)
		}
	}
	return filepath.Join(from, filepath.FromSlash(k)), nil
}

// FolderBrowse is the browse of the local-folder and Documents connectors: a
// folder's entries, folders first then files, each by name; with a query,
// every entry below parent whose name holds it. Hidden entries are left out.
// Each entry carries the item roles: its key the path
// relative to the folder, its kind folder or file, its parent folder's key,
// the folders above it as its path, and when it changed. A location with no
// folder yet - a carrier's Documents before anything is deposited - has
// nothing to browse.
func (s *Service) FolderBrowse(_ context.Context, in api.ConnectorBrowseInput) (api.ConnectorBrowseOutput, error) {
	if p, _ := in.Location["path"].(string); strings.TrimSpace(p) == "" {
		return api.ConnectorBrowseOutput{Items: []api.BrowseItem{}}, nil
	}
	from, err := folderOf(in.Location, in.Root)
	if err != nil {
		return api.ConnectorBrowseOutput{}, err
	}
	dir, err := underFolder(from, in.Parent)
	if err != nil {
		return api.ConnectorBrowseOutput{}, err
	}
	item := func(p string, d fs.DirEntry) api.BrowseItem {
		rel, _ := filepath.Rel(from, p)
		rel = filepath.ToSlash(rel)
		it := api.BrowseItem{Key: rel, Title: d.Name(), Kind: "file", URI: "file:///" + strings.TrimPrefix(filepath.ToSlash(p), "/")}
		if up := path.Dir(rel); up != "." {
			it.Parent, it.Path = up, strings.Split(up, "/")
		}
		if info, err := d.Info(); err == nil {
			it.Modified = info.ModTime().UTC().Format(time.RFC3339)
		}
		if d.IsDir() {
			it.Kind = "folder"
			if entries, err := os.ReadDir(p); err == nil {
				for _, e := range entries {
					if !strings.HasPrefix(e.Name(), ".") {
						it.HasChildren = true
						break
					}
				}
			}
		}
		return it
	}
	out := api.ConnectorBrowseOutput{Items: []api.BrowseItem{}}
	if q := strings.ToLower(strings.TrimSpace(in.Query)); q != "" {
		err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == dir {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.Contains(strings.ToLower(d.Name()), q) {
				out.Items = append(out.Items, item(p, d))
			}
			return nil
		})
	} else {
		var entries []fs.DirEntry
		if entries, err = os.ReadDir(dir); err == nil {
			for _, d := range entries {
				if !strings.HasPrefix(d.Name(), ".") {
					out.Items = append(out.Items, item(filepath.Join(dir, d.Name()), d))
				}
			}
		}
	}
	if err != nil {
		return api.ConnectorBrowseOutput{}, fmt.Errorf("sources: browse %s: %w", dir, err)
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if (a.Kind == "folder") != (b.Kind == "folder") {
			return a.Kind == "folder"
		}
		return a.Key < b.Key
	})
	return out, nil
}

// pickedFiles is what a selection's items bring from a folder: each picked
// file; a picked folder's own files, and with its children every file below.
// Keys are paths relative to the folder.
func pickedFiles(from string, items []api.SelectedItem) (map[string]bool, error) {
	files := map[string]bool{}
	for _, it := range items {
		p, err := underFolder(from, it.Key)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", it.Key, err)
		}
		if !st.IsDir() {
			files[filepath.ToSlash(it.Key)] = true
			continue
		}
		err = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if q != p && (strings.HasPrefix(d.Name(), ".") || !it.WithChildren) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(from, q)
			files[filepath.ToSlash(rel)] = true
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
