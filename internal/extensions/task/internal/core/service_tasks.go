// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

func (s *Service) List(ctx context.Context, in taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	// The catalog only. There is no fallback to the execution map: a task the
	// catalog does not carry is not listed (e24 / N1,
	// TestTaskServiceList_IsTheCatalogOnly).
	if s.library == nil {
		return taskapi.ListTasksResponse{}, nil
	}
	resp, err := s.library.ListTasks(ctx, in)
	if err != nil {
		return resp, err
	}
	resp.Tasks = advertisedOnly(resp.Tasks)
	return resp, nil
}

// advertisedOnly drops tasks with an explicit `advertised: false` from a list
// surface. The discovery surfaces (CLI `tasks list`, KTSP/RPC consumers) stay
// uncluttered with worker/sub-tasks; resolution by exact name via Get is
// unaffected, so hidden tasks remain callable (visibility ≠ callability).
func advertisedOnly(in []taskapi.TaskSummary) []taskapi.TaskSummary {
	out := in[:0:0]
	for _, t := range in {
		if t.Advertised != nil && !*t.Advertised {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Get resolves one task by name or URI: the catalog first, then the
// execution map. Resolution by exact name is not discovery — a task that runs
// when named (Run) describes itself when named too; it is still never listed.
func (s *Service) Get(ctx context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	if s.library != nil {
		if info, err := s.library.GetTask(ctx, in); err == nil {
			return info, nil
		}
	}
	if t, ok := s.tasks[in.Name]; ok {
		return taskInfoFrom(t), nil
	}
	for _, t := range s.tasks {
		if t.Name == in.Name {
			return taskInfoFrom(t), nil
		}
	}
	return taskapi.TaskInfo{}, fmt.Errorf("task %q not found", in.Name)
}

// SetLibrary replaces the task library used for List/Get operations.
// Called by WrapTaskLibraryWithRemotes; every source it passes is a
// projection of the catalog (projections.md), never a loader walk.
func (s *Service) SetLibrary(lib taskapi.TaskLibrary) {
	s.library = lib
}

// taskSummaryFrom builds the compact descriptor returned by ListTasks.
// Capability is the one piece of frontmatter metadata we surface at list
// time — task pickers use it as a sorting/grouping key.
func taskSummaryFrom(t *taskapi.Task) taskapi.TaskSummary {
	var capability string
	if t.Config != nil {
		if v, ok := t.Config["capability"].(string); ok {
			capability = v
		}
	}
	return taskapi.TaskSummary{
		Name:        t.Name,
		Title:       t.Title,
		Provider:    t.Provider,
		URI:         t.URI(),
		Type:        t.Type,
		Description: t.Description,
		Advertised:  t.Advertised,
		Capability:  capability,
		Icon:        t.Icon,
		IconName:    t.IconName,
		Hashtags:    t.Hashtags,
		Category:    t.Category,
		Tags:        t.Tags,
		Kind:        t.Kind,
		Types:       t.Types,
		Audience:    t.Audience,
		Filter:      t.Filter,
		SessionMode: t.SessionMode,
	}
}

// taskInfoFrom builds the full descriptor returned by GetTask.
func taskInfoFrom(t *taskapi.Task) taskapi.TaskInfo {
	cfg := t.Config
	return taskapi.TaskInfo{
		Name:         t.Name,
		Title:        t.Title,
		Provider:     t.Provider,
		URI:          t.URI(),
		Type:         t.Type,
		Description:  t.Description,
		Advertised:   t.Advertised,
		Config:       cfg,
		Body:         t.Body,
		InputSchema:  t.InputSchema,
		OutputSchema: t.OutputSchema,
	}
}

// resolveTask resolves a TaskRunRequest to a concrete Task, or an inline
// TaskDef. A name or URI is resolved in one order, each a source of its own,
// never a lane falling back on another:
//
//  1. the catalog (taskcontrib.CatalogTask): the entry's data layer and its
//     contracts' source layers, so an override entry the catalog applies is
//     the task that runs. A bare name several providers ship is ambiguous
//     unless one of them is this binary's own;
//  2. the ExtraDirs' tasks (provider "user"), which the catalog never carries;
//  3. the library's other sources (the skill-as-task wrappers).
func (s *Service) resolveTask(ctx context.Context, in taskapi.TaskRunRequest) (*taskapi.Task, error) {
	switch {
	case in.TaskName != "":
		// The catalog resource URI `<provider>:/task/<id>` (ktw-gui's
		// listings) is the task URI `<provider>:<id>` too.
		if idx := strings.Index(in.TaskName, ":/task/"); idx >= 0 {
			in.TaskName = in.TaskName[:idx] + ":" + in.TaskName[idx+len(":/task/"):]
		}
		if cat := s.catalog(ctx); cat != nil {
			t, err := taskcontrib.CatalogTask(ctx, cat, in.TaskName, s.ownTaskProviders(ctx)...)
			if err == nil {
				return t, nil
			}
			if !errors.Is(err, taskcontrib.ErrTaskNotInCatalog) {
				return nil, err
			}
		}
		if t, ok := s.tasks[in.TaskName]; ok {
			return t, nil
		}
		if !strings.Contains(in.TaskName, ":") {
			var matches []*taskapi.Task
			for _, t := range s.tasks {
				if t.Name == in.TaskName {
					matches = append(matches, t)
				}
			}
			if len(matches) == 1 {
				return matches[0], nil
			}
			if len(matches) > 1 {
				uris := make([]string, 0, len(matches))
				for _, m := range matches {
					uris = append(uris, m.URI())
				}
				return nil, fmt.Errorf("ambiguous task name %q across providers: %v — disambiguate by using full URI", in.TaskName, uris)
			}
		}
		if s.library != nil {
			info, err := s.library.GetTask(ctx, taskapi.GetTaskRequest{Name: in.TaskName})
			if err == nil && info.Name != "" {
				return &taskapi.Task{
					Name:              info.Name,
					Provider:          info.Provider,
					Type:              info.Type,
					Description:       info.Description,
					Config:            info.Config,
					Body:              info.Body,
					RawConfig:         info.Config,
					InputSchema:       info.InputSchema,
					OutputSchema:      info.OutputSchema,
					PermissionProfile: info.PermissionProfile,
					Group:             info.Group,
				}, nil
			}
		}
		return nil, fmt.Errorf("task %q not found", in.TaskName)

	case len(in.TaskDef) > 0:
		return inlineTask(in.TaskDef), nil

	default:
		return nil, fmt.Errorf("TaskRunRequest must specify taskName or taskDef")
	}
}

// catalog is the catalog the application publishes, resolved at each run
// (never captured at construction): the self view, or the universe view where
// UniverseCatalogExtension is wired. nil before it is published.
func (s *Service) catalog(ctx context.Context) mbapi.Catalog {
	return (&appCatalogTaskLibrary{appCtx: s.appCtx}).catalog(ctx)
}

// ownTaskProviders are the providers of the tasks this binary ships: the one
// a bare name shipped by several providers resolves to.
func (s *Service) ownTaskProviders(ctx context.Context) []string {
	actx := s.appCtx
	if actx == nil {
		actx = ctx
	}
	var out []string
	for _, p := range application.UseExtensions[contribapi.ContributionProvider](actx) {
		if p.Type() == mbapi.ResourceTypeTask {
			out = append(out, p.Provider())
		}
	}
	return out
}

// inlineTaskMeta are an inline definition's keys that describe the task;
// every other top-level key is its config.
var inlineTaskMeta = map[string]bool{"name": true, "type": true, "description": true, "body": true, "config": true}

// inlineTask is the task an inline definition describes: its meta, its body
// (the prompt of an agent), and its config — the top-level keys its producers
// write (a persona's plugins and profile, a delegated worker's capability),
// with a `config` block on top. Reading `config` alone dropped them all, and
// the body: a persona ran as a blank agent.
func inlineTask(def map[string]any) *taskapi.Task {
	name, _ := def["name"].(string)
	typeName, _ := def["type"].(string)
	description, _ := def["description"].(string)
	body, _ := def["body"].(string)
	config := map[string]any{}
	for k, v := range def {
		if !inlineTaskMeta[k] {
			config[k] = v
		}
	}
	if block, ok := def["config"].(map[string]any); ok {
		for k, v := range block {
			config[k] = v
		}
	}
	return &taskapi.Task{Name: name, Type: typeName, Description: description, Body: body, Config: config, RawConfig: def}
}

// hasProvides is shared with skill_task_library.go (the new home of the
// skill-as-task wrapping logic — moved out of NewTaskService to fix the
// init-time race with sidecarDiscoveryExt).
func hasProvides(provides []string, target string) bool {
	for _, p := range provides {
		if p == target {
			return true
		}
	}
	return false
}
