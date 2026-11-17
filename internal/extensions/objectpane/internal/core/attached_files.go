// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// AttachedFiles answers the files an object attaches — its `attaches` edges,
// paths in its module, and its own named files that exist — for the Files panel. The edge is the only link: no
// file is an object, and nothing is stored here. Each file is stat'ed through
// CWM's module file API (rooted at the module, `..` refused), so the panel
// reads only what the object's module holds. The id is bare or qualified:
// every read goes to the project holding the object.
func (s *Service) AttachedFiles(ctx context.Context, in api.AttachedFilesInput) (api.AttachedFilesOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.AttachedFilesOutput{}, fmt.Errorf("%w: attached-files: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.AttachedFilesOutput{}, fmt.Errorf("object-pane: resolve CWMService: %w", err)
	}
	return attachedFiles(ctx, cwm, in)
}

// fileStore is the part of CWM the Files source reads.
type fileStore interface {
	EdgeList(context.Context, cwmapi.EdgeListInput) (cwmapi.EdgeListOutput, error)
	ObjectFileStat(context.Context, cwmapi.ObjectFileStatInput) (cwmapi.ObjectFileStatOutput, error)
	ObjectNamedFileList(context.Context, cwmapi.ObjectNamedFileListInput) (cwmapi.ObjectNamedFileListOutput, error)
}

func attachedFiles(ctx context.Context, cwm fileStore, in api.AttachedFilesInput) (api.AttachedFilesOutput, error) {
	// The module's own folder, as an edge names it (`modules/<m>`): an edge's
	// project path minus this prefix is the module-relative path.
	root, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: in.ID, Path: "."})
	if err != nil {
		return api.AttachedFilesOutput{}, err
	}
	prefix := strings.TrimSuffix(root.ProjectPath, "/") + "/"
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: in.ID, Kind: cwmapi.EdgeKindAttaches, Direction: cwmapi.DirectionOut})
	if err != nil {
		return api.AttachedFilesOutput{}, err
	}
	// Edges carry bare ends: the object's own id, bare, is what they name
	// (the qualified in.ID routed the reads to its project).
	_, bare, err := cwmapi.SplitReference(in.ID)
	if err != nil {
		return api.AttachedFilesOutput{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(bare)
	out := api.AttachedFilesOutput{ID: in.ID, Module: module, Files: []api.AttachedFile{}}
	seen := map[string]bool{}
	for _, e := range edges.Edges {
		if e.From != bare || seen[e.To] {
			continue
		}
		seen[e.To] = true
		to := path.Clean(strings.ReplaceAll(e.To, `\`, "/"))
		f := api.AttachedFile{ProjectPath: e.To, Name: path.Base(to), Kind: string(e.Kind), MediaType: render.MediaTypeForPath(to)}
		rel, inModule := strings.CutPrefix(to, prefix)
		if !inModule {
			// Not in the object's module: listed, never read through it.
			f.Path, f.Folder, f.Outside = to, path.Dir(to), true
			out.Files = append(out.Files, f)
			continue
		}
		f.Path = rel
		if d := path.Dir(rel); d != "." {
			f.Folder = d
		}
		if st, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: in.ID, Path: rel}); err == nil && !st.Entry.IsDir {
			f.SizeBytes = st.Entry.SizeBytes
		} else {
			f.Missing = true
		}
		out.Files = append(out.Files, f)
	}
	// The object's OWN named files that exist (declared on it at runtime — a
	// feedback request's screenshot): listed beside the edges, once per path,
	// the name kept. A type's or a capability's files are its machinery (a
	// record, a manifest), shown by their own panels, never here.
	named, err := cwm.ObjectNamedFileList(ctx, cwmapi.ObjectNamedFileListInput{ID: in.ID})
	if err != nil {
		return api.AttachedFilesOutput{}, err
	}
	for _, nf := range named.Files {
		rel := path.Clean(strings.ReplaceAll(nf.Path, `\`, "/"))
		if nf.Source != cwmapi.NamedFileFromObject || !nf.Exists || nf.Error != "" || seen[prefix+rel] {
			continue
		}
		seen[prefix+rel] = true
		f := api.AttachedFile{Path: rel, ProjectPath: prefix + rel, Name: path.Base(rel), Kind: api.NamedFileKind,
			Named: nf.Name, MediaType: nf.Media, SizeBytes: nf.SizeBytes}
		if f.MediaType == "" {
			f.MediaType = render.MediaTypeForPath(rel)
		}
		if d := path.Dir(rel); d != "." {
			f.Folder = d
		}
		out.Files = append(out.Files, f)
	}
	sort.SliceStable(out.Files, func(i, j int) bool {
		a, b := out.Files[i], out.Files[j]
		if a.Folder != b.Folder {
			return a.Folder < b.Folder
		}
		return a.Name < b.Name
	})
	out.Selected = selectedFile(out.Files, in.File, prefix)
	return out, nil
}

// selectedFile is the file shown first: the one asked (module- or
// project-relative), else an index or a README, else the first readable one.
func selectedFile(files []api.AttachedFile, asked, prefix string) string {
	asked = strings.TrimSpace(strings.ReplaceAll(asked, `\`, "/"))
	if asked != "" {
		asked = strings.TrimPrefix(path.Clean(asked), prefix)
	}
	var readable []api.AttachedFile
	for _, f := range files {
		if !f.Missing && !f.Outside {
			readable = append(readable, f)
		}
	}
	for _, f := range readable {
		if asked != "" && f.Path == asked {
			return f.Path
		}
	}
	for _, f := range readable {
		if n := strings.ToLower(f.Name); n == "index.md" || n == "readme.md" {
			return f.Path
		}
	}
	if len(readable) > 0 {
		return readable[0].Path
	}
	return ""
}
