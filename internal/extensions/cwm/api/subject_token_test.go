// SPDX-License-Identifier: Apache-2.0

package api

import (
	"regexp"
	"testing"
)

var tokenRe = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

// TestSubjectTokenIsAlwaysValid pins the contract the NATS builders enforce.
// A token that fails this panics inside the builder and events.recoverPublish
// swallows it — the event vanishes silently and a suspended task waits forever.
func TestSubjectTokenIsAlwaysValid(t *testing.T) {
	for _, id := range []string{
		"cwm", "cwm.g1", "compose.CMP-010", "mod-1.UC-001",
		"cwm.p1d-call-sites-migrated",
		"compose.CMP-010-routes-a-skill-request-to-create-skill-and-then-some-more",
		"UPPER.CASE", "with spaces", "dots....everywhere", "-leading", "trailing-",
		"...", "é-accented", "",
	} {
		got := SubjectToken(id)
		if !tokenRe.MatchString(got) {
			t.Errorf("SubjectToken(%q) = %q — not a valid subject token", id, got)
		}
		if len(got) > 63 {
			t.Errorf("SubjectToken(%q) is %d chars, over the 63 limit", id, len(got))
		}
	}
}

// TestSubjectTokenIsInjectiveWhereSlugsCollide is the reason the hash exists.
// These pairs flatten to the SAME slug; if the token were the slug alone, a
// subscriber waiting on one would be woken by writes to the other.
func TestSubjectTokenIsInjectiveWhereSlugsCollide(t *testing.T) {
	for _, pair := range [][2]string{
		{"cwm.g1", "cwm-g1"},
		{"m.UC-1", "m.uc1"},
		{"compose.CMP-010", "compose.cmp010"},
		{"cwm.p1-build-green", "cwm.p1build.green"},
	} {
		a, b := SubjectToken(pair[0]), SubjectToken(pair[1])
		if a == b {
			t.Errorf("SubjectToken(%q) == SubjectToken(%q) == %q — ids that differ must not share a subject",
				pair[0], pair[1], a)
		}
	}
}

// TestSubjectTokenIsDeterministic pins the property the whole scheme rests on:
// publisher and subscriber each derive the token from the id they hold and
// never exchange it, so the function must give the same answer every time.
func TestSubjectTokenIsDeterministic(t *testing.T) {
	const id = "cwm.approval-20260917t101500z"
	first := SubjectToken(id)
	for i := 0; i < 100; i++ {
		if got := SubjectToken(id); got != first {
			t.Fatalf("SubjectToken is not deterministic: %q then %q", first, got)
		}
	}
}

// TestSubjectTokenStaysReadable pins the half that is for humans: the slug
// prefix must survive, or a NATS trace becomes unreadable exactly when someone
// is debugging a task that did not wake.
func TestSubjectTokenStaysReadable(t *testing.T) {
	got := SubjectToken("cwm.g1-module-self-contained")
	const wantPrefix = "cwm-g1-module-self-contained-"
	if len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("SubjectToken = %q, want the readable prefix %q", got, wantPrefix)
	}
}
