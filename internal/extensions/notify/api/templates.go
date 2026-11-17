// SPDX-License-Identifier: Apache-2.0

package api

// CLI render templates for NotifyService methods (the sidecars' common rich style).
// Used by auto-generated KTSP CLI when --out=rich (default).

const listProfilesTmpl = `{{ header "Notification Profiles" -}}
{{ if .profiles -}}
{{ table .profiles "name" }}
{{ dim (printf "%d profile(s)" (count .profiles)) }}
{{- else -}}
{{ dim "No profiles configured." }}
{{- end }}`

const listChannelsTmpl = `{{ header "Notification Channels" -}}
{{ if .channels -}}
{{ table .channels "name" "type" }}
{{ dim (printf "%d channel(s)" (count .channels)) }}
{{- else -}}
{{ dim "No channels configured." }}
{{- end }}`

const listVoicesTmpl = `{{ header "TTS Voices" -}}
{{ if .voices -}}
{{ table .voices "name" "id" "description" }}
{{ dim (printf "%d voice(s)" (count .voices)) }}
{{- else -}}
{{ dim "No voices configured." }}
{{- end }}`
