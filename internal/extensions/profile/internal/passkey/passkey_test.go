// SPDX-License-Identifier: Apache-2.0

package passkey

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/passkeytest"
)

const origin = "https://pc.example.test"

func testStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := time.Now()
	flip := func(b []byte) ([]byte, error) {
		out := make([]byte, len(b))
		for i := range b {
			out[i] = b[i] ^ 0x5a
		}
		return out, nil
	}
	s := New(t.TempDir(), flip, flip)
	s.Now = func() time.Time { return now }
	return s, &now
}

// THE ONE-TAP PHONE APPROVAL: a passkey registered from the phone signs, with
// user verification, a challenge bound to one act; it is checked once, for
// that act and person; anything else is refused.
func TestAPasskeyApprovesOneActOnce(t *testing.T) {
	s, now := testStore(t)
	ph := passkeytest.NewPhone(t, "pc.example.test")

	if _, _, err := s.BeginApproval("jdoe", "decide:m.OD-1", origin); err == nil || !strings.Contains(err.Error(), "no passkey") {
		t.Fatalf("an approval with no passkey: %v; want refused", err)
	}
	if _, _, err := s.BeginRegistration("jdoe", "http://pc.example.test"); err == nil {
		t.Fatal("a registration over plain http began")
	}
	h, opts, err := s.BeginRegistration("jdoe", origin)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRegistration(h, ph.Create(t, origin, opts)); err != nil {
		t.Fatalf("registration: %v", err)
	}
	if n := s.Count("jdoe"); n != 1 {
		t.Fatalf("passkeys: %d, want 1", n)
	}

	approve := func(act string, p *passkeytest.Phone) (string, error) {
		t.Helper()
		h, opts, err := s.BeginApproval("jdoe", act, origin)
		if err != nil {
			t.Fatal(err)
		}
		return h, s.FinishApproval(h, p.Get(t, origin, opts))
	}
	h1, err := approve("decide:m.OD-1", ph)
	if err != nil {
		t.Fatalf("a Face ID approval: %v", err)
	}
	if err := s.Check(h1, "decide:m.OD-2", "jdoe"); err == nil {
		t.Fatal("the approval held for another act")
	}
	if err := s.Check(h1, "decide:m.OD-1", "jdoe"); err != nil {
		t.Fatalf("the approval for its act: %v", err)
	}
	if err := s.Check(h1, "decide:m.OD-1", "jdoe"); err == nil {
		t.Fatal("an approval checked twice")
	}

	// Without Face ID.
	ph.UV = false
	if _, err := approve("decide:m.OD-3", ph); err == nil {
		t.Fatal("an approval without user verification passed")
	}
	ph.UV = true
	// Another key, never registered.
	if _, err := approve("decide:m.OD-4", passkeytest.NewPhone(t, "pc.example.test")); err == nil {
		t.Fatal("an unregistered passkey approved")
	}
	// A replayed answer: the same signed answer for the same challenge again.
	h2, opts2, _ := s.BeginApproval("jdoe", "decide:m.OD-5", origin)
	answer := ph.Get(t, origin, opts2)
	if err := s.FinishApproval(h2, answer); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if err := s.FinishApproval(h2, answer); err == nil {
		t.Fatal("a replayed answer passed")
	}
	// A counter that goes back (a cloned key).
	h3, opts3, _ := s.BeginApproval("jdoe", "decide:m.OD-6", origin)
	ph.Count = 0
	if err := s.FinishApproval(h3, ph.Get(t, origin, opts3)); err == nil {
		t.Fatal("a counter going back passed")
	}
	ph.Count = 100
	// A stale challenge.
	h4, opts4, _ := s.BeginApproval("jdoe", "decide:m.OD-7", origin)
	*now = now.Add(pendingTTL + time.Minute)
	if err := s.FinishApproval(h4, ph.Get(t, origin, opts4)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("a stale challenge: %v; want expired", err)
	}
	// Revoked: no approval begins.
	if err := s.Revoke("jdoe"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.BeginApproval("jdoe", "decide:m.OD-8", origin); err == nil {
		t.Fatal("an approval began after revoking")
	}
}
