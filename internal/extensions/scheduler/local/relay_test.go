// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/relaytest"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A PERSON'S TO-DO RELAYED — reported from an agent's session for the person,
// it passes only relaying that person's report, confirmed, with relaying on,
// and its result records the relay; relaying off it is refused and nothing is
// collected.
func TestAPersonsReportRelayedAndConfirmed(t *testing.T) {
	ctx, _, svc, got := world(t)
	ask := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", For: api.ForPerson, Start: api.StartManual})
	relayed := relaytest.Relaying(ctx, "claude-code", person.ID, "report:"+ask.ID)
	report := api.ReportInput{ID: ask.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "approved", By: person}}

	relaytest.Set(t, false)
	if _, err := svc.Report(relayed, report); err == nil || !strings.Contains(err.Error(), "relaying the person's acts is off") {
		t.Fatalf("relayed with relaying off: %v; want refused", err)
	}
	if len(got.seen()) != 0 {
		t.Fatalf("collected after a refused report: %v", got.seen())
	}
	relaytest.Set(t, true)
	out, err := svc.Report(relayed, report)
	if err != nil {
		t.Fatalf("relayed, confirmed, relaying on: %v", err)
	}
	r := out.Todo.Result
	if r == nil || r.Relayed == nil || r.Relayed.By != "claude-code" || r.Relayed.Confirmed != "prompt" || r.By.ID != person.ID {
		t.Fatalf("result %+v; want the person's, relayed by claude-code at the prompt", r)
	}
}
