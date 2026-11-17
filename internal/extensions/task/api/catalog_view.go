// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TaskSummaryFromCatalog projects a catalog `TaskBody` (+ provider) into
// the legacy `TaskSummary` shape that today's `TaskLibrary.ListTasks`
// consumers expect. Lets a consumer that currently does:
//
//	out, _ := lib.ListTasks(ctx, ListTasksRequest{})
//	for _, t := range out.Tasks { ... }
//
// migrate to:
//
//	tasks, _ := api.TaskSummariesFromCatalog(ctx, catalog)
//	for _, t := range tasks { ... }
//
// without rewriting the consumer's iteration logic. Pure projection —
// no RPC, no allocation beyond the slice header.
//
// `provider` comes from the `Contribution.Provider`; the rest from the
// decoded `TaskBody`. `Body` (the markdown task body) and `Config`
// (frontmatter config block, as opaque bytes) are intentionally NOT
// projected into `TaskSummary` — `TaskSummary` is the picker shape,
// `TaskInfo` carries the full definition. Use `TaskBody` directly when
// you need the full data (future migration deletes `TaskInfo` in favour
// of `TaskBody`).
func TaskSummaryFromCatalog(provider string, body mbapi.TaskBody) TaskSummary {
	NormalizeTaskBody(&body)
	return TaskSummary{
		Name:        body.Name,
		Title:       body.Title,
		Provider:    provider,
		URI:         provider + ":" + body.Name,
		Type:        body.Type,
		Description: body.Description,
		Advertised:  body.Advertised,
		Capability:  body.Capability,
		Icon:        body.Icon,
		IconName:    body.IconName,
		Hashtags:    body.Hashtags,
		Category:    body.Category,
		Tags:        body.Tags,
		Kind:        body.Kind,
		Types:       body.Types,
		Audience:    body.Audience,
		Filter:      body.Filter,
		SessionMode: body.SessionMode,
	}
}

// TaskSummariesFromCatalog walks every loaded manifest in the catalog,
// extracts `type: task` resource entries, and returns them as a flat
// `[]TaskSummary`. Order matches the catalog's iteration.
//
// Unadvertised tasks (`TaskBody.Advertised == &false`) are filtered out
// here so the discovery surfaces (GUI gallery, task picker, agent system
// prompt) stay uncluttered with worker/sub-tasks. The catalog itself still
// carries them — direct resolution by exact name/URI via GetTask keeps
// working (visibility ≠ callability), mirroring the skills convention.
//
// Empty catalog (no descriptors loaded) returns an empty slice + nil
// error.
//
// SOURCE-SIDE replacement for `TaskLibrary.ListTasks` across consumers
// (agent system prompt builder, delegate tool, GUI Inspector, task
// picker). Per `bus-split-aggregator-to-catalog`.
func TaskSummariesFromCatalog(ctx context.Context, cat mbapi.Catalog) ([]TaskSummary, error) {
	return taskSummariesFromCatalog(ctx, cat, false)
}

// TaskSummariesFromCatalogAll is like TaskSummariesFromCatalog but INCLUDES
// unadvertised tasks, each carrying its real `Advertised` flag. Use when a
// consumer must enumerate the full set (e.g. resolving an exact name that
// may be hidden). The discovery surfaces keep using the advertised-only
// TaskSummariesFromCatalog.
func TaskSummariesFromCatalogAll(ctx context.Context, cat mbapi.Catalog) ([]TaskSummary, error) {
	return taskSummariesFromCatalog(ctx, cat, true)
}

func taskSummariesFromCatalog(ctx context.Context, cat mbapi.Catalog, includeUnadvertised bool) ([]TaskSummary, error) {
	if cat == nil {
		return nil, nil
	}
	// Two-plane index: walk every `type: task` index entry and resolve
	// each body lazily via Catalog.GetBody (content-store hit / one
	// bounded per-hash fetch). Attribution is by the contribution's own
	// provider so layer providers (e.g. "universe") round-trip.
	entries, err := mbapi.QueryTyped[mbapi.TaskBody](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeTask, Catalog: cat})
	if err != nil {
		return nil, err
	}
	out := make([]TaskSummary, 0, len(entries))
	for _, e := range entries {
		// Default-true: only an explicit `advertised: false` hides it.
		advertised := e.Body.Advertised == nil || *e.Body.Advertised
		if !includeUnadvertised && !advertised {
			continue
		}
		out = append(out, TaskSummaryFromCatalog(e.Provider, e.Body))
	}
	return out, nil
}
