// SPDX-License-Identifier: Apache-2.0

package engine

// PanelInputURI is the event URI of a display panel's input (submit,
// action, message, cancel) - the subject subjects.PanelInSubject names
func PanelInputURI(panelID string) string {
	return "ktw-agents:!universe/panel/" + panelID + "/in"
}

// PanelInputWait is the wait condition on a display panel's input: a durable
// wait - the task service's event bridge subscribes its URI each time the run
// waits, after a restart too - matched under label.
func PanelInputWait(panelID, label string) WaitCondition {
	uri := PanelInputURI(panelID)
	return WaitCondition{Topic: ExternalEventTopic(uri), EventURI: uri, Label: label}
}
