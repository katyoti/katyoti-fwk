// SPDX-License-Identifier: Apache-2.0

package api

const listWorkspacesTemplate = `{{ header "Workspaces" -}}
{{ if .workspaces -}}
{{ table .workspaces "name" "type" "path" }}
{{ dim (printf "%d workspace(s)" (count .workspaces)) }}
{{- else -}}
{{ dim "No workspaces configured. Use 'create-workspace' to add one." }}
{{- end }}`

const listJobsTemplate = `{{ header "Sync Jobs" -}}
{{ if .jobs -}}
{{ table .jobs "name" "source" "dest" "mode" "enabled" "schedule" }}
{{ dim (printf "%d job(s)" (count .jobs)) }}
{{- else -}}
{{ dim "No jobs configured. Use 'create-job' to add one." }}
{{- end }}`

const runResultTemplate = `{{ header "Sync Results" -}}
{{ if .jobs -}}
{{ table .jobs "name" "status" "transferred" "deleted" "errors" "duration" }}
{{- else -}}
{{ dim "No jobs were run." }}
{{- end }}`

const statusTemplate = `{{ header (printf "Status: %s" .name) -}}
{{ if .lastRun -}}
{{ dim (printf "Last run: %s (%s)" .lastRun .lastStatus) }}
{{- end }}
{{ if .changes -}}
{{ header "Pending Changes" -}}
{{ table .changes "action" "path" "size" }}
{{ dim (printf "%d change(s)" (count .changes)) }}
{{- else -}}
{{ dim "No pending changes." }}
{{- end }}`

const getJobTemplate = `{{ header (printf "Job: %s" .name) }}
{{ dim (printf "Source:  %s" .source) }}
{{ dim (printf "Dest:    %s" .dest) }}
{{ dim (printf "Mode:    %s" .mode) }}
{{ dim (printf "Enabled: %v" .enabled) }}`

const getWorkspaceTemplate = `{{ header (printf "Workspace: %s" .name) }}
{{ dim (printf "Type: %s" .type) }}
{{ if .config -}}

{{ header "Config" -}}
{{ range $k, $v := .config -}}
{{ printf "  %-15s  %v" $k $v }}
{{ end }}
{{- end }}`

const listProvidersTemplate = `{{ header "Available Providers" -}}
{{ if .providers -}}
{{ table .providers "name" "description" }}
{{ dim (printf "%d provider(s)" (count .providers)) }}
{{- else -}}
{{ dim "No providers available." }}
{{- end }}`
