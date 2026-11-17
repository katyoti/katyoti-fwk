// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// OWNED NAMED FILES live with their object: a named file declared `owned:
// true` (memory, comments) is moved when a rename moves its path, and
// deleted with the object. A file that is not owned (a module's concepts page
// a capability points at) is only named by the object, never carried or
// removed by it.

// ownedFilePaths is every owned named file in force on the object, by
// (capability, name), as an absolute path. Read while the object is indexed.
func (s *Service) ownedFilePaths(ctx stdctx.Context, id string) map[string]string {
	obj, env, capIDs, err := s.namedFileContext(ctx, id)
	if err != nil {
		return nil
	}
	root, _, err := s.objectFileRoot(id)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, e := range s.effectiveNamedFiles(obj, capIDs) {
		if !e.decl.Owned || (e.capability != "" && !slices.Contains(capIDs, e.capability)) {
			continue
		}
		if p, err := resolveNamedFilePath(e.decl, env); err == nil {
			out[namedFileKey(e.capability, e.name)] = filepath.Join(root, filepath.FromSlash(p))
		}
	}
	return out
}

// carryOwnedFiles moves each owned file from its old path to the new one, when
// the rename changed it. A file the new object would overwrite is refused.
func carryOwnedFiles(from, to map[string]string) error {
	for k, src := range from {
		dst, ok := to[k]
		if !ok || dst == src {
			continue
		}
		if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("%s already exists — the moved object's file would overwrite it", dst)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// removeOwnedFiles deletes the owned files of a deleted object.
func removeOwnedFiles(paths map[string]string) {
	for _, p := range paths {
		_ = os.Remove(p) // absent is the common case
	}
}
