// SPDX-License-Identifier: Apache-2.0

package api

const statusTmpl = `{{ header (printf "KB: %s" .name) }}
{{ if .sources -}}

{{ header "Sources" -}}
{{ table .sources "name" "type" "lastSynced" }}
{{- end }}
{{ if .bundles -}}

{{ header "Bundles" -}}
{{ dim (printf "Total: %.0f  Pending: %.0f" .bundles.total .bundles.pending) }}
{{- end }}
{{ if .index -}}

{{ header "Index" -}}
{{ dim (printf "Backend: %s" .index.backend) }}
{{ dim (printf "Vectors: %.0f" .index.vectorCount) }}
{{- end }}`

const queryTmpl = `{{ header "Query Results" -}}
{{ if .results -}}
{{ range $i, $r := .results -}}

{{ printf "  %d. [%.2f] %s" (add $i 1) $r.score $r.source.path }}
{{ printf "     %s" $r.content }}
{{ end }}
{{ dim (printf "%d result(s)" (count .results)) }}
{{- else -}}
{{ dim "No results." }}
{{- end }}`
