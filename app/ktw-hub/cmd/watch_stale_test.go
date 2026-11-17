// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
)

// The watch compares against the RECORD, not a baseline taken when it
// started. Modelled here as the decision the watch makes each tick: a member
// whose record says "built from digest A" while the live tree is B is stale on
// the FIRST tick — there is no "has it changed since I started looking?"
// anywhere in the decision. That question is what let an override edited
// between hydrate and watch-start stay invisible for the life of the process.
func TestWatch_StaleAgainstTheRecordFiresOnTheFirstTick(t *testing.T) {
	members := []appcore.SidecarSpec{
		{Name: "ktw-a", Binary: "a.exe"},
		{Name: "ktw-b", Binary: "b.exe"},
		{Name: "no-binary"},
	}
	// The record for ktw-b disagrees with its live sources from the very
	// first look; nothing "changed" while watching.
	fresh := func(name, _ string) (bool, error) {
		return name != "ktw-b", nil
	}
	got := staleMembers(members, fresh)
	if len(got) != 1 || got[0] != "ktw-b" {
		t.Fatalf("staleMembers = %v, want [ktw-b] on the first tick", got)
	}

	// A freshness error is stale, not fresh: the safe direction.
	errs := func(name, _ string) (bool, error) {
		if name == "ktw-a" {
			return false, errors.New("stat binary: no such file")
		}
		return true, nil
	}
	if got := staleMembers(members, errs); len(got) != 1 || got[0] != "ktw-a" {
		t.Fatalf("a freshness error must count as stale, got %v", got)
	}

	// Everything fresh: nothing to do, no baseline involved.
	if got := staleMembers(members, func(string, string) (bool, error) { return true, nil }); len(got) != 0 {
		t.Fatalf("all fresh: want nothing stale, got %v", got)
	}
}
