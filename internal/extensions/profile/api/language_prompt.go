// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// languageNames names each supported language in English (the language of the
// agents' instructions).
var languageNames = map[string]string{"en": "English", "fr": "French"}

// LanguagePrompt is the system-prompt block that tells an agent the language
// of the person it works for: its replies, its questions and the documents it
// writes follow it, whatever the language of its instructions. Agent
// instructions stay in their source language; only the agent's output follows
// the person. Empty for an empty or unsupported language (the agent then
// follows the conversation).
func LanguagePrompt(lang string) string {
	name, ok := languageNames[strings.TrimSpace(lang)]
	if !ok {
		return ""
	}
	return "<person_language>\n" +
		"The person you work for reads " + name + " (" + lang + "). " +
		"Write your replies, your questions to them and every document you produce in " + name + ", " +
		"whatever the language of these instructions or of the task. " +
		"Keep identifiers, code, file paths, URIs and the names of controls in the interface exactly as they are. " +
		"Quote an object's content in the language it was written in.\n" +
		"</person_language>"
}
