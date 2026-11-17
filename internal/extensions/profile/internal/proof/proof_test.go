// SPDX-License-Identifier: Apache-2.0

package proof

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// RFC 6238's test vectors (SHA1, 8 digits, the ASCII secret 1234...890).
func TestRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{59: "94287082", 1111111109: "07081804", 1111111111: "14050471",
		1234567890: "89005924", 2000000000: "69279037", 20000000000: "65353130"} {
		if got := code(secret, stepAt(time.Unix(unix, 0)), 8); got != want {
			t.Errorf("T=%d: %s, want %s", unix, got, want)
		}
	}
}

// testStore seals by reversing bytes (not the identity, so nothing reads it
// plain by accident) at a clock the test moves.
func testStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	rev := func(b []byte) ([]byte, error) {
		out := make([]byte, len(b))
		for i := range b {
			out[len(b)-1-i] = b[i] ^ 0x5a
		}
		return out, nil
	}
	s := &Store{Dir: t.TempDir(), Now: func() time.Time { return now }, seal: rev, unseal: rev}
	return s, &now
}

func secretOf(t *testing.T, s *Store) []byte {
	t.Helper()
	st, err := s.read()
	if err != nil || st == nil {
		t.Fatalf("read: %v %v", st, err)
	}
	return st.Secret
}

// A code proves one act once: within ±1 step, never twice, never an older
// step; nothing enrolled or unconfirmed fails closed; wrong codes slow the
// next attempts down; revoking fails closed again; every attempt is journaled.
func TestACodeProvesOneActOnce(t *testing.T) {
	s, now := testStore(t)
	if err := s.Prove("jdoe", "decide:m.OD-1", "123456"); err != ErrNotEnrolled {
		t.Fatalf("nothing enrolled: %v; want it to fail closed", err)
	}
	uri, b32, err := s.Enrol("KaTyoti", "jdoe")
	if err != nil || !strings.HasPrefix(uri, "otpauth://totp/KaTyoti:jdoe?") || !strings.Contains(uri, "secret="+b32) {
		t.Fatalf("enrol: %q %q %v", uri, b32, err)
	}
	sec := secretOf(t, s)
	if err := s.Prove("jdoe", "decide:m.OD-1", code(sec, stepAt(*now), digits)); err != ErrNotEnrolled {
		t.Fatalf("enrolled, unconfirmed: %v; want it to fail closed", err)
	}
	if err := s.Confirm(code(sec, stepAt(*now), digits)); err != nil {
		t.Fatal(err)
	}
	// The step the confirmation used is spent: the next one proves.
	*now = now.Add(period * time.Second)
	c := code(sec, stepAt(*now), digits)
	if err := s.Prove("jdoe", "decide:m.OD-1", c); err != nil {
		t.Fatalf("a fresh code: %v", err)
	}
	if err := s.Prove("jdoe", "answer:sys-inbox.e1", c); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("the same code for another act: %v; want refused, wait for the next", err)
	}
	later := *now
	*now = now.Add(5 * period * time.Second)
	if err := s.Prove("jdoe", "answer:sys-inbox.e1", c); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("the used code again, once expired: %v; want refused as already used", err)
	}
	*now = later
	*now = now.Add(period * time.Second)
	if err := s.Prove("jdoe", "answer:sys-inbox.e1", code(sec, stepAt(*now)+1, digits)); err != nil {
		t.Fatalf("a code one step ahead (the app's clock): %v", err)
	}
	*now = now.Add(3 * period * time.Second)
	if err := s.Prove("jdoe", "hold:k", code(sec, stepAt(*now)-2, digits)); err == nil {
		t.Fatal("a code two steps old proved")
	}
	if err := s.Prove("jdoe", "", code(sec, stepAt(*now), digits)); err == nil {
		t.Fatal("a code proved no act")
	}

	// Wrong codes slow down, then the window passes.
	for i := 0; i < backoffFailures; i++ {
		_ = s.Prove("jdoe", "hold:k", "000000")
	}
	if err := s.Prove("jdoe", "hold:k", code(sec, stepAt(*now), digits)); err == nil || !strings.Contains(err.Error(), "wait") {
		t.Fatalf("after %d wrong codes: %v; want slowed down", backoffFailures, err)
	}
	*now = now.Add(backoffWindow + time.Minute)
	if err := s.Prove("jdoe", "hold:k", code(sec, stepAt(*now), digits)); err != nil {
		t.Fatalf("after the window: %v", err)
	}

	journal, _ := os.ReadFile(filepath.Join(s.Dir, journalFile))
	if lines := strings.Count(string(journal), "\n"); lines < 10 || !strings.Contains(string(journal), `"act":"decide:m.OD-1","ok":true`) {
		t.Fatalf("journal: %d lines\n%s", lines, journal)
	}
	sealed, _ := os.ReadFile(filepath.Join(s.Dir, stateFile))
	if bytes.Contains(sealed, sec) || strings.Contains(string(sealed), "secret") {
		t.Fatal("the state file holds the secret in the clear")
	}
	if err := s.Revoke(); err != nil {
		t.Fatal(err)
	}
	if err := s.Prove("jdoe", "hold:k", code(sec, stepAt(*now)+1, digits)); err != ErrNotEnrolled {
		t.Fatalf("revoked: %v; want it to fail closed", err)
	}
}

// On Windows the secret is sealed with DPAPI and unsealed again.
func TestTheSecretIsSealedByThePlatform(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI is Windows'")
	}
	plain := []byte("a secret of twenty b")
	sealed, err := seal(plain)
	if err != nil || bytes.Contains(sealed, plain) {
		t.Fatalf("seal: %v", err)
	}
	back, err := unseal(sealed)
	if err != nil || !bytes.Equal(back, plain) {
		t.Fatalf("unseal: %q %v", back, err)
	}
}
