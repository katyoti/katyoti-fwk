// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// ErrTaskNotInCatalog says the catalog carries no task by that name: the
// caller may look in a source of its own (the task service's ExtraDirs).
var ErrTaskNotInCatalog = errors.New("task not in the catalog")

// CatalogTask is the task the runtime runs for ref, read from the catalog:
// its data layer (the frontmatter as written, its body folded in, or the
// override entry that replaced it) read through the body lane and turned
// into a task by the same reading as Parse; its input and output contracts,
// when the data declares none inline, read through the content lane
// (taskapi.TaskContracts: the source layer of its contract data-types). The
// lanes never fall back on one another: a data layer that does not read as a
// task is that task's error, never a re-read of its file.
//
// ref is a task URI (`provider:name`), its resource form
// (`provider:/task/name`) or a bare name. A bare name shipped by several
// providers is ambiguous, unless exactly one of them is in prefer (the
// caller's own providers). ErrTaskNotInCatalog when no entry answers.
func CatalogTask(ctx context.Context, cat mbapi.Catalog, ref string, prefer ...string) (*taskapi.Task, error) {
	if cat == nil {
		return nil, fmt.Errorf("task %q: %w (no catalog)", ref, ErrTaskNotInCatalog)
	}
	if i := strings.Index(ref, ":/"+mbapi.ResourceTypeTask+"/"); i >= 0 {
		ref = ref[:i] + ":" + ref[i+len(":/"+mbapi.ResourceTypeTask+"/"):]
	}
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	provider, name, isURI := strings.Cut(ref, ":")
	if !isURI {
		name, provider = ref, ""
	}
	uri := ""
	if isURI {
		one, err := mbapi.GetOne[json.RawMessage](ctx, q, provider+":/"+mbapi.ResourceTypeTask+"/"+name)
		switch {
		case err == nil:
			return catalogTaskOf(ctx, cat, one.Provider, one.URI, one.Body)
		case !notInCatalog(err):
			return nil, fmt.Errorf("task %q: %w", ref, err)
		}
	}
	// Not addressed by its id: the task whose data names it (an entry whose
	// id is not its name, or a bare name).
	entries, err := mbapi.QueryTyped[mbapi.TaskBody](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeTask, Catalog: cat})
	if err != nil {
		return nil, fmt.Errorf("task %q: catalog tasks: %w", ref, err)
	}
	var matches []mbapi.Attributed[mbapi.TaskBody]
	for _, e := range entries {
		if e.Body.Name == name && (!isURI || e.Provider == provider) {
			matches = append(matches, e)
		}
	}
	if len(matches) > 1 {
		var own []mbapi.Attributed[mbapi.TaskBody]
		for _, m := range matches {
			for _, p := range prefer {
				if m.Provider == p {
					own = append(own, m)
					break
				}
			}
		}
		if len(own) == 1 {
			matches = own
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("task %q: %w", ref, ErrTaskNotInCatalog)
	case 1:
		uri = matches[0].URI
	default:
		uris := make([]string, 0, len(matches))
		for _, m := range matches {
			uris = append(uris, m.Provider+":"+m.Body.Name)
		}
		return nil, fmt.Errorf("ambiguous task name %q across providers: %v — disambiguate by using full URI", ref, uris)
	}
	one, err := mbapi.GetOne[json.RawMessage](ctx, q, uri)
	if err != nil {
		return nil, fmt.Errorf("task %q: %w", ref, err)
	}
	return catalogTaskOf(ctx, cat, one.Provider, one.URI, one.Body)
}

// catalogTaskOf reads one entry's data layer as a task and completes it with
// its contracts.
func catalogTaskOf(ctx context.Context, cat mbapi.Catalog, provider, uri string, data json.RawMessage) (*taskapi.Task, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("task %s: no data layer", uri)
	}
	// Decoded as YAML, of which JSON is a subset: the fields get the types
	// the file's frontmatter gives them (an integer stays an int).
	var raw map[string]any
	if err := marshal.Unmarshal(data, marshal.YAML, &raw); err != nil {
		return nil, fmt.Errorf("task %s: its data layer: %w", uri, err)
	}
	body, _ := raw["body"].(string)
	fields := make(map[string]any, len(raw))
	for k, v := range raw {
		if k != "body" {
			fields[k] = v
		}
	}
	t, err := taskFromFields(fields, body, true)
	if err != nil {
		return nil, fmt.Errorf("task %s: %w", uri, err)
	}
	t.Provider = provider
	if len(t.InputSchema) == 0 || len(t.OutputSchema) == 0 {
		in, out, err := taskapi.TaskContracts(ctx, cat, provider, t.Name)
		if err != nil {
			return nil, fmt.Errorf("task %s: its contracts: %w", uri, err)
		}
		if len(t.InputSchema) == 0 {
			t.InputSchema = in
		}
		if len(t.OutputSchema) == 0 {
			t.OutputSchema = out
		}
	}
	return t, nil
}

// notInCatalog says whether err says the catalog has no such entry.
func notInCatalog(err error) bool {
	return errors.Is(err, mbapi.ErrNotInView) || errors.Is(err, mbapi.ErrSidecarNotInCatalog) ||
		errors.As(err, new(contribapi.ErrContributionNotFound))
}
