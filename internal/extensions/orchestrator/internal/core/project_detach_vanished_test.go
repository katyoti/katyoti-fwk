// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// A FOLDER THAT NO LONGER EXISTS IS STILL DETACHED. Its
// pointer file went with it, so detach finds the universe by name - or, none
// named, by the universe whose projects.yaml lists the folder - and removes
// the entry instead of answering « detached » and changing nothing.
func TestDetachProject_VanishedFolder(t *testing.T) {
	for _, named := range []bool{true, false} {
		t.Run(map[bool]string{true: "universe named", false: "universe found by its list"}[named], func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("KTW_KATYOTI_ROOT", root)
			application.ResetGlobals()
			t.Cleanup(application.ResetGlobals)
			seedUniverseFor(t, root, "acme")
			seedUniverseFor(t, root, "other")

			svc := &Service{}
			ctx := context.Background()
			project := t.TempDir()
			if _, err := svc.AttachProject(ctx, api.AttachProjectInput{ProjectDir: project, Universe: "acme"}); err != nil {
				t.Fatalf("attach: %v", err)
			}
			if err := os.RemoveAll(project); err != nil {
				t.Fatal(err)
			}

			in := api.DetachProjectInput{ProjectDir: project}
			if named {
				in.Universe = "acme"
			}
			if _, err := svc.DetachProject(ctx, in); err != nil {
				t.Fatalf("detach: %v", err)
			}
			out, err := svc.ListProjects(ctx, api.ListProjectsInput{Universe: "acme"})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Projects) != 0 {
				t.Fatalf("after detaching the vanished folder, acme still lists %v", out.Projects)
			}
		})
	}
}
