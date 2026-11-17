// SPDX-License-Identifier: Apache-2.0

package core

// « RETIRER »: a source taken off its carrier - what it
// brought leaves the knowledge base.

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// Remove takes a source off its carrier. The carrier's own source: the files
// it mirrored are deleted, its source-documents and then itself archived (its
// history kept), and the carrier reindexed so its documents leave the base
// . A source that only feeds the carrier: the feeds edge goes, the
// source stays.
func (s *Service) Remove(ctx context.Context, in api.RemoveInput) (api.RemoveOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.RemoveOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	id := strings.TrimSpace(in.ID)
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return api.RemoveOutput{}, fmt.Errorf("%w: %q is no object id", cwmapi.ErrInvalidInput, in.ID)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return api.RemoveOutput{}, err
	}
	src := got.Object
	if src.Type != api.InformationSourceType {
		return api.RemoveOutput{}, fmt.Errorf("%w: %q is a %s, not an %s", cwmapi.ErrInvalidInput, id, src.Type, api.InformationSourceType)
	}
	name := src.Name
	if name == "" {
		name = id
	}
	if c := strings.TrimSpace(in.Carrier); c != "" && c != src.Parent {
		if _, err := cwm.EdgeDelete(ctx, cwmapi.EdgeDeleteInput{From: id, To: c, Kind: api.FeedsEdge}); err != nil {
			return api.RemoveOutput{}, fmt.Errorf("sources: unlink %s from %s: %w", id, c, err)
		}
		return api.RemoveOutput{OK: true, Message: fmt.Sprintf("%s n'alimente plus %s.", name, c)}, nil
	}
	st, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: module, Path: "."})
	if err != nil {
		return api.RemoveOutput{}, fmt.Errorf("sources: module %q folder: %w", module, err)
	}
	root := projectRoot(ctx, cwm, module)
	dest := filepath.Join(root, filepath.FromSlash(path.Join(filepath.ToSlash(st.ProjectPath), "input", localID(id))))
	// Its anchors go even when nothing was mirrored yet.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return api.RemoveOutput{}, fmt.Errorf("sources: %s's folder: %w", id, err)
	}
	removed, err := s.dropUnbrought(ctx, cwm, module, id, dest, nil, nil)
	if err != nil {
		return api.RemoveOutput{}, err
	}
	_ = os.RemoveAll(dest)
	if _, err := cwm.ObjectArchive(ctx, cwmapi.ObjectArchiveInput{ID: id}); err != nil {
		return api.RemoveOutput{}, fmt.Errorf("sources: archive %s: %w", id, err)
	}
	out := api.RemoveOutput{OK: true, Removed: len(removed),
		Message: fmt.Sprintf("%s retirée : %d document(s) quittent la base de connaissance.", name, len(removed))}
	if src.Parent != "" {
		if r, err := s.Reindex(ctx, api.ReindexInput{ID: src.Parent}); err != nil {
			out.Message += " Base non réindexée : " + err.Error()
		} else if !r.OK {
			out.Message += " " + r.Message
		}
	}
	return out, nil
}
