// SPDX-License-Identifier: Apache-2.0

package core

// « DÉPOSER UN DOCUMENT »: a file a person gives a carrier
// joins its « Documents » source and its knowledge base, saying who
// deposited it.

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// DepositMaxSize is the largest document a deposit takes: the call crosses
// the hub in one message of at most 8 MiB, base64 adding a third - the file
// button's own limit.
const DepositMaxSize = 5 * 1024 * 1024

// Deposit writes the file into the carrier's documents/ folder - under a free
// name when the name is taken, never over another document - picks it in the
// carrier's « Documents » source with who deposited it (
// keeper record), reads it and reindexes the carrier.
func (s *Service) Deposit(ctx context.Context, in api.DepositInput) (api.DepositOutput, error) {
	name := strings.TrimSpace(in.Name)
	if err := depositName(name); err != nil {
		return api.DepositOutput{}, fmt.Errorf("%w: %q: %v", cwmapi.ErrInvalidInput, in.Name, err)
	}
	if base64.StdEncoding.DecodedLen(len(in.Content)) > DepositMaxSize+3 {
		return api.DepositOutput{}, fmt.Errorf("%w: %s est trop volumineux : %d Mo au plus", cwmapi.ErrInvalidInput, name, DepositMaxSize/(1024*1024))
	}
	content, err := base64.StdEncoding.DecodeString(in.Content)
	if err != nil {
		return api.DepositOutput{}, fmt.Errorf("%w: the content of %s is not base64: %v", cwmapi.ErrInvalidInput, name, err)
	}
	if len(content) > DepositMaxSize {
		return api.DepositOutput{}, fmt.Errorf("%w: %s est trop volumineux : %d Mo au plus", cwmapi.ErrInvalidInput, name, DepositMaxSize/(1024*1024))
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.DepositOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	c, err := carrierOf(ctx, cwm, in.ID)
	if err != nil {
		return api.DepositOutput{}, err
	}
	used, err := s.Use(ctx, api.UseInput{ID: in.ID, Connector: api.DocumentsConnector})
	if err != nil {
		return api.DepositOutput{}, err
	}
	f, module, err := sourceFields(ctx, cwm, used.Source)
	if err != nil {
		return api.DepositOutput{}, err
	}
	folder := path.Join(filepath.ToSlash(c.folder), api.DocumentsFolder)
	dir := filepath.Join(projectRoot(ctx, cwm, c.id), filepath.FromSlash(folder))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return api.DepositOutput{}, fmt.Errorf("sources: the documents folder of %s: %w", in.ID, err)
	}
	before := api.SelectedItems(f.Selection)
	given := name
	name = freeName(dir, name, before)
	// O_EXCL: a document written meanwhile under that name is never replaced.
	w, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return api.DepositOutput{}, fmt.Errorf("sources: deposit %s: %w", name, err)
	}
	_, err = w.Write(content)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return api.DepositOutput{}, fmt.Errorf("sources: deposit %s: %w", name, err)
	}

	f.Location = map[string]any{"path": folder}
	items := append(append([]api.SelectedItem{}, before...), api.SelectedItem{Key: name, Title: name})
	items = keepers(items, before, in.By, time.Now().UTC().Format(time.RFC3339))
	picked := make([]any, 0, len(items))
	now := map[string]bool{}
	for _, it := range items {
		m := map[string]any{"key": it.Key, "title": it.Title}
		keptInto(m, it)
		picked = append(picked, m)
		now[it.Key] = true
	}
	if f.Selection == nil {
		f.Selection = map[string]any{}
	}
	f.Selection[api.SelectionItems] = picked
	if err := s.recordSource(ctx, cwm, module, used.Source, f); err != nil {
		return api.DepositOutput{}, err
	}
	said := name
	if name != given {
		said = fmt.Sprintf("%s (déposé sous %s : le nom était pris)", given, name)
	}
	out := api.DepositOutput{OK: true, Source: used.Source, Document: path.Join(folder, name),
		Message: fmt.Sprintf("%s déposé : il rejoint la source « Documents » et la base de connaissance.", said)}
	if _, err := s.applied(ctx, cwm, used.Source, now, nil); err != nil {
		out.Message = fmt.Sprintf("%s déposé, mais pas encore lu : %v", name, err)
		return out, nil
	}
	if r, err := s.Reindex(ctx, api.ReindexInput{ID: in.ID}); err != nil {
		out.Message += " Base non réindexée : " + err.Error()
	} else if !r.OK {
		out.Message += " " + r.Message
	}
	return out, nil
}

// depositName refuses what is no plain file name on every system: a folder in
// it, a name Windows reserves (CON, NUL.pdf…), a character it refuses.
func depositName(name string) error {
	if name == "" || !filepath.IsLocal(name) || strings.ContainsAny(name, `/\:*?"<>|`) || name != filepath.Base(name) {
		return fmt.Errorf(`no file name - a deposited document is named without a folder or any of / \ : * ? " < > |`)
	}
	for _, r := range name {
		if r < 32 {
			return fmt.Errorf("a control character in the name")
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("a name ending with a dot or a space")
	}
	stem := strings.ToUpper(strings.TrimSpace(strings.SplitN(name, ".", 2)[0]))
	switch stem {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("%s is a name Windows reserves", stem)
	}
	return nil
}

// freeName is name when no document holds it - neither a file in dir nor an
// item of the source - else « stem (2).ext », (3)…: a deposit never replaces
// another.
func freeName(dir, name string, items []api.SelectedItem) string {
	taken := func(n string) bool {
		for _, it := range items {
			if strings.EqualFold(it.Key, n) {
				return true
			}
		}
		_, err := os.Stat(filepath.Join(dir, n))
		return err == nil
	}
	if !taken(name) {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s (%d)%s", stem, i, ext); !taken(n) {
			return n
		}
	}
}

// deposited are the documents deposited on a carrier: its own « Documents »
// source's picked files, each with who deposited it and where it stands.
func deposited(ctx context.Context, cwm cwmapi.CWMService, srcs []api.SourceSummary) []api.DepositedDocument {
	var out []api.DepositedDocument
	for _, src := range srcs {
		if src.Via != "child" || src.Connector != api.DocumentsConnector {
			continue
		}
		f, _, err := sourceFields(ctx, cwm, src.ID)
		if err != nil {
			continue
		}
		folder, _ := f.Location["path"].(string)
		for _, it := range api.SelectedItems(f.Selection) {
			d := api.DepositedDocument{Name: it.Title, Path: path.Join(folder, it.Key), At: it.KeptAt, State: f.States[it.Key].State}
			if d.Name == "" {
				d.Name = it.Key
			}
			where := "pas encore lu"
			switch d.State {
			case api.ItemRead:
				where = "dans la base"
			case api.ItemFailed:
				where = "non lu"
			}
			d.Label = where
			if it.KeptBy != nil {
				d.By = it.KeptBy.ID
				d.Label = "déposé par " + d.By + " · " + where
			}
			out = append(out, d)
		}
	}
	return out
}

// DepositAccept is what the « Déposer un document » button offers to choose.
const DepositAccept = ".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.md,.txt,.csv"

// DocumentsPage is a carrier's « Documents » section as a page region
// - the « Sources » tab and a study's « Schémas et
// documents » hold it: « Déposer un document » hands a file chosen on the
// machine to Deposit with who presses it, then each deposited document says
// who deposited it and whether it is in the base, and opens in place in the
// reader drawer.
func (s *Service) DocumentsPage(ctx context.Context, in api.DocumentsPageInput) (api.ConnectorPanelOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.DocumentsPage: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ConnectorPanelOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	if _, err := carrierOf(ctx, cwm, in.ID); err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	srcs, err := sourcesOf(ctx, cwm, in.ID)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	docs := []any{}
	for _, d := range deposited(ctx, cwm, srcs) {
		docs = append(docs, map[string]any{"name": d.Name, "path": d.Path, "by": d.By, "at": d.At, "state": d.State, "label": d.Label})
	}
	tree := map[string]any{"kind": "section", "size": "md", "title": "Documents",
		"description": "Les fichiers déposés sur l'objet, lus dans sa base de connaissance", "icon": "file-upload", "color": "blue",
		"children": []any{
			map[string]any{"kind": "button", "label": "Déposer un document", "icon": "upload", "size": "xs", "variant": "light",
				"action": map[string]any{"label": "Déposer un document", "uri": "ktw-agents:sources/deposit",
					"args": map[string]any{"id": in.ID, "by": "${person}"}, "file": map[string]any{"accept": DepositAccept}}},
			map[string]any{"kind": "list", "dataField": "$.deposited", "gap": "xs", "empty": "Aucun document déposé.",
				"item": map[string]any{"kind": "hbox", "gap": "sm", "children": []any{
					map[string]any{"kind": "link", "text": "(document)", "dataField": "$.item.name", "size": "sm",
						"action": map[string]any{"label": "Ouvrir le document", "drawer": map[string]any{"file": "${item.path}", "object": in.ID, "title": "${item.name}"}}},
					map[string]any{"kind": "text", "content": "", "dataField": "$.item.label", "size": "xs", "color": "dimmed"},
				}}},
		}}
	return api.ConnectorPanelOutput{DSLTree: tree, Data: map[string]any{"deposited": docs}, Title: "Documents"}, nil
}
