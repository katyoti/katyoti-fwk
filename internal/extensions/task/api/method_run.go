// SPDX-License-Identifier: Apache-2.0

package api

// A long method run as a run: the `method` machine strategy
// runs one method of any service under a display panel it owns.

// MethodMachine is the strategy name of a method run.
const MethodMachine = "method"

// ProgressPanelType is the panel type a method run opens: the generic steps
// and progress type.
const ProgressPanelType = "ktw-agents:/panel-type/progress"

// MethodRunDef is the inline task (TaskRunRequest.TaskDef) that runs the
// method uri as a run whose panel is panelID - minted by the caller with
// panel.NewID, so it answers the run and the panel at once; title names the
// panel (the uri when empty); object is the CWM object the run works on (its
// id, empty for none), named on the panel's open so a kept outcome lands on
// it. The method's input is the request's Input.
func MethodRunDef(uri, panelID, title, object string) map[string]any {
	cfg := map[string]any{"uri": uri, "panel": panelID}
	if title != "" {
		cfg["title"] = title
	}
	if object != "" {
		cfg["object"] = object
	}
	return map[string]any{"type": MethodMachine, "name": "method-run", "config": cfg}
}
