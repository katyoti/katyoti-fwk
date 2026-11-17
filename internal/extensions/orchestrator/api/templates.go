// SPDX-License-Identifier: Apache-2.0

package api

// CLI render templates for OrchestratorService methods (the sidecar CLI style).

const listUniversesTmpl = `{{ header "Universes" -}}
{{ if .universes -}}
{{ table .universes "name" "displayName" "homePath" "protected" }}
{{ dim (printf "%d universe(s)" (count .universes)) }}
{{- else -}}
{{ dim "No universes registered." }}
{{- end }}`

const getUniverseTmpl = `{{ header (printf "Universe: %s" .summary.name) }}
{{ dim (printf "Display:    %s" .summary.displayName) }}
{{ dim (printf "Home:       %s" .summary.homePath) }}
{{ if .summary.description -}}
{{ dim (printf "Description: %s" .summary.description) }}
{{- end }}
{{ if .summary.protected -}}
{{ dim "Protected:  yes" }}
{{- end }}
{{ if .sidecars -}}

{{ header "Sidecars" -}}
{{ table .sidecars "name" "transport" "binary" }}
{{ dim (printf "%d sidecar(s)" (count .sidecars)) }}
{{- else -}}
{{ dim "No sidecars activated." }}
{{- end }}`

// listProjectsTmpl renders the attached project paths. ListProjectsOutput carries
// a plain []string, so this is a bullet list rather than a table.
const listProjectsTmpl = `{{ header "Attached projects" -}}
{{ if .projects -}}
{{ range .projects }}{{ printf "  %s" . }}
{{ end -}}
{{ dim (printf "%d project(s)" (count .projects)) }}
{{- else -}}
{{ dim "No projects attached to this universe." }}
{{- end }}`
