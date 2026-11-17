// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// A LIVE GUI WALK RUNS ONLY IN ITS BATCH. A probe that
// drives the GUI on a scenario case takes minutes, needs the GUI built and
// deployed on the case, and is run by the GUI batch (scripts/gui-batch.sh),
// which records its criterion's verdict. GUIWalk is the one answer to « is
// this probe a GUI walk »: verify() (a gate's question) counts such a
// criterion from its recorded verdict and never runs it, and planification
// keeps it out of every check but its batch's.

// LiveTests marks the scripts of live tests run on a scenario case.
const LiveTests = "tests/live/"

// GUIWalk says a probe runs a live walk that drives the GUI: a tests/live
// script of the GUI (its path under ktw-gui) or one naming wails or a
// browser. Any other tests/live script is a CLI probe.
func GUIWalk(probe string) bool {
	for _, w := range strings.Fields(strings.ReplaceAll(probe, "\\", "/")) {
		if !strings.Contains(w, LiveTests) {
			continue
		}
		w = strings.ToLower(w)
		if strings.Contains(w, "ktw-gui") || strings.Contains(w, "wails") || strings.Contains(w, "browser") {
			return true
		}
	}
	return false
}
