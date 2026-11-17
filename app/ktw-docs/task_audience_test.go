// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

// ktw-docs declares whom its tasks are for:
// « Import d'un modèle PowerPoint » and « Création d'une
// présentation » are end-user actions a tagged universe offers at its first
// level; « Aperçu d'un thème » is a demo.
func TestDocsTasksDeclareTheirAudience(t *testing.T) {
	for task, want := range map[string]string{
		"deck-theme-import":     taskapi.AudienceEndUser,
		"slidedeck-fast-create": taskapi.AudienceEndUser,
		"slidedeck-theme-demo":  taskapi.AudienceDemo,
	} {
		body, err := embeddedFS.ReadFile("embed/tasks/" + task + ".md")
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := taskcontrib.Parse(body)
		if err != nil {
			t.Fatalf("%s: %v", task, err)
		}
		if parsed.Audience != want {
			t.Errorf("%s: audience %q, want %q", task, parsed.Audience, want)
		}
	}
}
