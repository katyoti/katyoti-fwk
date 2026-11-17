// SPDX-License-Identifier: Apache-2.0

package api

// CLI render templates for TaskService methods.
// Used by the auto-generated KTSP CLI commands when --out=rich (default).
//
// Helpers `header`, `table`, `dim`, `count` are registered by the console
// renderer (see internal/core/console). Same template language used by
// SkillManager — see internal/extensions/skills/api/templates.go for the
// established pattern.

const tasksListTemplate = `{{ header "Tasks" -}}
{{ if .tasks -}}
{{ table .tasks "Provider:provider" "name" "type" "capability" "description" }}
{{ dim (printf "%d task(s)" (count .tasks)) }}
{{- else -}}
{{ dim "No tasks found." }}
{{- end }}`

const taskGetTemplate = `{{ header .name }}
{{ if .description }}{{ .description }}{{ end }}

{{ dim "Type:" }} {{ .type }}
{{- if .config }}
{{ dim "Config:" }} {{ .config }}
{{- end }}
{{- if .body }}

{{ dim "Body:" }}
{{ .body }}
{{- end }}`

const tasksStatusTemplate = `{{ header (printf "Run %s" .runID) }}
{{ dim "Task:" }}   {{ .taskName }}
{{ dim "Status:" }} {{ .status }}
{{- if .startedAt }}
{{ dim "Started:" }} {{ .startedAt }}
{{- end }}
{{- if .endedAt }}
{{ dim "Ended:" }}   {{ .endedAt }}
{{- end }}
{{- if .error }}
{{ dim "Error:" }}   {{ .error }}
{{- end }}`

const tasksRunsTemplate = `{{ header "Runs" -}}
{{ if .runs -}}
{{ table .runs "runID" "taskName" "status" "group" "target" "startedAt" }}
{{ dim (printf "%d run(s)" (count .runs)) }}
{{- else -}}
{{ dim "No runs." }}
{{- end }}`

const tasksGroupsTemplate = `{{ header "Task groups" -}}
{{ table .groups "name" "running" "waiting" "queued" "maxRunning" "on" }}`
