// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// catalog_task_data.go — a task as the catalog publishes it, read the way the
// engine reads the file.
//
// The catalog publishes a task's frontmatter as written (the kind's
// `source:` block); the defaults the task parser applies to a file
// (taskcontrib.Parse) are applied by the readers, here, once: every reader of
// a task's data (the summaries, the library, the GUI listing) goes through
// NormalizeTaskBody. The input and output contracts are their own
// contributions (the task's companions), read by TaskContracts.

// NormalizeTaskBody applies to a task's data the defaults the task parser
// applies to its file (mbapi.TaskBody.Normalize).
func NormalizeTaskBody(b *mbapi.TaskBody) { b.Normalize() }

// TaskContracts reads a task's input and output contracts — the data-types
// `<provider>:/data-type/task/<name>/input|output` its contract files are
// published as — each as the DSL document encoded as JSON (the shape the
// engine reads, Task.InputSchema). Empty when the task has none.
func TaskContracts(ctx context.Context, cat mbapi.Catalog, provider, name string) (in, out json.RawMessage, err error) {
	// Which contracts exist is the index's answer: only those are opened.
	dts, err := cat.GetContributionsByType(ctx, contribapi.ResourceTypeDataType)
	if err != nil {
		return nil, nil, err
	}
	published := map[string]bool{}
	for _, c := range dts {
		published[c.ResolvedURI()] = true
	}
	read := func(role mbapi.SchemaRole) (json.RawMessage, error) {
		uri := mbapi.TaskSchemaURI(provider, name, role)
		if uri == "" || !published[uri] {
			return nil, nil
		}
		l, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, uri, "source")
		if err != nil {
			if errors.Is(err, mbapi.ErrSidecarNotInCatalog) || isNotFound(err) {
				return nil, nil // the task declares no such contract
			}
			return nil, err
		}
		raw, err := l.Bytes()
		if err != nil {
			return nil, err
		}
		var tree any
		if err := marshal.Unmarshal(raw, marshal.YAML, &tree); err != nil {
			return nil, err
		}
		return json.Marshal(tree)
	}
	if in, err = read(mbapi.SchemaRoleInput); err != nil {
		return nil, nil, err
	}
	if out, err = read(mbapi.SchemaRoleOutput); err != nil {
		return nil, nil, err
	}
	return in, out, nil
}

// isNotFound says whether err says the contribution does not exist.
func isNotFound(err error) bool {
	return errors.As(err, new(contribapi.ErrContributionNotFound))
}
