// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	taskcore "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
	imethodrun "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/methodrun"
	contextplugin "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/plugin/context"
	itaskplugin "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/plugin/task"
	iwkflow "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/wkflow"
)

// Extension returns the full task service extension (TaskService runner).
// Only ktw-agents should wire this — sidecars contribute task DEFINITIONS
// via `ContributeTaskResources` only and do not publish a per-binary
// `task-library` slot.
//
// Tasks are surfaced cross-binary as `Type: "task"` Contributions through
// the manifest's ContributionProvidersPoint; ktw-agents discovers them
// via `<sidecar>:resources/list --type task` (universe-wide through
// CatalogService) and feeds them into its own TaskService runner.
// Sidecars are pure contributors — there is no `task-library` KTSP slot
// on a sidecar binary (per Phase E §5).
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishService[taskapi.TaskService, taskcore.TaskServiceConfig](
			taskcore.NewTaskService,
			appcore.WithServiceDef(taskapi.TaskServiceDef),
			appcore.WithKTSPContainer(),
		))

		// Once the process serves its universe, restore the runs a previous
		// process left (restore.go).
		contributeRestore(init)

		// Contribute the canonical TaskBody data-type Contribution
		// shipped by this extension. The hub's catalog Phase-2 wire-up
		// registers it with SchemaService at boot. Empty provider →
		// resolved to the binary's app name at Contribute time (per
		// cli-command-surface ADR §2.2).
		manifestcontrib.ContributeDataTypeResources(
			"",
			loader.NewEmbedFSLoader(dataTypesFS, "."),
			"embed/data-types",
		)(init)
	}
}

// Re-exported extensions from internal packages.
// Bootstrap imports local/ only — these expose the extension registration.

// TaskPluginExtension contributes the complete/delegate agent plugin strategy.
var TaskPluginExtension = itaskplugin.Extension

// ContextPluginExtension contributes the task context agent plugin strategy.
var ContextPluginExtension = contextplugin.Extension

// WorkflowExtension contributes the workflow machine strategy.
var WorkflowExtension = iwkflow.Extension

// MethodExtension contributes the method machine strategy: a long method
// run as a run that owns its display panel.
var MethodExtension = imethodrun.Extension
