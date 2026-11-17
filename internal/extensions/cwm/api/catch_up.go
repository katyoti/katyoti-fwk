// SPDX-License-Identifier: Apache-2.0

package api

// THE CATCH-UP'S THRESHOLD: a catch-up of CatchUpPerObjectMax
// moved objects or fewer publishes, beside its grouped `project.caught-up`
// event, one object event per object (operation caught-up), so per-object
// reactions run as for a verb write. Above it the grouped event says Held
// and no per-object event goes out until CatchUpRelease is called with the
// grouped event's id - the answer to its « À faire » card. A catch-up never
// writes an object file either way.

// CatchUpPerObjectMax is the most moved objects a catch-up publishes one
// event each for without asking.
const CatchUpPerObjectMax = 50

// CatchUpReleaseInput names the held catch-up by its grouped event's id.
type CatchUpReleaseInput struct {
	EventID string `json:"event_id" jsonschema:"The id of the project.caught-up event that held its per-object events"`
}

// CatchUpReleaseOutput says how many per-object events went out.
type CatchUpReleaseOutput struct {
	Published int `json:"published" jsonschema:"How many per-object events were published"`
}
