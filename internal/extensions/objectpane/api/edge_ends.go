// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EdgeEnds names the other ends of the edges of the object holder in the
// project holding it, ready for ReadRowsOf. An edge never crosses a project
// and CWM answers its ends bare, while ReadRowsOf answers each served
// project's object for a bare id: a caller passing the bare ends would list
// another project's homonym. With several projects served each bare id is
// qualified with the holder's project (a qualified holder names it, a bare
// one is read to learn it); with one project the ids are answered unchanged.
// An id already qualified keeps its project.
func EdgeEnds(ctx context.Context, cwm cwmapi.CWMService, holder string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	key, bare, err := cwmapi.SplitReference(holder)
	if err != nil {
		return nil, err
	}
	if key == "" {
		module, err := cwmapi.ModuleOfObjectID(bare)
		if err != nil {
			return nil, err
		}
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: bare})
		if err != nil {
			return nil, err
		}
		key = got.Project
	}
	if key == "" {
		return ids, nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id
		if k, b, err := cwmapi.SplitReference(id); err == nil && k == "" {
			out[i] = cwmapi.QualifiedID(key, b)
		}
	}
	return out, nil
}

// Unresolved counts the ids that no row answers - an id is resolved by the
// row of its object (of its project when qualified), never by subtracting
// counts: a bare id may answer a row per project.
func Unresolved(ids []string, rows []ObjectRow) int {
	n := 0
	for _, id := range ids {
		key, bare, err := cwmapi.SplitReference(id)
		if err != nil {
			n++
			continue
		}
		found := false
		for _, r := range rows {
			if r.ID == bare && (key == "" || r.Project == "" || r.Project == key) {
				found = true
				break
			}
		}
		if !found {
			n++
		}
	}
	return n
}
