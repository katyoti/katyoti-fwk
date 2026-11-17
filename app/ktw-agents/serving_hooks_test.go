// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// assertRestoresRunsWhenServing: the ktw-agents that ships starts, once it
// serves its universe, the work only the serving process does — the task runs'
// restore after a restart. (The serving-hook point was registered only by a
// constructor nothing in the binary called: the restore never ran — found by a
// live restart test.) Rides on the census's application: a process builds one.
func assertRestoresRunsWhenServing(t *testing.T, app *application.Application) {
	t.Helper()
	var names []string
	for _, h := range application.UseAppExtensions[ktsapi.ServingHook](app) {
		names = append(names, h.Name)
		if h.Name == "task-restore" {
			if !h.First {
				t.Error("the task restore is not a first serving hook: the planner may see runs before they are restored")
			}
			return
		}
	}
	t.Errorf("serving hooks = %v; want the task restore among them", names)
}
