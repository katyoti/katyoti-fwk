// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	skillsapi "github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// skillTaskLibrary is a TaskLibrary source that wraps every skill with
// `provides: [task]` in its frontmatter as a runnable task.
//
// Data source: `mbapi.Catalog` published by ktw-agents's
// sidecarDiscoveryExt. The catalog has every sidecar's `type: skill`
// resources after the boot probe; this library walks them
// (`api.SkillsFromCatalog`) on every ListTasks call. No RPC fan-out
// to remote SkillManager slots — the same data is already
// in-memory from the discovery probe.
//
// Resolved lazily per call so that sidecars whose manifest lands
// later (e.g., after a `catalog refresh`) are picked up automatically.
// Catalog reads are cheap (in-memory map walk).
//
// Per `bus-split-aggregator-to-catalog` step 11 — closes the
// last remaining `SkillManager.List` call site in the task pipeline.
type skillTaskLibrary struct{}

// NewSkillTaskLibrary returns a TaskLibrary source for skill-derived
// tasks. Resolves the SkillManager from the call-time context, so
// composite/remote registrations done after this library is constructed
// are still picked up.
func NewSkillTaskLibrary() taskapi.TaskLibrary {
	return &skillTaskLibrary{}
}

func (l *skillTaskLibrary) ListTasks(ctx context.Context, _ taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	tasks := l.collect(ctx)
	if len(tasks) == 0 {
		return taskapi.ListTasksResponse{}, nil
	}
	summaries := make([]taskapi.TaskSummary, 0, len(tasks))
	for _, t := range tasks {
		summaries = append(summaries, taskSummaryFrom(t))
	}
	return taskapi.ListTasksResponse{Tasks: summaries}, nil
}

func (l *skillTaskLibrary) GetTask(ctx context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	tasks := l.collect(ctx)
	// URI exact match first.
	if t, ok := tasks[in.Name]; ok {
		return taskInfoFrom(t), nil
	}
	// Bare name fallback (mirrors localTaskLibrary semantics).
	var matches []*taskapi.Task
	for _, t := range tasks {
		if t.Name == in.Name {
			matches = append(matches, t)
		}
	}
	if len(matches) == 1 {
		return taskInfoFrom(matches[0]), nil
	}
	if len(matches) > 1 {
		uris := make([]string, 0, len(matches))
		for _, m := range matches {
			uris = append(uris, m.URI())
		}
		return taskapi.TaskInfo{}, fmt.Errorf("ambiguous skill-task name %q across providers: %v", in.Name, uris)
	}
	return taskapi.TaskInfo{}, fmt.Errorf("skill-task %q not found", in.Name)
}

// collect walks the catalog for `type: skill` entries, filters those
// declaring `provides: [task]`, and projects each into a `*Task`. Returns
// an empty map (never nil) on any error so the composite library can keep
// going without short-circuiting.
//
// Body text mirrors the prompt from the previous addRunnableSkills helper
// so the agent's behaviour is identical to before — only WHERE the
// wrapping happens has changed (catalog walk per call vs SkillManager
// RPC fan-out).
func (l *skillTaskLibrary) collect(ctx context.Context) map[string]*taskapi.Task {
	out := map[string]*taskapi.Task{}
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return out
	}
	skills, err := skillsapi.SkillsFromCatalog(ctx, cat)
	if err != nil {
		return out
	}
	for _, info := range skills {
		if !hasProvides(info.Provides, "task") {
			continue
		}
		uri := info.URI()
		if _, exists := out[uri]; exists {
			continue
		}
		out[uri] = &taskapi.Task{
			Name:        info.Name,
			Provider:    info.Provider,
			Type:        "agent",
			Description: info.Description,
			Icon:        info.Icon,
			IconName:    info.IconName,
			Hashtags:    info.Hashtags,
			Category:    info.Category,
			Config: map[string]any{
				presets.ConfigKeyCapability: "smart-best",
				"plugins":                   []any{"augmented"},
			},
			Body: fmt.Sprintf(`A user request will arrive as the first message in this conversation. Your job is to handle it using the "%s" skill methodology.

1. Call **skills-get-instruction** with skill_name "%s" to load the skill's methodology and references.
2. Read the user's request (first message).
3. Apply the skill's methodology to the user's request.
4. Call **complete** with the result when done.

If the first message is empty or generic ("run the skill"), proceed with the skill's default flow — many skills (e.g., interview-technique) define an interactive flow that doesn't require pre-supplied input.`, uri, uri),
		}
	}
	return out
}
