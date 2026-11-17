// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// CatalogTaskLibrary implements `taskapi.TaskLibrary` over the universe
// view: the `mbapi.Catalog` a consumer obtains through
// `manifestuniverse.UniverseCatalogExtension` (in ktw-agents, wired by
// sidecarDiscoveryExt), queried with `mbapi.Query` / `QueryTyped` / `GetOne`
// at universe reach. Listing tasks is one walk of the index — no RPC per
// sidecar. (It replaced a per-sidecar remote task library, deleted; see
// modules/contributions/projections.md for the task library's standing as a
// projection of the catalog, never an authority.)
//
// Behaviour:
//   - `ListTasks`: every `task` entry in the view as a `TaskSummary`
//     (`taskapi.TaskSummariesFromCatalog`). The same task name from two
//     providers stays two entries with distinct URIs (`provider:name`).
//   - `GetTask`: a `provider:name` input resolves the one entry
//     (`mbapi.GetOne`); a bare name scans the kind for a unique match. The
//     `TaskBody.Config` bytes decode back into `map[string]any`.
//   - `InputSchema` / `OutputSchema` are the task's declared I/O contracts,
//     carried in `TaskBody` (see taskInfoFromBody); nil for a task that
//     declares none.
type CatalogTaskLibrary struct {
	cat mbapi.Catalog
}

// NewCatalogTaskLibrary builds a TaskLibrary backed by the given catalog.
// `nil` catalog produces a library that returns empty `ListTasks` and
// a clear error from `GetTask` — useful for binaries where the catalog
// isn't yet wired.
func NewCatalogTaskLibrary(cat mbapi.Catalog) taskapi.TaskLibrary {
	return &CatalogTaskLibrary{cat: cat}
}

// ListTasks returns every `type: task` entry across every cataloged
// provider as a flat `[]TaskSummary`. Multi-provider dedup is implicit
// — the catalog already keys manifests by provider name, so the same
// task name shipped by different providers comes back as separate
// entries with distinct URIs (`provider:name`).
func (l *CatalogTaskLibrary) ListTasks(ctx context.Context, _ taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	tasks, err := taskapi.TaskSummariesFromCatalog(ctx, l.cat)
	if err != nil {
		return taskapi.ListTasksResponse{}, err
	}
	return taskapi.ListTasksResponse{Tasks: tasks}, nil
}

// GetTask resolves a task by URI (`provider:name`) or bare name into
// a full `TaskInfo`. URI-form is the fast path (direct manifest
// lookup). Bare-name form walks every manifest looking for a unique
// match — mirrors `localTaskLibrary.GetTask`'s bare-name semantics
// (single match = success; multiple matches = ambiguity error;
// zero = not found).
func (l *CatalogTaskLibrary) GetTask(ctx context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	if l.cat == nil {
		return taskapi.TaskInfo{}, fmt.Errorf("CatalogTaskLibrary: nil catalog (sidecarDiscoveryExt didn't publish?)")
	}

	// Stage 1: explicit URI (`provider:name`) — addressed, so resolve the
	// one entry rather than materialising every task body to find it.
	provider, name, hasURI := cutOnce(in.Name, ':')
	if hasURI {
		one, err := mbapi.GetOne[mbapi.TaskBody](ctx,
			mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: l.cat},
			provider+":/"+mbapi.ResourceTypeTask+"/"+name)
		if err == nil && one.Body.Name == name {
			return l.withContracts(ctx, taskInfoFromBody(provider, one.Body))
		}
		// A task whose contribution ID differs from its body name is not
		// addressable, so fall through to the scan rather than reporting
		// it missing.
	}

	// Two-plane index: pull every `type: task` body once, resolving each
	// lazily via Catalog.GetBody (content-store hit / one bounded per-hash
	// fetch). Attribution is by the contribution's own provider so layer
	// providers (e.g. "universe") round-trip.
	entries, err := mbapi.QueryTyped[mbapi.TaskBody](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeTask, Catalog: l.cat})
	if err != nil {
		return taskapi.TaskInfo{}, fmt.Errorf("catalog tasks: %w", err)
	}

	if hasURI {
		for _, e := range entries {
			if e.Provider == provider && e.Body.Name == name {
				return l.withContracts(ctx, taskInfoFromBody(provider, e.Body))
			}
		}
		return taskapi.TaskInfo{}, fmt.Errorf("task %q not found in provider %q", in.Name, provider)
	}

	// Stage 2: bare name — collect every match for a unique-match check.
	var matches []*matchedTask
	for _, e := range entries {
		if e.Body.Name == in.Name {
			matches = append(matches, &matchedTask{provider: e.Provider, body: e.Body})
		}
	}
	switch len(matches) {
	case 1:
		return l.withContracts(ctx, taskInfoFromBody(matches[0].provider, matches[0].body))
	case 0:
		return taskapi.TaskInfo{}, fmt.Errorf("task %q not found", in.Name)
	default:
		uris := make([]string, 0, len(matches))
		for _, m := range matches {
			uris = append(uris, m.provider+":"+m.body.Name)
		}
		return taskapi.TaskInfo{}, fmt.Errorf("ambiguous task name %q across providers: %v — disambiguate by using full URI", in.Name, uris)
	}
}

type matchedTask struct {
	provider string
	body     mbapi.TaskBody
}

// withContracts completes a task's info with its input and output contracts,
// read from their own contributions (taskapi.TaskContracts) when its data does
// not carry them inline.
func (l *CatalogTaskLibrary) withContracts(ctx context.Context, info taskapi.TaskInfo) (taskapi.TaskInfo, error) {
	if len(info.InputSchema) > 0 || len(info.OutputSchema) > 0 {
		return info, nil
	}
	in, out, err := taskapi.TaskContracts(ctx, l.cat, info.Provider, info.Name)
	if err != nil {
		return info, fmt.Errorf("task %s: its contracts: %w", info.URI, err)
	}
	info.InputSchema, info.OutputSchema = in, out
	return info, nil
}

// taskInfoFromBody projects a `TaskBody` (+ provider) into the legacy
// `TaskInfo` shape consumers expect from `TaskLibrary.GetTask`. The
// `Config` field is JSON-decoded from the body's opaque bytes — the
// shape `TaskLibrary.GetTask` callers expect.
func taskInfoFromBody(provider string, b mbapi.TaskBody) taskapi.TaskInfo {
	taskapi.NormalizeTaskBody(&b)
	info := taskapi.TaskInfo{
		Name:              b.Name,
		Title:             b.Title,
		Provider:          provider,
		URI:               provider + ":" + b.Name,
		Type:              b.Type,
		Description:       b.Description,
		Advertised:        b.Advertised,
		Body:              b.Body,
		PermissionProfile: b.PermissionProfile,
		Group:             b.Group,
	}
	if len(b.Config) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(b.Config, &cfg); err == nil {
			info.Config = cfg
		}
	}
	// The I/O contracts are the task's own contributions (its contract
	// files): withContracts reads them; a body still carrying them inline
	// (an older member's manifest) is read as it was.
	if len(b.InputSchema) > 0 {
		info.InputSchema = json.RawMessage(b.InputSchema)
	}
	if len(b.OutputSchema) > 0 {
		info.OutputSchema = json.RawMessage(b.OutputSchema)
	}
	return info
}

// cutOnce is `strings.Cut` inlined to avoid an import for one usage.
func cutOnce(s string, sep byte) (before, after string, found bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// Compile-time assertion.
var _ taskapi.TaskLibrary = (*CatalogTaskLibrary)(nil)
