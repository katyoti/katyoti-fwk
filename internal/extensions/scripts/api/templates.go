// SPDX-License-Identifier: Apache-2.0

package api

const listScriptsTmpl = `{{ header "Scripts" -}}
{{ if .scripts -}}
{{ table .scripts "name" "provider" "extension" "description" }}
{{ dim (printf "%d script(s)" (count .scripts)) }}
{{- else -}}
{{ dim "No scripts found." }}
{{- end }}`
