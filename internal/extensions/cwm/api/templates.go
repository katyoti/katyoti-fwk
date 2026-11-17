// SPDX-License-Identifier: Apache-2.0

package api

// CLI render templates for CWMService methods.
// Used by auto-generated KTSP CLI commands for rich output.
//
// Templates use the standard auto-CLI helpers: header, table, dim,
// printf, count, range, add. See:
//   - internal/extensions/leanix/api/templates.go
//   - internal/extensions/kb/api/templates.go
// for reference patterns.
//
// Templates land here as their methods get full I/O types in
// subtasks 2-7. Currently declared as empty constants so cwm.go's
// CWMServiceDef compiles. Real bodies fill in per-subtask.

// Subtask 2: node templates
const nodeListTmpl = `{{ header "Nodes" -}}
{{ if .nodes -}}
{{ table .nodes "id" "type" "name" "slug" "priority" }}
{{ dim (printf "%d node(s)" (count .nodes)) }}
{{- else -}}
{{ dim "No nodes." }}
{{- end }}`

const nodeTreeTmpl = `{{ header "Tree" -}}
{{ if .nodes -}}
{{ range .nodes -}}
{{ printf "%s· %s (%s)" .indent .node.name .node.type }}
{{ end }}
{{ dim (printf "%d node(s)" (count .nodes)) }}
{{- else -}}
{{ dim "Empty tree." }}
{{- end }}`

// Subtask 3: item template — per-kind grouped list using
// StructuredFields. Renders one-liners (no body dumps).
const itemEffectiveTmpl = `{{ header (printf "Effective items: %s" .kind) -}}
{{ if .items -}}
{{ range .items -}}
{{ printf "  %s — %s" .id .title }}{{ if .summary }} · {{ .summary }}{{ end }}{{ if .source_node }} · from {{ .source_node }}{{ end }}
{{ end }}
{{ dim (printf "%d effective item(s)" (count .items)) }}
{{- else -}}
{{ dim "No effective items." }}
{{- end }}`

// Subtask 7: lint templates
