// SPDX-License-Identifier: Apache-2.0

// Package soldocs places a solution's documents into a project: each markdown
// file, in its language, becomes a CWM document of the project's first module,
// so the GUI shows it like any other document. It writes them through CWM's
// verbs on the project folder (cwm/folder) - validated, indexed - never a file
// of its own.
package soldocs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmfolder "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/folder"
)

// FirstModule is the project's first module folder (sorted).
func FirstModule(project string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(project, "modules"))
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	if err != nil || len(names) == 0 {
		return "", fmt.Errorf("a solution's documents go into a module of the project, and %s has none", project)
	}
	sort.Strings(names)
	return names[0], nil
}

// Placer places documents into one project, opened once through CWM.
type Placer struct {
	module string
	cwm    *cwmfolder.Project
}

// Open opens the project folder (no hub needs to serve it) to place a
// solution's documents into its first module. Close it when done.
func Open(ctx context.Context, project string) (*Placer, error) {
	module, err := FirstModule(project)
	if err != nil {
		return nil, err
	}
	p, err := cwmfolder.Open(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("open the project %s: %w", project, err)
	}
	return &Placer{module: module, cwm: p}, nil
}

// Close releases the project.
func (p *Placer) Close() { p.cwm.Close() }

// Place makes the markdown md (file name, language) a document of the
// project's first module and answers its id: `<module>.<slug>` with `<slug>`
// the file name without extension, then the language. Its title is the
// markdown's first `# ` heading, else the file name. A document placed before
// (the same solution installed again) is updated in place.
func (p *Placer) Place(ctx context.Context, lang, name string, md []byte) (string, error) {
	slug := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	if lang != "" {
		slug += "-" + strings.ToLower(lang)
	}
	body := strings.ReplaceAll(string(md), "\r\n", "\n")
	title := strings.TrimSuffix(name, filepath.Ext(name))
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			break
		}
	}
	id := p.module + "." + slug
	obj := cwmapi.Object{ID: id, Type: "document", Name: title, Parent: p.module,
		Fields: map[string]any{"title": title, "lang": lang}, Body: body}
	got, err := p.cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: p.module, ID: id})
	switch {
	case err == nil:
		if _, err := p.cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: p.module, Object: obj, Revision: got.Revision}); err != nil {
			return "", fmt.Errorf("update the document %s: %w", id, err)
		}
	case errors.Is(err, cwmapi.ErrNotFound):
		if _, err := p.cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: p.module, Object: obj}); err != nil {
			return "", fmt.Errorf("create the document %s: %w", id, err)
		}
	default:
		return "", fmt.Errorf("read the document %s: %w", id, err)
	}
	return id, nil
}
