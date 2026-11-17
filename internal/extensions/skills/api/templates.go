// SPDX-License-Identifier: Apache-2.0

package api

// CLI render templates for SkillManager methods.
// Used by auto-generated KTSP CLI commands for rich output.

const listTemplate = `{{ header "Skills" -}}
{{ if .skills -}}
{{ table .skills "name" "description" "requires" }}
{{ dim (printf "%d skill(s)" (count .skills)) }}
{{- else -}}
{{ dim "No skills found." }}
{{- end }}`

const instructionsTemplate = `{{ if .instructions -}}
{{ .instructions }}
{{- else -}}
{{ dim "No instructions found." }}
{{- end }}`

const referenceTemplate = `{{ if .content -}}
{{ .content }}
{{- else -}}
{{ dim "No content found." }}
{{- end }}`
