// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// AN OBJECT'S FILES ARE ITS MODULE'S FILES (decided 2026-09-25).
//
// The module is the self-contained unit (G1), so the file root is the module
// folder and every path is relative to it. The node verbs these replace wrote
// into the node's own folder under `.katyoti` — recreatable state, gitignored,
// present only on the machine that wrote it. A module whose attachments exist
// in one person's workspace is the opposite of self-contained.
//
// What is NOT in git is not a smaller file here: it is a LINK, an edge whose
// kind declares `target: uri`. Two shapes, no third.
//
// The project object belongs to no module: its file root is its committed
// space at the project root, `<project>/.ktw/` (api.ProjectFilesFolder).

// objectFileRoot resolves the module folder an object's files live in.
//
// It takes an OBJECT id rather than a module name on purpose: the module is
// derived from the id, which is the one place module membership is stated, so
// a caller cannot name a module its object does not belong to.
func (s *Service) objectFileRoot(id string) (root, module string, err error) {
	if strings.TrimSpace(id) == "" {
		return "", "", fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	if err := s.index().ensureLoaded(); err != nil {
		return "", "", err
	}
	if _, ok := s.index().get(id); !ok {
		return "", "", api.ObjectNotFound(id)
	}
	// THE INDEX'S ANSWER, not the id's. A module object's id is BARE (`cwm`),
	// so splitting on the dot would deny a module its own folder; the scan
	// recorded which folder every object came from.
	module, _ = s.index().moduleOf(id)
	if module == "" {
		// THE PROJECT OBJECT belongs to no module: its folder is the project
		// root, and its files live in its committed space there,
		// `<project>/.ktw/` (identity-and-index.md §7) — never scanned for
		// objects, committed like a module's planning space.
		return filepath.Join(s.Config.ObjectsRoot(), api.ProjectFilesFolder), "", nil
	}
	return filepath.Join(s.Config.ObjectsRoot(), s.modulesDirName(), module), module, nil
}

// objectFileAbs resolves a module-relative path to an absolute one that is
// guaranteed to stay inside the module folder.
//
// AN OBJECT FILE IS NOT REACHABLE THROUGH THIS API. `.cwm.yaml` and `.cwm.md`
// are the object store's own files: they have a verb each (ObjectCreate /
// Update / Delete) that validates types, capabilities and edges. Letting a
// byte-level write past that would be a second write path into the same files
// with none of the checks — which is exactly how an object gets a shape
// nothing declared.
func (s *Service) objectFileAbs(id, rel string) (abs, root, module string, err error) {
	root, module, err = s.objectFileRoot(id)
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(rel) == "" {
		return "", "", "", fmt.Errorf("%w: path required", api.ErrInvalidInput)
	}
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".." {
			return "", "", "", fmt.Errorf("%w: path must not contain '..'", api.ErrInvalidInput)
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(rel)), "/")
	if isObjectFileName(clean) {
		return "", "", "", fmt.Errorf("%w: %q is an object file — write it through ObjectCreate / ObjectUpdate, which check what a byte write cannot", api.ErrInvalidInput, clean)
	}
	abs = filepath.Join(root, filepath.FromSlash(clean))
	// Belt and braces after the clean: a platform edge case must not escape.
	r, rerr := filepath.Rel(root, abs)
	if rerr != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("%w: path escapes the module folder", api.ErrInvalidInput)
	}
	return abs, root, module, nil
}

// isObjectFileName reports whether a path names an object file.
func isObjectFileName(p string) bool {
	return strings.HasSuffix(p, api.ObjectFileSuffix) || strings.HasSuffix(p, api.ObjectFileSuffixMD)
}

// projectRelPath is the file as a `target: path` edge stores it — relative to
// the PROJECT, because that is what edge validation resolves against.
// The project object (module "") files under `.ktw/` at the project root.
func (s *Service) projectRelPath(module, rel string) string {
	if module == "" {
		return path.Join(api.ProjectFilesFolder, filepath.ToSlash(rel))
	}
	return path.Join(s.modulesDirName(), module, filepath.ToSlash(rel))
}

// ObjectFileRead returns bytes attached to an object's module.
func (s *Service) ObjectFileRead(_ stdctx.Context, in api.ObjectFileReadInput) (api.ObjectFileReadOutput, error) {
	abs, _, _, err := s.objectFileAbs(in.ID, in.Path)
	if err != nil {
		return api.ObjectFileReadOutput{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return api.ObjectFileReadOutput{}, fmt.Errorf("%w: %s", api.ErrNotFound, in.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return api.ObjectFileReadOutput{}, err
	}
	out := api.ObjectFileReadOutput{SizeBytes: info.Size()}
	if in.Offset > 0 {
		if in.Offset >= len(data) {
			data = nil
		} else {
			data = data[in.Offset:]
		}
		out.IsPartial = true
	}
	if in.Limit > 0 && in.Limit < len(data) {
		data = data[:in.Limit]
		out.IsPartial = true
	}
	out.Content = data
	if in.WithMeta {
		out.Meta = readSidecar(abs)
	}
	return out, nil
}

// ObjectFileWrite writes bytes into the object's module.
func (s *Service) ObjectFileWrite(ctx stdctx.Context, in api.ObjectFileWriteInput) (api.ObjectFileWriteOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectFileWrite"); err != nil {
		return api.ObjectFileWriteOutput{}, err
	}
	abs, _, module, err := s.objectFileAbs(in.ID, in.Path)
	if err != nil {
		return api.ObjectFileWriteOutput{}, err
	}
	if err := s.checkReactionWrites(ctx, module, "ObjectFileWrite"); err != nil {
		return api.ObjectFileWriteOutput{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return api.ObjectFileWriteOutput{}, err
	}
	if in.Append {
		f, oerr := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if oerr != nil {
			return api.ObjectFileWriteOutput{}, oerr
		}
		if _, werr := f.Write(in.Content); werr != nil {
			_ = f.Close()
			return api.ObjectFileWriteOutput{}, werr
		}
		if cerr := f.Close(); cerr != nil {
			return api.ObjectFileWriteOutput{}, cerr
		}
	} else if err := os.WriteFile(abs, in.Content, 0o644); err != nil {
		return api.ObjectFileWriteOutput{}, err
	}
	if in.Meta != nil {
		mb, merr := yaml.Marshal(in.Meta)
		if merr != nil {
			return api.ObjectFileWriteOutput{}, merr
		}
		if werr := atomicio.WriteAtomic(abs+metaSuffix, mb, 0o644); werr != nil {
			return api.ObjectFileWriteOutput{}, werr
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return api.ObjectFileWriteOutput{}, err
	}
	rel := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(in.Path)), "/")
	return api.ObjectFileWriteOutput{
		Path:        rel,
		ProjectPath: s.projectRelPath(module, rel),
		SizeBytes:   info.Size(),
		ModTime:     info.ModTime(),
	}, nil
}

// ObjectFileList lists a module's files, object files excluded.
//
// THE OBJECT FILES ARE NOT LISTED HERE. They are the object store's, reachable
// through ObjectList, and showing them in a file listing would invite the
// byte-level edit the write path refuses.
func (s *Service) ObjectFileList(_ stdctx.Context, in api.ObjectFileListInput) (api.ObjectFileListOutput, error) {
	root, module, err := s.objectFileRoot(in.ID)
	if err != nil {
		return api.ObjectFileListOutput{}, err
	}
	out := api.ObjectFileListOutput{Module: module, Files: []api.FileEntry{}}
	prefix := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(in.Prefix)), "/")
	if in.Prefix == "" {
		prefix = ""
	}
	err = filepath.Walk(root, func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			if os.IsNotExist(werr) {
				return nil // a module with no files yet is not an error
			}
			return werr
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." {
			return nil
		}
		if info.IsDir() {
			if !in.Recursive && relSlash != "." {
				// Non-recursive: descend only far enough to satisfy the prefix.
				if prefix == "" || !strings.HasPrefix(prefix+"/", relSlash+"/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if isObjectFileName(relSlash) || strings.HasSuffix(relSlash, metaSuffix) {
			return nil
		}
		if prefix != "" && !strings.HasPrefix(relSlash, prefix) {
			return nil
		}
		entry := api.FileEntry{Path: relSlash, SizeBytes: info.Size(), ModTime: info.ModTime()}
		if in.WithMeta {
			entry.Meta = readSidecar(p)
		}
		out.Files = append(out.Files, entry)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return api.ObjectFileListOutput{}, err
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	return out, nil
}

// ObjectFileDelete removes a file from the object's module.
func (s *Service) ObjectFileDelete(ctx stdctx.Context, in api.ObjectFileDeleteInput) (api.ObjectFileDeleteOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectFileDelete"); err != nil {
		return api.ObjectFileDeleteOutput{}, err
	}
	abs, _, module, err := s.objectFileAbs(in.ID, in.Path)
	if err != nil {
		return api.ObjectFileDeleteOutput{}, err
	}
	if err := s.checkReactionWrites(ctx, module, "ObjectFileDelete"); err != nil {
		return api.ObjectFileDeleteOutput{}, err
	}
	if err := os.Remove(abs); err != nil {
		if os.IsNotExist(err) {
			// Idempotent: deleting what is not there is the state asked for.
			return api.ObjectFileDeleteOutput{DeletedPaths: []string{}}, nil
		}
		return api.ObjectFileDeleteOutput{}, err
	}
	deleted := []string{filepath.ToSlash(in.Path)}
	if in.WithMeta {
		if err := os.Remove(abs + metaSuffix); err == nil {
			deleted = append(deleted, filepath.ToSlash(in.Path)+metaSuffix)
		}
	}
	return api.ObjectFileDeleteOutput{DeletedPaths: deleted}, nil
}

// ObjectFileStat describes one file without reading it.
func (s *Service) ObjectFileStat(_ stdctx.Context, in api.ObjectFileStatInput) (api.ObjectFileStatOutput, error) {
	abs, _, module, err := s.objectFileAbs(in.ID, in.Path)
	if err != nil {
		return api.ObjectFileStatOutput{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return api.ObjectFileStatOutput{}, fmt.Errorf("%w: %s", api.ErrNotFound, in.Path)
	}
	rel := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(in.Path)), "/")
	return api.ObjectFileStatOutput{
		Entry: api.FileEntry{
			Path: rel, IsDir: info.IsDir(), SizeBytes: info.Size(),
			ModTime: info.ModTime(), Meta: readSidecar(abs),
		},
		ProjectPath: s.projectRelPath(module, rel),
	}, nil
}
